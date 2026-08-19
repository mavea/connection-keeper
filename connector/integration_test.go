package connector

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	keepermanager "connection-keeper/manager"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type mockInputConn struct {
	id int
}

type noopTestLogger struct{}

func (noopTestLogger) DebugContext(context.Context, string, ...any) {}
func (noopTestLogger) InfoContext(context.Context, string, ...any)  {}
func (noopTestLogger) WarnContext(context.Context, string, ...any)  {}
func (noopTestLogger) ErrorContext(context.Context, string, ...any) {}

type integrationDrainConfig struct{}

func (integrationDrainConfig) CancelWaitTimeOut() time.Duration { return time.Millisecond }
func (integrationDrainConfig) MaxRetryWaitAttempts() uint8      { return 1 }

type lifecycleRecorder struct {
	createdIDs      []int
	handoverTargets []int
	drainedIDs      []int
}

type inputScenario struct {
	rec    *lifecycleRecorder
	failOn map[int]error
	calls  int
}

func (s *inputScenario) constructor(_ context.Context) (*mockInputConn, domain.ReadinessFunc, domain.HandoverConnectionFunc[mockInputConn], error) {
	s.calls++
	call := s.calls
	if err, ok := s.failOn[call]; ok {
		return nil, nil, nil, err
	}

	conn := &mockInputConn{id: call}
	s.rec.createdIDs = append(s.rec.createdIDs, conn.id)

	return conn,
		func(context.Context) error { return nil },
		func(_ context.Context, next *mockInputConn) (domain.DrainingFunc, error) {
			if next == nil {
				s.rec.handoverTargets = append(s.rec.handoverTargets, -1)
			} else {
				s.rec.handoverTargets = append(s.rec.handoverTargets, next.id)
			}

			return func() error {
				s.rec.drainedIDs = append(s.rec.drainedIDs, conn.id)
				return nil
			}, nil
		},
		nil
}

func newInputIntegrationHarness(
	t *testing.T,
	rec *lifecycleRecorder,
	invalidate domain.ConnectionInvalidationFunc,
	failOn map[int]error,
) (intlDomain.Manager, domain.KindType[mockInputConn], intlDomain.Connector, intlDomain.Generation[mockInputConn]) {
	t.Helper()

	mgr := keepermanager.New(context.Background(), integrationDrainConfig{})
	if err := mgr.SetLogger(noopTestLogger{}); err != nil {
		t.Fatalf("set logger: %v", err)
	}
	scenario := &inputScenario{rec: rec, failOn: failOn}

	kind, err := NewInput[mockInputConn](
		context.Background(),
		"http",
		mgr,
		NewConfig(1, time.Millisecond, 0, 1, 0, false, 0),
		invalidate,
		scenario.constructor,
	)
	if err != nil {
		t.Fatalf("new input connector: %v", err)
	}

	connector, err := mgr.ConnectorManager().GetConnector(kind)
	if err != nil {
		t.Fatalf("get connector: %v", err)
	}

	gen := currentGeneration(t, mgr, kind)

	return mgr, kind, connector, gen
}

func newInvalidationSequence(values ...bool) domain.ConnectionInvalidationFunc {
	calls := 0
	return func() (bool, error) {
		if len(values) == 0 {
			return false, nil
		}
		if calls >= len(values) {
			return values[len(values)-1], nil
		}
		value := values[calls]
		calls++
		return value, nil
	}
}

func currentGeneration(t *testing.T, mgr intlDomain.Manager, kind domain.Kind) intlDomain.Generation[mockInputConn] {
	t.Helper()

	genAny := mgr.RegistryManager().GetGeneration(kind)
	if genAny == nil {
		t.Fatal("expected current generation")
	}

	gen, ok := genAny.(intlDomain.Generation[mockInputConn])
	if !ok {
		t.Fatalf("unexpected generation type: %T", genAny)
	}

	return gen
}

func openRequests(t *testing.T, gen intlDomain.Generation[mockInputConn], wantID int, count int) []domain.CancelFunc {
	t.Helper()

	releases := make([]domain.CancelFunc, 0, count)
	for i := 0; i < count; i++ {
		conn, release := gen.Conn()
		if conn == nil {
			t.Fatal("expected non-nil connection")
		}
		if conn.id != wantID {
			t.Fatalf("connection id = %d, want %d", conn.id, wantID)
		}
		releases = append(releases, release)
	}

	return releases
}

func releaseAll(t *testing.T, releases []domain.CancelFunc) {
	t.Helper()

	for i, release := range releases {
		if err := release(); err != nil {
			t.Fatalf("release #%d failed: %v", i+1, err)
		}
	}
}

func waitForClose(t *testing.T, gen intlDomain.Generation[mockInputConn]) {
	t.Helper()

	select {
	case <-gen.WaitForClose():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("generation did not close in time")
	}
}

