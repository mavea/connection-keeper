package group

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
)

type neighbors struct {
	group
	list         []intlDomain.Connector
	listVersions []uint
}

func NewNeighbors(
	name string,
	mgr intlDomain.Manager,
	list ...domain.Kind,
) (domain.KindGroup, error) {
	if len(list) == 0 {
		return nil, ErrListEmpty
	}

	neighborSet, kind, err := newGroup[neighbors](name, mgr)
	if err != nil {
		return nil, err
	}

	connectorMgr := mgr.ConnectorManager()
	neighborSet.kind = kind
	neighborSet.list = make([]intlDomain.Connector, 0, len(list))
	neighborSet.listVersions = make([]uint, 0, len(list))
	var conn intlDomain.Connector
	for _, k := range list {
		conn, err = connectorMgr.GetConnector(k)
		if err != nil {
			return nil, err
		}
		neighborSet.list = append(neighborSet.list, conn)
		neighborSet.listVersions = append(neighborSet.listVersions, conn.Version())
	}

	neighborSet.version, err = neighborSet.collectNestedVersions()
	if err != nil {
		return nil, err
	}
	err = connectorMgr.RegisterConnector(kind, neighborSet)
	if err != nil {
		return nil, err
	}

	return kind, nil
}

func (n *neighbors) collectNestedVersions() (uint, error) {
	var (
		version uint
		v       uint
	)

	for _, v = range n.listVersions {
		version += v
	}

	return version, nil
}

func (n *neighbors) forceRefreshConnector(ctx context.Context, ignore domain.Kind) (uint, error) {
	var (
		version  uint
		v        uint
		conn     intlDomain.Connector
		err      error
		ignoreID = -1
		i        int
	)
	if ignore != nil {
		ignoreID = ignore.ID()
	}

	for i, conn = range n.list {
		if conn.Kind().ID() == ignoreID {
			n.listVersions[i] = conn.Version()
			version += n.listVersions[i]
			continue
		}
		v = conn.Version()
		if n.listVersions[i] != v {
			n.listVersions[i] = v
			version += n.listVersions[i]

			continue
		}
		_, err = conn.ReconnectIfNeeded(ctx, true)
		if err != nil {
			return 0, err
		}
		n.listVersions[i] = conn.Version()
		version += n.listVersions[i]
	}

	return version, nil
}
func (n *neighbors) refreshIfNeededConnector(ctx context.Context) (uint, error) {
	var (
		conn    intlDomain.Connector
		err     error
		updated bool
		i       int
		v       uint
	)
	for i, conn = range n.list {
		updated, err = conn.ReconnectIfNeeded(ctx, false)
		if err != nil {
			return 0, err
		}
		v = n.listVersions[i]
		n.listVersions[i] = conn.Version()
		if updated || v != n.listVersions[i] {
			return n.forceRefreshConnector(ctx, conn.Kind())
		}
	}

	return n.collectNestedVersions()
}
func (n *neighbors) ReconnectIfNeeded(ctx context.Context, force bool) (bool, error) {
	var (
		version uint
		err     error
	)
	if force {
		version, err = n.forceRefreshConnector(ctx, nil)
	} else {
		version, err = n.refreshIfNeededConnector(ctx)
	}
	if err != nil {
		return false, err
	}
	if n.version == version {
		return false, nil
	}

	n.version = version

	return true, nil
}
