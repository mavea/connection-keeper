package lifecycle

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"errors"
	"sync"
)

var (
	ErrFabricAlreadyRegistered   = errors.New("fabric already registered")
	ErrFabricConstructorNotFound = errors.New("fabric constructor not found")
)

type connector struct {
	connectors []intlDomain.Connector

	// mu используется только для конкурентной регистрации коннекторов.
	// Чтение через GetConnector выполняется без локов в рамках single-thread сценария.
	mu sync.Mutex
}

// NewConnectorManager создаёт менеджер коннекторов.
// Возвращаемое значение:
//   - domain.ConnectorManager: менеджер хранения коннекторов.
func NewConnectorManager() intlDomain.ConnectorManager {
	return &connector{
		connectors: make([]intlDomain.Connector, 0),
	}
}

// RegisterConnector регистрирует коннектор для указанного kind.
// Параметры:
//   - kind: идентификатор типа коннектора (используется как индекс).
//   - con: коннектор для регистрации.
//
// Возвращаемое значение:
//   - error: ErrFabricAlreadyRegistered, если коннектор для этого kind уже зарегистрирован.
//
// Важно: метод потокобезопасен для многопоточной регистрации.
func (c *connector) RegisterConnector(kind domain.Kind, con intlDomain.Connector) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	id := kind.ID()
	registerSlice(&c.connectors, id)
	if t := c.connectors[id]; t != nil {
		return ErrFabricAlreadyRegistered
	}

	c.connectors[id] = con

	return nil
}

// GetConnector возвращает ранее зарегистрированный коннектор по kind.
// Параметры:
//   - kind: идентификатор типа коннектора (используется как индекс).
//
// Возвращаемые значения:
//   - domain.Connector: найденный коннектор.
//   - error: ErrFabricConstructorNotFound, если коннектор отсутствует.
//
// Важно: метод выполняется без блокировок для скорости и предполагает отсутствие
// конкурентного вызова с RegisterConnector.
func (c *connector) GetConnector(kind domain.Kind) (intlDomain.Connector, error) {
	id := kind.ID()
	if id >= len(c.connectors) {
		return nil, ErrFabricConstructorNotFound
	}
	con := c.connectors[id]
	if con == nil {
		return nil, ErrFabricConstructorNotFound
	}

	return con, nil
}

func (c *connector) List() []intlDomain.Connector {
	return c.connectors
}
