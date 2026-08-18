package domain

import (
	rootDomain "connection-keeper/domain"
	"context"
	"time"
)

type RunConfig interface {
	ConnectionCheckInterval() time.Duration
	DisableReadinessTimeout() time.Duration
	StopTimeout() time.Duration
}

type Manager interface {
	DrainManager() DrainManager

	// Получить менеджер kind идентификаторов
	KindManager() KindManager

	// ConnectorManager Получить менеджер конструкторов поколений соединений
	ConnectorManager() ConnectorManager

	// RegistryManager Получить менеджер активных соединений
	RegistryManager() RegistryManager

	// Readiness Возвращает информацию о том, включен ли режим готовности данного менеджера
	Readiness() bool

	// DisableReadiness Отключает режим готовности данного менеджера и возвращает предыдущее значение
	DisableReadiness() (oldValue bool)

	// EnableReadiness Включает режим готовности данного менеджера и возвращает предыдущее значение
	EnableReadiness() (oldValue bool)

	// SetReadiness Устанавливает режим готовности данного менеджера и возвращает предыдущее значение
	SetReadiness(value bool) (oldValue bool)

	// Logger Возвращает логгер
	Logger() rootDomain.Logger

	SetLogger(logger rootDomain.Logger) error

	Run(ctx context.Context, conf RunConfig) error

	Shutdown() error
}
