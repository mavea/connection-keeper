package connector

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"errors"
	"fmt"
	"time"
)

var (
	// ErrCtxCancel возвращается, когда операция прервана отменой контекста.
	ErrCtxCancel = errors.New("context canceled")
)

// connector содержит общую логику для input/output коннекторов.
//
// Структура не используется самостоятельно: она встраивается в `input` и `output`,
// где дополняется специализированным поведением создания и замены соединений.
type connector[C any] struct {
	isConnectionInvalidatedFunc domain.ConnectionInvalidationFunc
	kind                        domain.Kind
	conf                        intlDomain.ConnectorConfig

	mgr intlDomain.Manager

	version uint
}

// EnsureConnectionReady проверяет готовность соединения с повторами.
//
// Параметры:
//   - ctx: контекст операции проверки.
//   - conf: конфигурация retry/timeout для readiness-проверок.
//   - logger: логгер для диагностических сообщений.
//   - connectionReadinessFunc: функция фактической проверки готовности соединения.
//
// Возвращаемое значение:
//   - error: объединённая ошибка всех неуспешных попыток или nil при успехе.
func (c *connector[C]) EnsureConnectionReady(
	ctx context.Context,
	conf intlDomain.ConfigReadinessCheck,
	logger domain.Logger,
	connectionReadinessFunc func(ctx context.Context) error,
) error {
	var (
		err, errs  error
		retryLimit = conf.ReadinessRetryCount()
	)

	if retryLimit == 0 {
		retryLimit = 1
	}
	for ; retryLimit > 0; retryLimit-- {
		select {
		case <-ctx.Done():
			logger.WarnContext(ctx, fmt.Sprintf("readiness check interrupted: %v", ErrCtxCancel))
			return errors.Join(errs, ErrCtxCancel)
		default:
		}

		ctxTry, cancel := context.WithTimeout(ctx, conf.ReadinessRetryTimeout())
		err = connectionReadinessFunc(ctxTry)
		cancel()
		if err == nil {
			break
		}
		logger.WarnContext(ctx, fmt.Sprintf("readiness check failed: %v", err))
		errs = errors.Join(errs, err)

		if retryLimit > 1 {
			if err = c.waitRetryInterval(ctx, conf.ReadinessRetryInterval()); err != nil {
				logger.WarnContext(ctx, fmt.Sprintf("readiness retry interval wait failed: %v", err))
				return errors.Join(errs, err)
			}
		}
	}
	if err != nil {
		return errs
	}

	return nil
}

// IsConnectionInvalidated проверяет, требуется ли обновление соединения.
//
// Параметры: отсутствуют.
// Возвращаемые значения:
//   - bool: true, если текущее соединение нужно пересоздать.
//   - error: ошибка проверки invalidation-состояния.
func (c *connector[C]) IsConnectionInvalidated() (bool, error) {
	return c.isConnectionInvalidatedFunc()
}

// shouldReconnect определяет, нужно ли выполнять переподключение в текущем цикле.
//
// Параметры:
//   - ctx: контекст операции.
//   - conf: конфигурация readiness-проверок.
//   - gen: текущее поколение соединения.
//   - logger: логгер для диагностических сообщений.
//   - force: принудительный флаг обновления.
//
// Возвращаемые значения:
//   - bool: true, если нужно выполнить reconnect.
//   - error: ошибка проверки invalidation-состояния.
//
// Важно: ошибка readiness-проверки здесь трактуется как сигнал к обновлению
// и не возвращается наружу как фатальная.
func (c *connector[C]) shouldReconnect(
	ctx context.Context,
	conf intlDomain.ConfigReadinessCheck,
	gen intlDomain.Generation[C],
	logger domain.Logger,
	force bool,
) (bool, error) {
	update, err := c.IsConnectionInvalidated()
	if err != nil {
		logger.WarnContext(ctx, fmt.Sprintf("check needs update failed: %v", err))
		return false, err
	}
	if update || force {
		return true, nil
	}
	if gen == nil {
		return true, nil
	}
	err = c.EnsureConnectionReady(ctx, conf, logger, gen.Readiness)
	if err != nil {
		logger.WarnContext(ctx, fmt.Sprintf("readiness check failed: %v", err))
		return true, nil
	}

	return false, nil
}

// waitRetryInterval ожидает паузу между попытками.
//
// Параметры:
//   - ctx: контекст ожидания.
//   - retryInterval: длительность паузы между попытками.
//
// Возвращаемое значение:
//   - error: ошибка отмены контекста или nil.
//
// Важно: если retryInterval <= 0, ожидание пропускается (не ждём).
// В этом режиме возвращается только текущее состояние ctx (обычно nil, если контекст ещё активен).
func (c *connector[C]) waitRetryInterval(ctx context.Context, retryInterval time.Duration) error {
	if retryInterval <= 0 {
		return ctx.Err()
	}

	timer := time.NewTimer(retryInterval)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return errors.Join(ErrCtxCancel, ctx.Err())
	case <-timer.C:
		return nil
	}
}

// Version возвращает текущую версию коннектора.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - uint: номер версии, увеличиваемый при успешной смене поколения.
func (c *connector[C]) Version() uint {
	return c.version
}

// Kind возвращает тип коннектора.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - domain.Kind: идентификатор типа коннектора.
func (c *connector[C]) Kind() domain.Kind {
	return c.kind
}
