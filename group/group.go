package group

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	intlKind "connection-keeper/internal/kind"
)

type group struct {
	kind domain.Kind

	version uint
}

func newGroup[G any](
	name string,
	mgr intlDomain.Manager,
) (*G, domain.KindGroup, error) {
	var (
		groupStruct G
		kindMgr     = mgr.KindManager()
		kind, err   = intlKind.NewKind[group](name, kindMgr)
		registryMgr = mgr.RegistryManager()
	)
	if err != nil {
		return nil, nil, err
	}
	registryMgr.RegisterGeneration(kind)

	return &groupStruct, kind, nil
}

func (g *group) Version() uint {
	return g.version
}

func (g *group) Kind() domain.Kind {
	return g.kind
}

func (g *group) Close() error {
	return nil
}