func assertNoCloseYet(t *testing.T, gen intlDomain.Generation[mockInputConn]) {
	t.Helper()

	select {
	case <-gen.WaitForClose():
		t.Fatal("generation closed earlier than expected")
	default:
	}
}

func TestInputLifecycleGracefulAfterMultipleRequests(t *testing.T) {
	rec := &lifecycleRecorder{}
	mgr, _, con, gen := newInputIntegrationHarness(t, rec, newInvalidationSequence(false), nil)
	if gen.Version() != 1 {
		t.Fatalf("version = %d, want 1", gen.Version())
	}
	if got := gen.Refs(); got != 1 {
		t.Fatalf("refs after install = %d, want 1", got)
	}

	releases := openRequests(t, gen, 1, 3)
	if got := gen.Refs(); got != 4 {
		t.Fatalf("refs after 3 requests = %d, want 4", got)
	}
	releaseAll(t, releases)
	if got := gen.Refs(); got != 1 {
		t.Fatalf("refs after request release = %d, want 1", got)
	}

	if err := con.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if got := gen.Refs(); got != 0 {
		t.Fatalf("refs after close = %d, want 0", got)
	}
	waitForClose(t, gen)

	if !reflect.DeepEqual(rec.createdIDs, []int{1}) {
		t.Fatalf("created ids = %v, want %v", rec.createdIDs, []int{1})
	}
	if !reflect.DeepEqual(rec.handoverTargets, []int{-1}) {
		t.Fatalf("handover targets = %v, want %v", rec.handoverTargets, []int{-1})
	}
	if err := mgr.DrainManager().DrainAll(); err != nil {
		t.Fatalf("drain all failed: %v", err)
	}
	if !reflect.DeepEqual(rec.drainedIDs, []int{1}) {
		t.Fatalf("drained ids = %v, want %v", rec.drainedIDs, []int{1})
	}
}

func TestInputLifecycleFatalReconnectLeavesGenerationGraceful(t *testing.T) {
	rec := &lifecycleRecorder{}
	fatalErr := errors.New("constructor failed")
	mgr, kind, con, gen := newInputIntegrationHarness(t, rec, newInvalidationSequence(true), map[int]error{2: fatalErr})
	releases := openRequests(t, gen, 1, 2)
	if got := gen.Refs(); got != 3 {
		t.Fatalf("refs after 2 requests = %d, want 3", got)
	}

	if updated, err := con.ReconnectIfNeeded(context.Background(), false); !errors.Is(err, fatalErr) || updated {
		t.Fatalf("fatal reconnect = (%v, %v), want (false, fatalErr)", updated, err)
	}
	if got := currentGeneration(t, mgr, kind); got != gen {
		t.Fatal("generation changed after fatal reconnect")
	}
	if got := gen.Refs(); got != 3 {
		t.Fatalf("refs after fatal reconnect = %d, want 3", got)
	}
	if !reflect.DeepEqual(rec.createdIDs, []int{1}) {
		t.Fatalf("created ids = %v, want %v", rec.createdIDs, []int{1})
	}
	if len(rec.handoverTargets) != 0 {
		t.Fatalf("handover targets after fatal reconnect = %v, want empty", rec.handoverTargets)
	}

	releaseAll(t, releases)
	if got := gen.Refs(); got != 1 {
		t.Fatalf("refs after request release = %d, want 1", got)
	}
	if err := con.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	waitForClose(t, gen)
	if err := mgr.DrainManager().DrainAll(); err != nil {
		t.Fatalf("drain all failed: %v", err)
	}
	if !reflect.DeepEqual(rec.handoverTargets, []int{-1}) {
		t.Fatalf("handover targets = %v, want %v", rec.handoverTargets, []int{-1})
	}
	if !reflect.DeepEqual(rec.drainedIDs, []int{1}) {
		t.Fatalf("drained ids = %v, want %v", rec.drainedIDs, []int{1})
	}
}

