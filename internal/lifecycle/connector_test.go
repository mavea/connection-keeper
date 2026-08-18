package lifecycle

import (
	"connection-keeper/domain"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

type connectorTestKind struct {
	id int
}

func (k connectorTestKind) ID() int {
	return k.id
}

func (k connectorTestKind) String() string {
	return fmt.Sprintf("kind-%d", k.id)
}

type connectorTestConnector struct {
	kind domain.Kind
}

func (c connectorTestConnector) ReconnectIfNeeded(context.Context, bool) (bool, error) {
	return false, nil
}

func (c connectorTestConnector) Version() uint {
	return 1
}

func (c connectorTestConnector) Kind() domain.Kind {
	return c.kind
}

func (c connectorTestConnector) Close() error {
	return nil
}

func TestConnectorRegisterAndGet(t *testing.T) {
	mgr := NewConnectorManager()
	kind := connectorTestKind{id: 1}
	con := connectorTestConnector{kind: kind}

	if err := mgr.RegisterConnector(kind, con); err != nil {
		t.Fatalf("RegisterConnector вернул ошибку = %v, ожидается nil", err)
	}

	got, err := mgr.GetConnector(kind)
	if err != nil {
		t.Fatalf("GetConnector вернул ошибку = %v, ожидается nil", err)
	}
	if got == nil {
		t.Fatal("GetConnector вернул nil, ожидается коннектор")
	}
	if got.Kind().ID() != kind.ID() {
		t.Fatalf("ID коннектора = %d, ожидается %d", got.Kind().ID(), kind.ID())
	}
}

func TestConnectorRegisterSameKindConcurrent(t *testing.T) {
	mgr := NewConnectorManager()
	kind := connectorTestKind{id: 2}
	const workers = 16

	var okCount atomic.Int32
	var dupCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := mgr.RegisterConnector(kind, connectorTestConnector{kind: kind})
			switch {
			case err == nil:
				okCount.Add(1)
			case errors.Is(err, ErrFabricAlreadyRegistered):
				dupCount.Add(1)
			default:
				t.Errorf("неожиданная ошибка RegisterConnector: %v", err)
			}
		}()
	}
	wg.Wait()

	if got := okCount.Load(); got != 1 {
		t.Fatalf("успешных регистраций = %d, ожидается 1", got)
	}
	if got := dupCount.Load(); got != workers-1 {
		t.Fatalf("дубликатов = %d, ожидается %d", got, workers-1)
	}
}

func TestConnectorRegisterDifferentKindsConcurrent(t *testing.T) {
	mgr := NewConnectorManager()
	const workers = 32

	var wg sync.WaitGroup
	for i := 1; i <= workers; i++ {
		id := i
		kind := connectorTestKind{id: id}
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := mgr.RegisterConnector(kind, connectorTestConnector{kind: kind})
			if err != nil {
				t.Errorf("RegisterConnector id=%d вернул ошибку: %v", id, err)
			}
		}()
	}
	wg.Wait()

	for i := 1; i <= workers; i++ {
		kind := connectorTestKind{id: i}
		con, err := mgr.GetConnector(kind)
		if err != nil {
			t.Fatalf("GetConnector id=%d вернул ошибку: %v", i, err)
		}
		if con == nil {
			t.Fatalf("GetConnector id=%d вернул nil", i)
		}
	}
}

func TestConnectorGetUnknownKind(t *testing.T) {
	mgr := NewConnectorManager()
	_, err := mgr.GetConnector(connectorTestKind{id: 100})
	if !errors.Is(err, ErrFabricConstructorNotFound) {
		t.Fatalf("ошибка = %v, ожидается ErrFabricConstructorNotFound", err)
	}
}

