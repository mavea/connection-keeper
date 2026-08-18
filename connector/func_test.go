package connector

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	intlKind "connection-keeper/internal/kind"
	"context"
	"errors"
	"testing"
)

type testRegistryManager struct {
	registered []domain.Kind
}

func (m *testRegistryManager) RegisterGeneration(kind domain.Kind) {
	m.registered = append(m.registered, kind)
}
func (m *testRegistryManager) SetGeneration(domain.Kind, intlDomain.GenerationAny, any) {}
func (m *testRegistryManager) GetGeneration(domain.Kind) intlDomain.GenerationAny       { return nil }
func (m *testRegistryManager) GetGenerationAndCancelFunc(domain.Kind) (intlDomain.GenerationAny, any) {
	return nil, nil
}

type testConnectorManager struct {
	registerErr error

	registeredKind      domain.Kind
	registeredConnector intlDomain.Connector
	registerCalled      bool
	connectors          []intlDomain.Connector
}

func (m *testConnectorManager) RegisterConnector(kind domain.Kind, c intlDomain.Connector) error {
	m.registerCalled = true
	m.registeredKind = kind
	m.registeredConnector = c
	if m.registerErr != nil {
		return m.registerErr
	}
	m.connectors = append(m.connectors, c)
	return nil
}
func (m *testConnectorManager) GetConnector(kind domain.Kind) (intlDomain.Connector, error) {
	for _, c := range m.connectors {
		if c != nil && c.Kind().ID() == kind.ID() {
			return c, nil
		}
	}
	return nil, nil
}
func (m *testConnectorManager) List() []intlDomain.Connector {
	return append([]intlDomain.Connector(nil), m.connectors...)
}

type testDrainManager struct{}

func (m *testDrainManager) Register(intlDomain.GenerationAny, domain.DrainingFunc) {}
func (m *testDrainManager) DrainNext() error                                       { return nil }
func (m *testDrainManager) DrainAll() error                                        { return nil }

type testLogger struct{}

func (l testLogger) DebugContext(context.Context, string, ...any) {}
func (l testLogger) InfoContext(context.Context, string, ...any)  {}
func (l testLogger) WarnContext(context.Context, string, ...any)  {}
func (l testLogger) ErrorContext(context.Context, string, ...any) {}

type testManager struct {
	kindMgr      intlDomain.KindManager
	registryMgr  intlDomain.RegistryManager
	connectorMgr intlDomain.ConnectorManager
	drainMgr     intlDomain.DrainManager
	logger       domain.Logger
}

func (m *testManager) DrainManager() intlDomain.DrainManager           { return m.drainMgr }
func (m *testManager) KindManager() intlDomain.KindManager             { return m.kindMgr }
func (m *testManager) ConnectorManager() intlDomain.ConnectorManager   { return m.connectorMgr }
func (m *testManager) RegistryManager() intlDomain.RegistryManager     { return m.registryMgr }
func (m *testManager) Readiness() bool                                 { return true }
func (m *testManager) DisableReadiness() bool                          { return false }
func (m *testManager) EnableReadiness() bool                           { return true }
func (m *testManager) SetReadiness(bool) bool                          { return false }
func (m *testManager) SetLogger(domain.Logger) error                   { return nil }
func (m *testManager) Logger() domain.Logger                           { return m.logger }
func (m *testManager) Run(context.Context, intlDomain.RunConfig) error { return nil }
func (m *testManager) Shutdown() error                                 { return nil }

func newTestManager(kindMgr intlDomain.KindManager, registerErr error) (*testManager, *testRegistryManager, *testConnectorManager) {
	if kindMgr == nil {
		kindMgr = intlKind.NewKindManager()
	}
	registryMgr := &testRegistryManager{}
	connectorMgr := &testConnectorManager{registerErr: registerErr}
	mgr := &testManager{
		kindMgr:      kindMgr,
		registryMgr:  registryMgr,
		connectorMgr: connectorMgr,
		drainMgr:     &testDrainManager{},
		logger:       testLogger{},
	}
	return mgr, registryMgr, connectorMgr
}

