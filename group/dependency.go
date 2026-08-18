package group

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"errors"
)

var (
	ErrNilSourceConnector    = errors.New("nil source connector")
	ErrNilDependentConnector = errors.New("nil dependent connector")
)

type dependency struct {
	group
	sourceConnector    intlDomain.Connector
	dependentConnector intlDomain.Connector
	sourceVersion      uint
	dependentVersion   uint
}

func NewDependency(
	name string,
	mgr intlDomain.Manager,
	sourceConnector domain.Kind,
	dependentConnector domain.Kind,
) (domain.KindGroup, error) {

	if sourceConnector == nil {
		return nil, ErrNilSourceConnector
	}
	if dependentConnector == nil {
		return nil, ErrNilDependentConnector
	}

	depStruct, kind, err := newGroup[dependency](name, mgr)
	if err != nil {
		return nil, err
	}

	depStruct.kind = kind
	connectorMgr := mgr.ConnectorManager()

	depStruct.sourceConnector, err = connectorMgr.GetConnector(sourceConnector)
	if err != nil {
		return nil, err
	}
	depStruct.sourceVersion = depStruct.sourceConnector.Version()

	depStruct.dependentConnector, err = connectorMgr.GetConnector(dependentConnector)
	if err != nil {
		return nil, err
	}
	depStruct.dependentVersion = depStruct.dependentConnector.Version()

	err = connectorMgr.RegisterConnector(kind, depStruct)
	if err != nil {
		return nil, err
	}

	return kind, nil
}

func (d *dependency) ReconnectIfNeeded(ctx context.Context, force bool) (bool, error) {
	realVersionSource := d.sourceConnector.Version()
	// Быстрый путь: без force и без изменений source достаточно сверить версию dependent.
	if !force && realVersionSource == d.sourceVersion {
		realVersionDependent := d.dependentConnector.Version()
		if realVersionDependent == d.dependentVersion {
			return false, nil
		}
		d.dependentVersion = realVersionDependent

		return true, nil
	}
	d.sourceVersion = realVersionSource

	b, err := d.dependentConnector.ReconnectIfNeeded(ctx, true)
	d.dependentVersion = d.dependentConnector.Version()

	return b, err
}
