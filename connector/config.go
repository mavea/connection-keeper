package connector

import (
	"connection-keeper/internal/domain"
	"time"
)

// config — простая реализация domain.ConnectorConfig.
//
// Структура используется как удобный «ленивый» конструктор конфигурации
// (без выделения отдельных типов под каждый параметр), в том числе в тестах,
// где важно быстро собрать набор значений для сценария.
type config struct {
	readinessRetryCount    uint8
	readinessRetryTimeout  time.Duration
	readinessRetryInterval time.Duration

	connectRetryCount    uint8
	connectRetryInterval time.Duration

	disableReadinessOnUpdate        bool
	disableReadinessOnUpdateTimeout time.Duration
}

// NewConfig создаёт конфигурацию коннектора из переданных значений.
//
// Параметры:
//   - readinessRetryCount: количество повторов проверки готовности.
//   - readinessRetryTimeout: общий таймаут на цикл повторов проверки готовности.
//   - readinessRetryInterval: пауза между попытками проверки готовности.
//   - connectRetryCount: количество повторов создания/переподключения.
//   - connectRetryInterval: пауза между попытками подключения.
//   - disableReadinessOnUpdate: включает режим временного отключения readiness при обновлении поколения.
//   - disableReadinessOnUpdateTimeout: длительность окна отключённой readiness при обновлении.
//
// Возвращаемое значение:
//   - domain.ConnectorConfig: готовая конфигурация коннектора.
func NewConfig(
	readinessRetryCount uint8,
	readinessRetryTimeout time.Duration,
	readinessRetryInterval time.Duration,

	connectRetryCount uint8,
	connectRetryInterval time.Duration,

	disableReadinessOnUpdate bool,
	disableReadinessOnUpdateTimeout time.Duration,
) domain.ConnectorConfig {
	return &config{
		readinessRetryCount:    readinessRetryCount,
		readinessRetryTimeout:  readinessRetryTimeout,
		readinessRetryInterval: readinessRetryInterval,

		connectRetryCount:    connectRetryCount,
		connectRetryInterval: connectRetryInterval,

		disableReadinessOnUpdate:        disableReadinessOnUpdate,
		disableReadinessOnUpdateTimeout: disableReadinessOnUpdateTimeout,
	}
}

// ConnectRetryCount возвращает количество повторов подключения.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - uint8: число попыток повторного подключения.
func (c *config) ConnectRetryCount() uint8 {
	return c.connectRetryCount
}

// ConnectRetryInterval возвращает интервал между попытками подключения.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - time.Duration: задержка между попытками подключения.
func (c *config) ConnectRetryInterval() time.Duration {
	return c.connectRetryInterval
}

// ReadinessRetryCount возвращает количество повторов проверки готовности.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - uint8: число попыток проверки готовности.
func (c *config) ReadinessRetryCount() uint8 {
	return c.readinessRetryCount
}

// ReadinessRetryTimeout возвращает общий таймаут цикла проверки готовности.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - time.Duration: длительность таймаута проверки готовности.
func (c *config) ReadinessRetryTimeout() time.Duration {
	return c.readinessRetryTimeout
}

// ReadinessRetryInterval возвращает интервал между проверками готовности.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - time.Duration: задержка между проверками готовности.
func (c *config) ReadinessRetryInterval() time.Duration {
	return c.readinessRetryInterval
}

// DisableReadinessOnUpdate сообщает, нужно ли временно отключать readiness при обновлении поколения.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - bool: true, если readiness нужно временно отключать на время обновления.
func (c *config) DisableReadinessOnUpdate() bool {
	return c.disableReadinessOnUpdate
}

// DisableReadinessOnUpdateTimeout возвращает длительность окна отключённой readiness при обновлении.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - time.Duration: время, в течение которого readiness остаётся отключённой.
func (c *config) DisableReadinessOnUpdateTimeout() time.Duration {
	return c.disableReadinessOnUpdateTimeout
}
