package connector

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"connection-keeper/internal/generation"
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	ErrGenerationType = errors.New("generation has an unexpected type")
)

// input реализует коннектор входящего типа.
//
// Он использует общую механику из `connector` и добавляет специфику input-сценария:
// создание input-соединения, handover ресурсов и дренаж старого поколения.
type input[C any] struct {
	connector[C]
	constructorFunc domain.CreateInputConnectionFunc[C]
}

// NewInput создаёт input-коннектор.
//
// Параметры:
//   - kind: тип коннектора.
//   - conf: конфигурация retry/readiness/disable-readiness.
//   - isConnectionInvalidated: функция проверки, нужно ли обновлять соединение.
//   - constructorFunc: функция создания input-соединения.
//   - mgr: общий менеджер lifecycle.
//
// Возвращаемое значение:
//   - domain.Connector: инициализированный input-коннектор.
func NewInput[C any](
	kind domain.Kind,
	conf intlDomain.ConnectorConfig,
	isConnectionInvalidated domain.ConnectionInvalidationFunc,
	constructorFunc domain.CreateInputConnectionFunc[C],
	mgr intlDomain.Manager,
) intlDomain.Connector {
	i := &input[C]{
		constructorFunc: constructorFunc,
	}
	i.kind = kind
	i.isConnectionInvalidatedFunc = isConnectionInvalidated
	i.conf = conf
	i.mgr = mgr

	return i
}

// retryConnect создаёт input-соединение и проверяет его готовность с повторами.
//
// Параметры:
//   - ctx: контекст операции.
//   - conf: конфигурация retry/readiness.
//   - logger: логгер для диагностических сообщений.
//
// Возвращаемые значения:
//   - *C: созданное соединение.
//   - domain.ReadinessFunc: функция проверки готовности созданного соединения.
//   - domain.HandoverConnectionFunc[C]: функция передачи ресурсов этому соединению.
//   - error: объединённая ошибка попыток создания/проверки.
func (i *input[C]) retryConnect(
	ctx context.Context,
	conf intlDomain.ConnectorConfig,
	logger domain.Logger,
) (*C, domain.ReadinessFunc, domain.HandoverConnectionFunc[C], error) {
	var (
		err, errs     error
		retryLimit    = conf.ConnectRetryCount()
		conn          *C
		readinessFunc domain.ReadinessFunc
		handoverFunc  domain.HandoverConnectionFunc[C]
	)
	if retryLimit == 0 {
		retryLimit = 1
	}
	for ; retryLimit > 0; retryLimit-- {
		conn, readinessFunc, handoverFunc, err = i.constructorFunc(ctx)
		if err != nil {
			logger.WarnContext(ctx, fmt.Sprintf("connection constructor failed: %v", err))
			errs = errors.Join(errs, err)
			if retryLimit > 1 {
				if err = i.waitRetryInterval(ctx, conf.ConnectRetryInterval()); err != nil {
					logger.WarnContext(ctx, fmt.Sprintf("connect retry interval wait failed: %v", err))

					return nil, nil, nil, errors.Join(errs, err)
				}
			}

			continue
		}

		err = i.EnsureConnectionReady(ctx, conf, logger, readinessFunc)
		if err == nil {
			break
		}
		logger.WarnContext(ctx, fmt.Sprintf("readiness check failed: %v", err))
		errs = errors.Join(errs, err)
		if retryLimit > 1 {
			if err = i.waitRetryInterval(ctx, conf.ReadinessRetryInterval()); err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("readiness retry interval wait failed: %v", err))

				return nil, nil, nil, errors.Join(errs, err)
			}
		}
	}
	if err != nil {
		return nil, nil, nil, errs
	}

	return conn, readinessFunc, handoverFunc, nil
}

// disableAndTimeout по необходимости отключает readiness менеджера и ждёт окно переключения.
//
// Параметры:
//   - ctx: контекст ожидания.
//   - conf: конфигурация disable-readiness.
//
// Возвращаемое значение:
//   - error: ошибка отмены контекста или nil.
func (i *input[C]) disableAndTimeout(
	ctx context.Context,
	conf intlDomain.ConnectorConfig,
) error {
	if !conf.DisableReadinessOnUpdate() {
		return nil
	}
	if readiness := i.mgr.DisableReadiness(); !readiness {
		return nil
	}
	timer := time.NewTimer(conf.DisableReadinessOnUpdateTimeout())
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return errors.Join(ErrCtxCancel, ctx.Err())
	case <-timer.C:
		return nil
	}
}

