package domain

import (
	rootDomain "connection-keeper/domain"
	"context"
	"time"
)

// Connector - отвечает за управление жизненным циклом поколений соединения.
type Connector interface {
	// ReconnectIfNeeded Обновление соединения, по необходимости.
	// Необходимость проверяется методом вызова IsConnectionInvalidated и EnsureConnectionReady и анализа переданной force.
	// Если IsConnectionInvalidated или force дали истину, или EnsureConnectionReady вернула ошибку, происходит пересоздание поколения соединения
	//
	// Параметры:
	//   - ctx: контекст операции обновления.
	//   - force: флаг принудительного обновления.
	// Возвращаемые значения:
	//   - bool: true, если поколение было обновлено в этом вызове.
	//   - error: ошибка создания/переключения/закрытия поколений.
	ReconnectIfNeeded(ctx context.Context, force bool) (bool, error)

	// Version возвращает текущую версию коннектора.
	// Параметры: отсутствуют.
	// Возвращаемое значение:
	//   - uint: номер текущей версии поколения.
	Version() uint
	// Kind возвращает идентификатор типа коннектора.
	// Параметры: отсутствуют.
	// Возвращаемое значение:
	//   - Kind: тип коннектора.
	Kind() rootDomain.Kind
	// Close запускает вывод текущего поколения коннектора из активной эксплуатации.
	//
	// Параметры: отсутствуют.
	// Возвращаемое значение:
	//   - error: ошибка старта handover/shutdown или регистрации дренажа.
	Close() error
}

// ConfigReadinessCheck описывает параметры повторной проверки готовности соединения.
type ConfigReadinessCheck interface {
	// ReadinessRetryCount возвращает количество попыток проверки готовности.
	// Возвращаемое значение:
	//   - uint8: число попыток readiness-проверки.
	ReadinessRetryCount() uint8
	// ReadinessRetryTimeout возвращает таймаут одной попытки проверки готовности.
	// Возвращаемое значение:
	//   - time.Duration: таймаут одной readiness-проверки.
	ReadinessRetryTimeout() time.Duration
	// ReadinessRetryInterval возвращает паузу между попытками проверки готовности.
	// Возвращаемое значение:
	//   - time.Duration: интервал между readiness-повторами.
	ReadinessRetryInterval() time.Duration
}

// ConfigConnectRetry описывает параметры повторов создания/переподключения соединения.
type ConfigConnectRetry interface {
	// ConnectRetryCount возвращает количество попыток создания/переподключения.
	// Возвращаемое значение:
	//   - uint8: число connect-попыток.
	ConnectRetryCount() uint8
	// ConnectRetryInterval возвращает паузу между попытками создания/переподключения.
	// Возвращаемое значение:
	//   - time.Duration: интервал между connect-повторами.
	ConnectRetryInterval() time.Duration
}

// ConnectorConfig объединяет конфигурацию retry-политик и переключения readiness при обновлении.
type ConnectorConfig interface {
	ConfigReadinessCheck
	ConfigConnectRetry

	// DisableReadinessOnUpdate возвращает флаг временного отключения readiness во время обновления поколения.
	// Возвращаемое значение:
	//   - bool: true, если readiness нужно временно отключать.
	DisableReadinessOnUpdate() bool
	// DisableReadinessOnUpdateTimeout возвращает окно ожидания после отключения readiness.
	// Возвращаемое значение:
	//   - time.Duration: длительность окна ожидания перед переключением.
	DisableReadinessOnUpdateTimeout() time.Duration
}
