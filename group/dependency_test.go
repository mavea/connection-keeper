package group

import (
	"context"
	"errors"
	"testing"
)

func TestDependencyReconnectIfNeededNoChangesReturnsFalse(t *testing.T) {
	t.Parallel()

	source := &testConnector{kind: testKind{id: 1, name: "source"}, version: 10}
	dependent := &testConnector{kind: testKind{id: 2, name: "dependent"}, version: 20}
	d := &dependency{
		sourceConnector:    source,
		dependentConnector: dependent,
		sourceVersion:      10,
		dependentVersion:   20,
	}

	updated, err := d.ReconnectIfNeeded(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated {
		t.Fatal("expected no update")
	}
	if len(dependent.reconnects) != 0 {
		t.Fatalf("dependent reconnect should not be called, got %v", dependent.reconnects)
	}
}

func TestDependencyReconnectIfNeededDependentVersionChangedReturnsTrue(t *testing.T) {
	t.Parallel()

	source := &testConnector{kind: testKind{id: 1, name: "source"}, version: 10}
	dependent := &testConnector{kind: testKind{id: 2, name: "dependent"}, version: 21}
	d := &dependency{
		sourceConnector:    source,
		dependentConnector: dependent,
		sourceVersion:      10,
		dependentVersion:   20,
	}

	updated, err := d.ReconnectIfNeeded(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected update")
	}
	if d.dependentVersion != 21 {
		t.Fatalf("dependentVersion = %d, want 21", d.dependentVersion)
	}
	if len(dependent.reconnects) != 0 {
		t.Fatalf("dependent reconnect should not be called, got %v", dependent.reconnects)
	}
}

func TestDependencyReconnectIfNeededSourceChangedForcesDependentReconnect(t *testing.T) {
	t.Parallel()

	source := &testConnector{kind: testKind{id: 1, name: "source"}, version: 11}
	dependent := &testConnector{kind: testKind{id: 2, name: "dependent"}, version: 20}
	dependent.reconnect = func(force bool) (bool, error) {
		if !force {
			t.Fatal("expected force=true for dependent reconnect")
		}
		dependent.version = 22
		return true, nil
	}

	d := &dependency{
		sourceConnector:    source,
		dependentConnector: dependent,
		sourceVersion:      10,
		dependentVersion:   20,
	}

	updated, err := d.ReconnectIfNeeded(context.Background(), false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected update")
	}
	if d.sourceVersion != 11 {
		t.Fatalf("sourceVersion = %d, want 11", d.sourceVersion)
	}
	if d.dependentVersion != 22 {
		t.Fatalf("dependentVersion = %d, want 22", d.dependentVersion)
	}
	if len(dependent.reconnects) != 1 || !dependent.reconnects[0] {
		t.Fatalf("expected one dependent reconnect with force=true, got %v", dependent.reconnects)
	}
}

func TestDependencyReconnectIfNeededForceTrueAlwaysReconnectsDependent(t *testing.T) {
	t.Parallel()

	source := &testConnector{kind: testKind{id: 1, name: "source"}, version: 10}
	dependent := &testConnector{kind: testKind{id: 2, name: "dependent"}, version: 20}
	dependent.reconnect = func(force bool) (bool, error) {
		if !force {
			t.Fatal("expected force=true for dependent reconnect")
		}
		dependent.version = 21
		return true, nil
	}

	d := &dependency{
		sourceConnector:    source,
		dependentConnector: dependent,
		sourceVersion:      10,
		dependentVersion:   20,
	}

	updated, err := d.ReconnectIfNeeded(context.Background(), true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated {
		t.Fatal("expected update")
	}
	if len(dependent.reconnects) != 1 || !dependent.reconnects[0] {
		t.Fatalf("expected one dependent reconnect with force=true, got %v", dependent.reconnects)
	}
}

func TestDependencyReconnectIfNeededPropagatesReconnectError(t *testing.T) {
	t.Parallel()

	reconnectErr := errors.New("reconnect failed")
	source := &testConnector{kind: testKind{id: 1, name: "source"}, version: 12}
	dependent := &testConnector{kind: testKind{id: 2, name: "dependent"}, version: 20}
	dependent.reconnect = func(force bool) (bool, error) {
		if !force {
			t.Fatal("expected force=true for dependent reconnect")
		}
		return false, reconnectErr
	}

	d := &dependency{
		sourceConnector:    source,
		dependentConnector: dependent,
		sourceVersion:      10,
		dependentVersion:   20,
	}

	updated, err := d.ReconnectIfNeeded(context.Background(), false)
	if !errors.Is(err, reconnectErr) {
		t.Fatalf("expected reconnect error, got %v", err)
	}
	if updated {
		t.Fatal("expected updated=false on reconnect error")
	}
}