// getGenerationAndCancelFunc получает актуальное поколение input-коннектора и его handover callback.
//
// Параметры:
//   - registryMgr: менеджер registry.
//
// Возвращаемые значения:
//   - domain.Generation[C]: текущее поколение input-коннектора.
//   - domain.HandoverGenerationFunc[C]: callback вывода поколения из активной эксплуатации.
func (i *input[C]) getGenerationAndCancelFunc(
	registryMgr intlDomain.RegistryManager,
) (intlDomain.Generation[C], intlDomain.HandoverGenerationFunc[C]) {
	var (
		genT, cancelFuncT = registryMgr.GetGenerationAndCancelFunc(i.kind)
		gen               intlDomain.Generation[C]
		handoverFunc      intlDomain.HandoverGenerationFunc[C]
		ok                bool
	)

	// Fast path без лишних проверок типа: registry хранит значения строго в согласованном формате
	// внутри библиотеки, поэтому assertion выполняется напрямую ради скорости.
	if genT != nil {
		gen, ok = genT.(intlDomain.Generation[C])
	}
	if !ok {
		return nil, nil
	}

	if cancelFuncT != nil {
		handoverFunc = cancelFuncT.(intlDomain.HandoverGenerationFunc[C])
	}

	return gen, handoverFunc
}

// ReconnectIfNeeded при необходимости пересоздаёт input-соединение и публикует новое поколение.
//
// Параметры:
//   - ctx: контекст операции.
//   - force: принудительный флаг обновления.
//
// Возвращаемые значения:
//   - bool: true, если поколение было обновлено.
//   - error: ошибка создания/передачи ресурсов/публикации.
func (i *input[C]) ReconnectIfNeeded(
	ctx context.Context,
	force bool,
) (bool, error) {
	var (
		registryMgr       = i.mgr.RegistryManager()
		logger            = i.mgr.Logger()
		gen, handoverFunc = i.getGenerationAndCancelFunc(registryMgr)
		update            bool
		err               error
	)

	if gen != nil {
		// Проверяем, нужно ли нам обновление
		update, err = i.shouldReconnect(
			ctx,
			i.conf,
			gen,
			logger,
			force,
		)
		if err != nil {
			return false, err
		}
	}

	if !update && !force {
		return false, nil
	}

	// обновление нужно, создаём соединение
	conn, connReadinessFunc, connHandoverFunc, err := i.retryConnect(ctx, i.conf, logger)
	if err != nil {
		return false, err
	}

	// Соединение создано и первично проверено. Начинаем процесс передачи ресурсов.
	var (
		drainFunc domain.DrainingFunc
	)
	// по необходимости, выключаем сервис и ждем определённое время, пока запросы на него прекратятся
	err = i.disableAndTimeout(ctx, i.conf)
	if err != nil {
		return false, err
	}
	// Увеличиваем версию. Мы тут не работаем с гонкой, так что пишем в прямую
	i.version++
	// Передаём ресурсы старого поколения новому, если старое поколение уже существует.
	// По внутреннему контракту registry handoverFunc не может быть задан без gen.
	if handoverFunc != nil {
		drainFunc, err = handoverFunc(conn)
		if err != nil {
			return false, err
		}
	}
	// Создаём новое поколение и регистрируем его.
	newGen, newCancel := generation.NewInputGeneration(ctx, i.version, conn, connReadinessFunc, connHandoverFunc)
	registryMgr.SetGeneration(i.kind, newGen, newCancel)

	// Отправляем старое поколение в дренаж.
	// По внутреннему контракту registry handoverFunc не может быть задан без gen.
	if handoverFunc != nil {
		drainMgr := i.mgr.DrainManager()
		drainMgr.Register(gen, drainFunc)
	}

	return true, nil
}

// Close запускает вывод текущего input-поколения из активной эксплуатации.
//
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - error: ошибка запуска handover/draing callback или nil.
func (i *input[C]) Close() error {
	var (
		registryMgr       = i.mgr.RegistryManager()
		gen, handoverFunc = i.getGenerationAndCancelFunc(registryMgr)
	)

	if gen == nil && handoverFunc == nil {
		return nil
	}

	// По внутреннему контракту registry handoverFunc не может быть задан без gen.
	if handoverFunc != nil {
		drainMgr := i.mgr.DrainManager()
		drainFunc, err := handoverFunc(nil)
		if err != nil {
			return err
		}
		drainMgr.Register(gen, drainFunc)
	}

	return nil
}
