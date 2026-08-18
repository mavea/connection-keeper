package manager

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"errors"
	"testing"
	"time"
)

type testRunConfig struct {
	checkInterval    time.Duration
	disableReadiness time.Duration
	stopTimeout      time.Duration
}

func (c testRunConfig) ConnectionCheckInterval() time.Duration { return c.checkInterval }
func (c testRunConfig) DisableReadinessTimeout() time.Duration { return c.disableReadiness }
func (c testRunConfig) StopTimeout() time.Duration             { return c.stopTimeout }

// testKind реализует domain.Kind для unit-тестов менеджера.
type testKind struct{ id int }

func (k testKind) ID() int        { return k.id }
func (k testKind) String() string { return "kind" }

type testConnector struct {
	kind     domain.Kind
	closeErr error
}

func (c testConnector) ReconnectIfNeeded(context.Context, bool) (bool, error) { return false, nil }
func (c testConnector) Version() uint                                         { return 0 }
func (c testConnector) Kind() domain.Kind                                     { return c.kind }
func (c testConnector) Close() error                                          { return c.closeErr }

type testConnectorManager struct {
	list []intlDomain.Connector
}

func (m testConnectorManager) RegisterConnector(domain.Kind, intlDomain.Connector) error { return nil }
func (m testConnectorManager) GetConnector(domain.Kind) (intlDomain.Connector, error) {
	return nil, nil
}
func (m testConnectorManager) List() []intlDomain.Connector { return m.list }

type testDrainManager struct {
	drainAllErr error
}

func (m testDrainManager) Register(intlDomain.GenerationAny, domain.DrainingFunc) {}
func (m testDrainManager) DrainNext() error                                       { return nil }
func (m testDrainManager) DrainAll() error                                        { return m.drainAllErr }

func TestManagerStopReturnsNilWhenNotRunning(t *testing.T) {
	t.Parallel()

	mgr := &manager{conf: testRunConfig{disableReadiness: 10 * time.Millisecond}}
	if err := mgr.stop(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestManagerStopReturnsTimeoutWhenDoneNotClosed(t *testing.T) {
	t.Parallel()

	mgr := &manager{
		conf: testRunConfig{disableReadiness: 10 * time.Millisecond},
		done: make(chan struct{}),
	}
	mgr.runCancel = func() {}

	err := mgr.stop()
	if !errors.Is(err, ErrManagerIsNotStopped) {
		t.Fatalf("expected ErrManagerIsNotStopped, got %v", err)
	}
}

func TestManagerStopReturnsNilWhenDoneIsClosed(t *testing.T) {
	t.Parallel()

	done := make(chan struct{})
	mgr := &manager{
		conf: testRunConfig{disableReadiness: 20 * time.Millisecond, stopTimeout: 200 * time.Millisecond},
		done: done,
	}
	mgr.runCancel = func() { close(done) }

	if err := mgr.stop(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestManagerShutdownDisableReadinessReturnsCtxCancel(t *testing.T) {
	t.Parallel()

	runCtx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr := &manager{
		run:  runCtx,
		conf: testRunConfig{disableReadiness: 20 * time.Millisecond},
	}

	err := mgr.shutdownDisableReadiness()
	if !errors.Is(err, ErrCtxCancel) {
		t.Fatalf("expected ErrCtxCancel, got %v", err)
	}
}

func TestManagerShutdownDisableReadinessReturnsNilOnTimeout(t *testing.T) {
	t.Parallel()

	runCtx := context.Background()
	mgr := &manager{
		run:  runCtx,
		conf: testRunConfig{disableReadiness: 10 * time.Millisecond},
	}
	mgr.EnableReadiness()

	err := mgr.shutdownDisableReadiness()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if mgr.Readiness() {
		t.Fatal("expected readiness=false after shutdownDisableReadiness")
	}
}

func TestManagerShutdownCollectsConnectorAndDrainErrors(t *testing.T) {
	t.Parallel()

	connectorErr := errors.New("connector close error")
	drainErr := errors.New("drain all error")
	mgr := &manager{
		connectorMgr: testConnectorManager{list: []intlDomain.Connector{testConnector{kind: testKind{id: 1}, closeErr: connectorErr}}},
		drainMgr:     testDrainManager{drainAllErr: drainErr},
	}

	err := mgr.shutdown()
	if !errors.Is(err, connectorErr) {
		t.Fatalf("expected connector error in result, got %v", err)
	}
	if !errors.Is(err, drainErr) {
		t.Fatalf("expected drain error in result, got %v", err)
	}
}
