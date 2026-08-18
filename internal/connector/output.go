package connector

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"connection-keeper/internal/generation"
	"context"
	"errors"
	"fmt"
)

// output реализует коннектор исходящего типа.
//
// Он использует общую механику из `connector` и добавляет специфику output-сценария:
// создание output-соединения, shutdown старого поколения и дренаж его ресурсов.
type output[C any] struct {
	connector[C]
	constructorFunc domain.CreateOutputConnectionFunc[C]
}

// NewOutput создаёт output-коннектор.
//
// Параметры:
//   - kind: тип коннектора.
//   - conf: конфигурация retry/readiness/disable-readiness.
//   - isConnectionInvalidated: функция проверки, нужно ли обновлять соединение.
//   - constructorFunc: функция создания output-соединения.
//   - mgr: общий менеджер lifecycle.
//
// Возвращаемое значение:
//   - domain.Connector: инициализированный output-коннектор.
func NewOutput[C any](
	kind domain.Kind,
	conf intlDomain.ConnectorConfig,
	isConnectionInvalidated domain.ConnectionInvalidationFunc,
	constructorFunc domain.CreateOutputConnectionFunc[C],
	mgr intlDomain.Manager,
) intlDomain.Connector {
	o := &output[C]{
		constructorFunc: constructorFunc,
	}
	o.kind = kind
	o.isConnectionInvalidatedFunc = isConnectionInvalidated
	o.conf = conf
	o.mgr = mgr

	return o
}

// retryConnect создаёт output-соединение и проверяет его готовность с повторами.
//
// Параметры:
//   - ctx: контекст операции.
//   - conf: конфигурация retry/readiness.
//   - logger: логгер для диагностических сообщений.
//
// Возвращаемые значения:
//   - *C: созданное соединение.
//   - domain.ReadinessFunc: функция проверки готовности созданного соединения.
//   - domain.ShutdownConnectionFunc: функция остановки ресурсов созданного соединения.
//   - error: объединённая ошибка попыток создания/проверки.
func (o *output[C]) retryConnect(
	ctx context.Context,
	conf intlDomain.ConnectorConfig,
	logger domain.Logger,
) (*C, domain.ReadinessFunc, domain.ShutdownConnectionFunc, error) {
	var (
		err, errs     error
		retryLimit    = conf.ConnectRetryCount()
		conn          *C
		readinessFunc domain.ReadinessFunc
		shutdownFunc  domain.ShutdownConnectionFunc
	)
	if retryLimit == 0 {
		retryLimit = 1
	}
	for ; retryLimit > 0; retryLimit-- {
		conn, readinessFunc, shutdownFunc, err = o.constructorFunc(ctx)
		if err != nil {
			logger.WarnContext(ctx, fmt.Sprintf("connection constructor failed: %v", err))
			errs = errors.Join(errs, err)
			if retryLimit > 1 {
				if err = o.waitRetryInterval(ctx, conf.ConnectRetryInterval()); err != nil {
					logger.WarnContext(ctx, fmt.Sprintf("connect retry interval wait failed: %v", err))

					return nil, nil, nil, errors.Join(errs, err)
				}
			}

			continue
		}

		err = o.EnsureConnectionReady(ctx, conf, logger, readinessFunc)
		if err == nil {
			break
		}

		logger.WarnContext(ctx, fmt.Sprintf("readiness check failed: %v", err))
		errs = errors.Join(errs, err)
		if retryLimit > 1 {
			if err = o.waitRetryInterval(ctx, conf.ReadinessRetryInterval()); err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("readiness retry interval wait failed: %v", err))

				return nil, nil, nil, errors.Join(errs, err)
			}
		}

	}
	if err != nil {
		return nil, nil, nil, errs
	}

	return conn, readinessFunc, shutdownFunc, nil
}

// getGenerationAndCancelFunc получает актуальное поколение output-коннектора и его shutdown callback.
//
// Параметры:
//   - registryMgr: менеджер registry.
//
// Возвращаемые значения:
//   - domain.Generation[C]: текущее поколение output-коннектора.
//   - domain.ShutdownGenerationFunc: callback вывода поколения из активной эксплуатации.
func (o *output[C]) getGenerationAndCancelFunc(
	registryMgr intlDomain.RegistryManager,
) (intlDomain.Generation[C], intlDomain.ShutdownGenerationFunc) {
	var (
		genT, cancelFuncT = registryMgr.GetGenerationAndCancelFunc(o.kind)
		gen               intlDomain.Generation[C]
		shutdownFunc      intlDomain.ShutdownGenerationFunc
	)
	// Fast path без лишних проверок типа: registry хранит значения строго в согласованном формате
	// внутри библиотеки, поэтому assertion выполняется напрямую ради скорости.
	if genT != nil {
		gen = genT.(intlDomain.Generation[C])
	}
	if cancelFuncT != nil {
		shutdownFunc = cancelFuncT.(intlDomain.ShutdownGenerationFunc)
	}

	return gen, shutdownFunc
}

// ReconnectIfNeeded при необходимости пересоздаёт output-соединение и публикует новое поколение.
//
// Параметры:
//   - ctx: контекст операции.
//   - force: принудительный флаг обновления.
//
// Возвращаемые значения:
//   - bool: true, если поколение было обновлено.
//   - error: ошибка создания/остановки старого поколения/публикации.
func (o *output[C]) ReconnectIfNeeded(
	ctx context.Context,
	force bool,
) (bool, error) {
	var (
		registryMgr       = o.mgr.RegistryManager()
		logger            = o.mgr.Logger()
		gen, shutdownFunc = o.getGenerationAndCancelFunc(registryMgr)
		update            bool
		err               error
	)

	if gen != nil {
		// Проверяем, нужно ли нам обновление
		update, err = o.shouldReconnect(
			ctx,
			o.conf,
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
	conn, connReadinessFunc, connShutdownFunc, err := o.retryConnect(ctx, o.conf, logger)
	if err != nil {
		return false, err
	}

	// Соединение создано и первично проверено. Начинаем процесс замещения.
	// Увеличиваем версию. Мы тут не работаем с гонкой, так что пишем напрямую.
	o.version++
	newGen, newCancel := generation.NewOutputGeneration(ctx, o.version, conn, connReadinessFunc, connShutdownFunc)
	// Публикуем новое поколение до shutdown старого по контракту этого шага lifecycle:
	// в случае ошибки shutdown выполнение поднимает ошибку выше, что приводит к завершению
	// работы библиотеки и общему пути очистки системы.
	registryMgr.SetGeneration(o.kind, newGen, newCancel)

	// По внутреннему контракту registry shutdownFunc не может быть задан без gen.
	if shutdownFunc != nil {
		var (
			drainFunc domain.DrainingFunc
		)
		drainFunc, err = shutdownFunc()
		if err != nil {
			return false, err
		}
		drainMgr := o.mgr.DrainManager()
		drainMgr.Register(gen, drainFunc)
	}

	return true, nil
}

// Close запускает вывод текущего output-поколения из активной эксплуатации.
//
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - error: ошибка запуска shutdown/draining callback или nil.
func (o *output[C]) Close() error {
	var (
		registryMgr       = o.mgr.RegistryManager()
		gen, shutdownFunc = o.getGenerationAndCancelFunc(registryMgr)
	)

	// По внутреннему контракту registry shutdownFunc не может быть задан без gen.
	if shutdownFunc != nil {
		drainMgr := o.mgr.DrainManager()
		drainFunc, err := shutdownFunc()
		if err != nil {
			return err
		}
		drainMgr.Register(gen, drainFunc)
	}

	return nil
}
