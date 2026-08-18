package group

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"errors"
)

var (
	ErrListEmpty          = errors.New("list is empty")
	ErrKindAlreadyCreated = errors.New("kind already created")
)

type listG struct {
	group
	list []intlDomain.Connector
}

func NewList(
	name string,
	mgr intlDomain.Manager,
	list ...domain.Kind,
) (domain.KindGroup, error) {
	if len(list) == 0 {
		return nil, ErrListEmpty
	}

	listStruct, kind, err := newGroup[listG](name, mgr)
	if err != nil {
		return nil, err
	}

	connectorMgr := mgr.ConnectorManager()
	listStruct.kind = kind
	listStruct.list = make([]intlDomain.Connector, 0, len(list))
	var conn intlDomain.Connector
	for _, k := range list {
		conn, err = connectorMgr.GetConnector(k)
		if err != nil {
			return nil, err
		}
		listStruct.list = append(listStruct.list, conn)
	}

	listStruct.version, err = listStruct.collectNestedVersions()
	if err != nil {
		return nil, err
	}
	err = connectorMgr.RegisterConnector(kind, listStruct)
	if err != nil {
		return nil, err
	}

	return kind, nil
}

func (l *listG) collectNestedVersions() (uint, error) {
	var (
		version uint
		conn    intlDomain.Connector
	)

	for _, conn = range l.list {
		version += conn.Version()
	}

	return version, nil
}

func (l *listG) forceRefreshConnector(ctx context.Context) (uint, error) {
	var (
		version uint
		conn    intlDomain.Connector
		err     error
	)

	for _, conn = range l.list {
		_, err = conn.ReconnectIfNeeded(ctx, true)
		if err != nil {
			return 0, err
		}
		version += conn.Version()
	}

	return version, nil
}

func (l *listG) ReconnectIfNeeded(ctx context.Context, force bool) (bool, error) {
	var (
		version uint
		err     error
	)
	if force {
		version, err = l.forceRefreshConnector(ctx)
	} else {
		version, err = l.collectNestedVersions()
	}
	if err != nil {
		return false, err
	}
	if l.version == version {
		return false, nil
	}

	l.version = version

	return true, nil
}
