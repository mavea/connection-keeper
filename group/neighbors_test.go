package group

import (
	"connection-keeper/internal/domain"
	"context"
	"errors"
	"testing"
)

func TestNeighborsReconnectIfNeededNoUpdatesReturnsFalse(t *testing.T) {
	t.Parallel()

	k1 := testKind{id: 1, name: "c1"}
	k2 := testKind{id: 2, name: "c2"}
	c1 := &testConnector{kind: k1, version: 1}
	c2 := &testConnector{kind: k2, version: 2}

	n := &neighbors{
		list:         []domain.Connector{c1, c2},
		listVersions: []uint{1, 2},
	}
	n.version = 3

	updated, err := n.ReconnectIfNeeded(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated {
		t.Fatal("expected no updates")
	}
	if len(c1.reconnects) != 1 || c1.reconnects[0] {
		t.Fatalf("expected one reconnect(false) call for c1, got %v", c1.reconnects)
	}
	if len(c2.reconnects) != 1 || c2.reconnects[0] {
		t.Fatalf("expected one reconnect(false) call for c2, got %v", c2.reconnects)
	}
}

func TestNeighborsReconnectIfNeededNonForceRefreshesAllOnAnyChange(t *testing.T) {
	t.Parallel()

	k1 := testKind{id: 1, name: "c1"}
	k2 := testKind{id: 2, name: "c2"}
	c1 := &testConnector{kind: k1, version: 1}
	c1.reconnect = func(force bool) (bool, error) {
		if force {
			c1.version = 2
			return true, nil
		}
		return false, nil
	}
	c2 := &testConnector{kind: k2, version: 1}
	c2.reconnect = func(force bool) (bool, error) {
		if force {
			return false, nil
		}
		return true, nil
	}

	n := &neighbors{
		list:         []domain.Connector{c1, c2},
		listVersions: []uint{1, 1},
	}
	n.version = 2

	updated, err := n.ReconnectIfNeeded(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected update")
	}
	if n.version != 3 {
		t.Fatalf("neighbors version = %d, want 3", n.version)
	}
	if len(c1.reconnects) < 2 || c1.reconnects[0] || !c1.reconnects[1] {
		t.Fatalf("expected c1 reconnect sequence [false,true], got %v", c1.reconnects)
	}
	if len(c2.reconnects) != 1 || c2.reconnects[0] {
		t.Fatalf("expected one reconnect(false) call for c2, got %v", c2.reconnects)
	}
}

func TestNeighborsReconnectIfNeededPropagatesErrorFromRefreshCheck(t *testing.T) {
	t.Parallel()

	reconnectErr := errors.New("check failed")
	k1 := testKind{id: 1, name: "c1"}
	c1 := &testConnector{kind: k1, version: 1}
	c1.reconnect = func(force bool) (bool, error) {
		if force {
			return false, nil
		}
		return false, reconnectErr
	}

	n := &neighbors{
		list:         []domain.Connector{c1},
		listVersions: []uint{1},
	}
	n.version = 1

	updated, err := n.ReconnectIfNeeded(context.Background(), false)
	if !errors.Is(err, reconnectErr) {
		t.Fatalf("expected reconnect error, got %v", err)
	}
	if updated {
		t.Fatal("expected updated=false on error")
	}
}

func TestNeighborsReconnectIfNeededForcePropagatesError(t *testing.T) {
	t.Parallel()

	reconnectErr := errors.New("force failed")
	k1 := testKind{id: 1, name: "c1"}
	c1 := &testConnector{kind: k1, version: 1}
	c1.reconnect = func(force bool) (bool, error) {
		if !force {
			return false, nil
		}
		return false, reconnectErr
	}

	n := &neighbors{
		list:         []domain.Connector{c1},
		listVersions: []uint{1},
	}
	n.version = 1

	updated, err := n.ReconnectIfNeeded(context.Background(), true)
	if !errors.Is(err, reconnectErr) {
		t.Fatalf("expected reconnect error, got %v", err)
	}
	if updated {
		t.Fatal("expected updated=false on error")
	}
	if len(c1.reconnects) != 1 || !c1.reconnects[0] {
		t.Fatalf("expected one reconnect(true) call, got %v", c1.reconnects)
	}
}