func countKinds(t *testing.T, km intlDomain.KindManager) int {
	t.Helper()
	count := 0
	km.Kinds().Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

func TestNewInputSuccess(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, nil)

	gotKind, err := NewInput[int](context.Background(), "http", mgr,
		NewConfig(0, 0, 0, 0, 0, false, 0),
		nil,
		func(_ context.Context) (*int, domain.ReadinessFunc, domain.HandoverConnectionFunc[int], error) {
			v := 0
			return &v, func(context.Context) error { return nil }, nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind == nil {
		t.Fatal("expected non-nil kind")
	}
	if gotKind.String() != "http" {
		t.Fatalf("unexpected kind string: got %q, want %q", gotKind.String(), "http")
	}
	if gotKind.ID() < 0 {
		t.Fatalf("unexpected kind id: %d", gotKind.ID())
	}
	if len(registryMgr.registered) != 1 {
		t.Fatalf("registry registration mismatch: got %#v", registryMgr.registered)
	}
	if !connectorMgr.registerCalled {
		t.Fatal("connector registration was not called")
	}
	if len(connectorMgr.List()) != 1 {
		t.Fatalf("connector list length = %d, want 1", len(connectorMgr.List()))
	}
	if registryMgr.registered[0] != connectorMgr.registeredKind {
		t.Fatalf("registry/connector kinds mismatch: %v != %v", registryMgr.registered[0], connectorMgr.registeredKind)
	}
	if got := countKinds(t, kindMgr); got != 1 {
		t.Fatalf("kind manager count = %d, want 1", got)
	}
}

func TestNewInputDuplicateKind(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	if _, _, err := kindMgr.GetOrCreate("http"); err != nil {
		t.Fatalf("pre-create kind failed: %v", err)
	}
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, nil)

	gotKind, err := NewInput[int](context.Background(), "http", mgr, nil, nil, nil)
	if !errors.Is(err, intlKind.ErrKindAlreadyRegistered) {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind != nil {
		t.Fatalf("expected nil kind, got %v", gotKind)
	}
	if len(registryMgr.registered) != 0 {
		t.Fatalf("registry should not be touched, got %#v", registryMgr.registered)
	}
	if connectorMgr.registerCalled {
		t.Fatal("connector registration should not be called")
	}
}

func TestNewInputRegisterErrorRollbackDelete(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	errRegister := errors.New("register failed")
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, errRegister)

	gotKind, err := NewInput[int](context.Background(), "http", mgr, nil, nil, nil)
	if !errors.Is(err, errRegister) {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind != nil {
		t.Fatalf("expected nil kind, got %v", gotKind)
	}
	if len(registryMgr.registered) != 1 {
		t.Fatalf("registry registration mismatch: got %#v", registryMgr.registered)
	}
	if !connectorMgr.registerCalled {
		t.Fatal("connector registration was not called")
	}
	if got := countKinds(t, kindMgr); got != 0 {
		t.Fatalf("kind manager count after rollback = %d, want 0", got)
	}
}

func TestNewOutputSuccess(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, nil)

	gotKind, err := NewOutput[int](context.Background(), "pg", mgr,
		NewConfig(0, 0, 0, 0, 0, false, 0),
		nil,
		func(_ context.Context) (*int, domain.ReadinessFunc, domain.ShutdownConnectionFunc, error) {
			v := 0
			return &v, func(context.Context) error { return nil }, nil, nil
		},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind == nil {
		t.Fatal("expected non-nil kind")
	}
	if gotKind.String() != "pg" {
		t.Fatalf("unexpected kind string: got %q, want %q", gotKind.String(), "pg")
	}
	if gotKind.ID() < 0 {
		t.Fatalf("unexpected kind id: %d", gotKind.ID())
	}
	if len(registryMgr.registered) != 1 {
		t.Fatalf("registry registration mismatch: got %#v", registryMgr.registered)
	}
	if !connectorMgr.registerCalled {
		t.Fatal("connector registration was not called")
	}
	if len(connectorMgr.List()) != 1 {
		t.Fatalf("connector list length = %d, want 1", len(connectorMgr.List()))
	}
	if registryMgr.registered[0] != connectorMgr.registeredKind {
		t.Fatalf("registry/connector kinds mismatch: %v != %v", registryMgr.registered[0], connectorMgr.registeredKind)
	}
	if got := countKinds(t, kindMgr); got != 1 {
		t.Fatalf("kind manager count = %d, want 1", got)
	}
}

func TestNewOutputDuplicateKind(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	if _, _, err := kindMgr.GetOrCreate("pg"); err != nil {
		t.Fatalf("pre-create kind failed: %v", err)
	}
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, nil)

	gotKind, err := NewOutput[int](context.Background(), "pg", mgr, nil, nil, nil)
	if !errors.Is(err, intlKind.ErrKindAlreadyRegistered) {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind != nil {
		t.Fatalf("expected nil kind, got %v", gotKind)
	}
	if len(registryMgr.registered) != 0 {
		t.Fatalf("registry should not be touched, got %#v", registryMgr.registered)
	}
	if connectorMgr.registerCalled {
		t.Fatal("connector registration should not be called")
	}
}

func TestNewOutputRegisterErrorRollbackDelete(t *testing.T) {
	t.Parallel()

	kindMgr := intlKind.NewKindManager()
	errRegister := errors.New("register failed")
	mgr, registryMgr, connectorMgr := newTestManager(kindMgr, errRegister)

	gotKind, err := NewOutput[int](context.Background(), "pg", mgr, nil, nil, nil)
	if !errors.Is(err, errRegister) {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKind != nil {
		t.Fatalf("expected nil kind, got %v", gotKind)
	}
	if len(registryMgr.registered) != 1 {
		t.Fatalf("registry registration mismatch: got %#v", registryMgr.registered)
	}
	if !connectorMgr.registerCalled {
		t.Fatal("connector registration was not called")
	}
	if got := countKinds(t, kindMgr); got != 0 {
		t.Fatalf("kind manager count after rollback = %d, want 0", got)
	}
}