func TestInputLifecycleUpdateOnceKeepsRequestsAndDrainsOldGeneration(t *testing.T) {
	rec := &lifecycleRecorder{}
	mgr, kind, con, gen1 := newInputIntegrationHarness(t, rec, newInvalidationSequence(true), nil)
	req1 := openRequests(t, gen1, 1, 2)
	if got := gen1.Refs(); got != 3 {
		t.Fatalf("gen1 refs after 2 requests = %d, want 3", got)
	}
	if err := req1[0](); err != nil {
		t.Fatalf("first request release failed: %v", err)
	}
	if got := gen1.Refs(); got != 2 {
		t.Fatalf("gen1 refs after releasing one request = %d, want 2", got)
	}

	if updated, err := con.ReconnectIfNeeded(context.Background(), false); err != nil || !updated {
		t.Fatalf("update reconnect = (%v, %v), want (true, nil)", updated, err)
	}
	gen2 := currentGeneration(t, mgr, kind)
	if gen2 == gen1 {
		t.Fatal("expected a new generation after update")
	}
	if got := gen1.Refs(); got != 1 {
		t.Fatalf("gen1 refs after update = %d, want 1", got)
	}
	if got := gen2.Refs(); got != 1 {
		t.Fatalf("gen2 refs after install = %d, want 1", got)
	}
	assertNoCloseYet(t, gen1)

	req2 := openRequests(t, gen2, 2, 2)
	if got := gen2.Refs(); got != 3 {
		t.Fatalf("gen2 refs after 2 requests = %d, want 3", got)
	}
	releaseAll(t, req2)
	if got := gen2.Refs(); got != 1 {
		t.Fatalf("gen2 refs after request release = %d, want 1", got)
	}
	if err := req1[1](); err != nil {
		t.Fatalf("second request release on gen1 failed: %v", err)
	}
	waitForClose(t, gen1)

	if err := con.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if got := gen2.Refs(); got != 0 {
		t.Fatalf("gen2 refs after close = %d, want 0", got)
	}
	waitForClose(t, gen2)
	if err := mgr.DrainManager().DrainAll(); err != nil {
		t.Fatalf("drain all failed: %v", err)
	}

	if !reflect.DeepEqual(rec.createdIDs, []int{1, 2}) {
		t.Fatalf("created ids = %v, want %v", rec.createdIDs, []int{1, 2})
	}
	if !reflect.DeepEqual(rec.handoverTargets, []int{2, -1}) {
		t.Fatalf("handover targets = %v, want %v", rec.handoverTargets, []int{2, -1})
	}
	if !reflect.DeepEqual(rec.drainedIDs, []int{1, 2}) {
		t.Fatalf("drained ids = %v, want %v", rec.drainedIDs, []int{1, 2})
	}
}

func TestInputLifecycleTwoUpdatesDrainGenerationsInOrder(t *testing.T) {
	rec := &lifecycleRecorder{}
	mgr, kind, con, gen1 := newInputIntegrationHarness(t, rec, newInvalidationSequence(true, true), nil)
	req1 := openRequests(t, gen1, 1, 2)
	if err := req1[0](); err != nil {
		t.Fatalf("gen1 request release failed: %v", err)
	}

	if updated, err := con.ReconnectIfNeeded(context.Background(), false); err != nil || !updated {
		t.Fatalf("first update reconnect = (%v, %v), want (true, nil)", updated, err)
	}
	gen2 := currentGeneration(t, mgr, kind)
	if gen2 == gen1 {
		t.Fatal("expected a new generation after first update")
	}
	if got := gen1.Refs(); got != 1 {
		t.Fatalf("gen1 refs after first update = %d, want 1", got)
	}

	req2 := openRequests(t, gen2, 2, 2)
	if err := req2[0](); err != nil {
		t.Fatalf("gen2 request release failed: %v", err)
	}

	if updated, err := con.ReconnectIfNeeded(context.Background(), false); err != nil || !updated {
		t.Fatalf("second update reconnect = (%v, %v), want (true, nil)", updated, err)
	}
	gen3 := currentGeneration(t, mgr, kind)
	if gen3 == gen2 {
		t.Fatal("expected a new generation after second update")
	}
	if got := gen2.Refs(); got != 1 {
		t.Fatalf("gen2 refs after second update = %d, want 1", got)
	}

	req3 := openRequests(t, gen3, 3, 2)
	if got := gen3.Refs(); got != 3 {
		t.Fatalf("gen3 refs after 2 requests = %d, want 3", got)
	}
	releaseAll(t, req3)
	if got := gen3.Refs(); got != 1 {
		t.Fatalf("gen3 refs after request release = %d, want 1", got)
	}

	if err := req1[1](); err != nil {
		t.Fatalf("gen1 second request release failed: %v", err)
	}
	waitForClose(t, gen1)
	if err := req2[1](); err != nil {
		t.Fatalf("gen2 second request release failed: %v", err)
	}
	waitForClose(t, gen2)

	if err := con.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}
	if got := gen3.Refs(); got != 0 {
		t.Fatalf("gen3 refs after close = %d, want 0", got)
	}
	waitForClose(t, gen3)
	if err := mgr.DrainManager().DrainAll(); err != nil {
		t.Fatalf("drain all failed: %v", err)
	}

	if !reflect.DeepEqual(rec.createdIDs, []int{1, 2, 3}) {
		t.Fatalf("created ids = %v, want %v", rec.createdIDs, []int{1, 2, 3})
	}
	if !reflect.DeepEqual(rec.handoverTargets, []int{2, 3, -1}) {
		t.Fatalf("handover targets = %v, want %v", rec.handoverTargets, []int{2, 3, -1})
	}
	if !reflect.DeepEqual(rec.drainedIDs, []int{1, 2, 3}) {
		t.Fatalf("drained ids = %v, want %v", rec.drainedIDs, []int{1, 2, 3})
	}
}
