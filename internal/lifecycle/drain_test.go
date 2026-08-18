package lifecycle

import (
	"connection-keeper/domain"
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"
)

type testDrainConfig struct {
	cancelTimeout time.Duration
	maxAttempts   uint8
}

func (c testDrainConfig) CancelWaitTimeOut() time.Duration {
	return c.cancelTimeout
}

func (c testDrainConfig) MaxRetryWaitAttempts() uint8 {
	return c.maxAttempts
}

type testGeneration struct {
	closed chan struct{}
}

func newTestGeneration(closed bool) *testGeneration {
	g := &testGeneration{closed: make(chan struct{})}
	if closed {
		close(g.closed)
	}

	return g
}

func (g *testGeneration) Any() (any, domain.CancelFunc) {
	return nil, func() error { return nil }
}

func (g *testGeneration) Version() uint {
	return 1
}

func (g *testGeneration) Refs() int64 {
	return 0
}

func (g *testGeneration) Readiness(context.Context) error {
	return nil
}

func (g *testGeneration) WaitForClose() <-chan struct{} {
	return g.closed
}

func TestDrainNext_EmptyQueue(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 1})
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
}

func TestDrainNext_ClosedGenerationCallsDrainOnce(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 1})
	g := newTestGeneration(true)

	var calls int32
	d.Register(g, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	if err := d.DrainNext(); err != nil {
		t.Fatalf("первый DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("число вызовов drain = %d, ожидается 1", got)
	}

	if err := d.DrainNext(); err != nil {
		t.Fatalf("второй DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("число вызовов drain после второго вызова = %d, ожидается 1", got)
	}
}

func TestDrainNext_TimeoutRetriesThenDrains(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 2})
	g := newTestGeneration(false)

	var calls int32
	d.Register(g, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	if err := d.DrainNext(); err != nil {
		t.Fatalf("первый DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Fatalf("число вызовов drain после первого таймаута = %d, ожидается 0", got)
	}

	if err := d.DrainNext(); err != nil {
		t.Fatalf("второй DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("число вызовов drain после достижения лимита = %d, ожидается 1", got)
	}

	if err := d.DrainNext(); err != nil {
		t.Fatalf("третий DrainNext() вернул ошибку = %v, ожидается nil", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("число вызовов drain после удаления = %d, ожидается 1", got)
	}
}

func TestDrainAll_JoinsDrainErrors(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 1})
	g1 := newTestGeneration(true)
	g2 := newTestGeneration(true)

	errOne := errors.New("drain one")
	errTwo := errors.New("drain two")

	// Функции возвращают ошибку только при первом вызове, при повторных — nil.
	// Это позволяет DrainAll удалить элемент из очереди и завершить цикл,
	// сохраняя при этом проверку на объединение ошибок.
	var calls1, calls2 int32
	d.Register(g1, func() error {
		if atomic.AddInt32(&calls1, 1) == 1 {
			return errOne
		}
		return nil
	})
	d.Register(g2, func() error {
		if atomic.AddInt32(&calls2, 1) == 1 {
			return errTwo
		}
		return nil
	})

	err := d.DrainAll()
	if err == nil {
		t.Fatal("DrainAll() вернул nil, ожидается объединённая ошибка")
	}
	if !errors.Is(err, errOne) {
		t.Fatalf("ошибка DrainAll() не содержит errOne: %v", err)
	}
	if !errors.Is(err, errTwo) {
		t.Fatalf("ошибка DrainAll() не содержит errTwo: %v", err)
	}
}

func TestDrainNext_PreservesOrderForClosedGenerations(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 1})

	var drained []int
	d.Register(newTestGeneration(true), func() error {
		drained = append(drained, 1)
		return nil
	})
	d.Register(newTestGeneration(true), func() error {
		drained = append(drained, 2)
		return nil
	})
	d.Register(newTestGeneration(true), func() error {
		drained = append(drained, 3)
		return nil
	})

	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #1 вернул ошибку = %v, ожидается nil", err)
	}
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #2 вернул ошибку = %v, ожидается nil", err)
	}
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #3 вернул ошибку = %v, ожидается nil", err)
	}

	want := []int{1, 2, 3}
	if !reflect.DeepEqual(drained, want) {
		t.Fatalf("порядок дренажа = %v, ожидается %v", drained, want)
	}
}

func TestDrainNext_NoSkipAfterTimeoutAndRemoval(t *testing.T) {
	d := NewDrainManager(testDrainConfig{cancelTimeout: time.Millisecond, maxAttempts: 2})

	var drained []int
	d.Register(newTestGeneration(false), func() error {
		drained = append(drained, 1)
		return nil
	})
	d.Register(newTestGeneration(true), func() error {
		drained = append(drained, 2)
		return nil
	})
	d.Register(newTestGeneration(true), func() error {
		drained = append(drained, 3)
		return nil
	})

	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #1 вернул ошибку = %v, ожидается nil", err)
	}
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #2 вернул ошибку = %v, ожидается nil", err)
	}
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #3 вернул ошибку = %v, ожидается nil", err)
	}
	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext #4 вернул ошибку = %v, ожидается nil", err)
	}

	want := []int{2, 3, 1}
	if !reflect.DeepEqual(drained, want) {
		t.Fatalf("порядок дренажа с ретраями = %v, ожидается %v", drained, want)
	}

	if err := d.DrainNext(); err != nil {
		t.Fatalf("DrainNext после пустой очереди вернул ошибку = %v, ожидается nil", err)
	}
}



