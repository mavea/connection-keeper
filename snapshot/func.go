package snapshot

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	intlKind "connection-keeper/internal/kind"
	intlConnector "connection-keeper/internal/snapshot"
	"errors"
)

var (
	ErrInvalidSnapshotConnectionType = errors.New("invalid snapshot connection type")
)

func NewSnapshot[C any](
	name string,
	mgr intlDomain.Manager,
	constructorFunc domain.CreateSnapshotFunc[C],
) (domain.Kind, error) {
	kindMgr := mgr.KindManager()
	kind, err := intlKind.NewKind[C](name, kindMgr)
	if err != nil {
		return nil, err
	}
	registryMgr := mgr.RegistryManager()
	registryMgr.RegisterGeneration(kind)

	connectorMgr := mgr.ConnectorManager()
	err = connectorMgr.RegisterConnector(kind, intlConnector.NewSnapshot(kind, constructorFunc, mgr))
	if err != nil {
		// Допускаем возможный orphan-slot в registry при rollback: на этой стадии внешнее соединение ещё не создано,
		// поэтому нет утечки внешних ресурсов; это осознанный компромисс текущего lifecycle-дизайна.
		kindMgr.Delete(kind)
		return nil, err
	}

	return kind, nil
}

func Connection[C any](
	builder domain.SnapshotBuilder,
	kind domain.KindType[C],
) (*C, error) {
	conn, err := builder.Connection(kind)
	if err != nil {
		return nil, err
	}
	typedConn, ok := conn.(*C)
	if !ok {
		return nil, ErrInvalidSnapshotConnectionType
	}
	return typedConn, nil
}
