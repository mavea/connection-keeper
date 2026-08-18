package group

import (
	"connection-keeper/domain"
	"context"
)

type testKind struct {
	id   int
	name string
}

func (k testKind) ID() int        { return k.id }
func (k testKind) String() string { return k.name }

type testConnector struct {
	kind       domain.Kind
	version    uint
	closeErr   error
	reconnects []bool
	reconnect  func(force bool) (bool, error)
}

func (c *testConnector) ReconnectIfNeeded(_ context.Context, force bool) (bool, error) {
	c.reconnects = append(c.reconnects, force)
	if c.reconnect != nil {
		return c.reconnect(force)
	}
	return false, nil
}

func (c *testConnector) Version() uint {
	return c.version
}

func (c *testConnector) Kind() domain.Kind {
	return c.kind
}

func (c *testConnector) Close() error {
	return c.closeErr
}
