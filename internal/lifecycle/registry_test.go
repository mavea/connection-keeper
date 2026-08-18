package lifecycle

import (
	"connection-keeper/domain"
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
)

type registryTestKind struct {
	id int
}

func (k registryTestKind) ID() int {
	return k.id
}

func (k registryTestKind) String() string {
	return fmt.Sprintf("kind-%d", k.id)
}

type registryTestGeneration struct {
	version uint
}

func (g registryTestGeneration) Any() (any, domain.CancelFunc) {
	return g, func() error { return nil }
}

func (g registryTestGeneration) Version() uint {
	return g.version
}

func (g registryTestGeneration) Refs() int64 {
	return 0
}

func (g registryTestGeneration) Readiness(context.Context) error {
	return nil
}

func (g registryTestGeneration) WaitForClose() <-chan struct{} {
	ch := make(chan struct{})
	return ch
}

func TestRegistryStoresAndLoadsConsistentPair(t *testing.T) {
	r := NewRegistryManager().(*registry)
	kind := registryTestKind{id: 1}

	r.RegisterGeneration(kind)
	gen := registryTestGeneration{version: 7}
	var drainCalls int32
	drainFunc := func() int {
		return int(atomic.AddInt32(&drainCalls, 1))
	}

	r.SetGeneration(kind, gen, drainFunc)

	loadedGen, loadedClose := r.GetGenerationAndCancelFunc(kind)
	if loadedGen == nil {
		t.Fatal("ожидалось поколение, но получен nil")
	}
	if got := loadedGen.(registryTestGeneration).Version(); got != gen.Version() {
		t.Fatalf("версия поколения = %d, ожидается %d", got, gen.Version())
	}
	if loadedClose == nil {
		t.Fatal("ожидалась функция дренажа, но получен nil")
	}
	if got := loadedClose.(func() int)(); got != 1 {
		t.Fatalf("результат callback = %d, ожидается 1", got)
	}

	if got := r.GetGeneration(kind); got == nil {
		t.Fatal("GetGeneration вернул nil, ожидалось поколение")
	}
}

func TestRegistryConcurrentReadersSeeConsistentPair(t *testing.T) {
	r := NewRegistryManager().(*registry)
	kind := registryTestKind{id: 2}
	r.RegisterGeneration(kind)
	r.SetGeneration(kind, registryTestGeneration{version: 0}, func() int { return 0 })

	const readers = 8
	const updates = 5000

	var started sync.WaitGroup
	started.Add(readers)
	var stop sync.WaitGroup
	stop.Add(readers)

	errCh := make(chan error, readers)
	var done atomic.Bool

	for i := 0; i < readers; i++ {
		go func() {
			defer stop.Done()
			started.Done()
			for !done.Load() {
				genAny, closeAny := r.GetGenerationAndCancelFunc(kind)
				if genAny == nil || closeAny == nil {
					errCh <- fmt.Errorf("получен nil во время чтения")
					return
				}

				gen := genAny.(registryTestGeneration)
				fn := closeAny.(func() int)
				if got, want := uint(fn()), gen.Version(); got != want {
					errCh <- fmt.Errorf("несогласованная пара: close=%d generation=%d", got, want)
					return
				}
			}
		}()
	}

	started.Wait()
	for i := 1; i <= updates; i++ {
		version := uint(i)
		r.SetGeneration(kind, registryTestGeneration{version: version}, func() int {
			return int(version)
		})
	}
	done.Store(true)
	stop.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestRegistryGetFromUnregisteredKindReturnsNil(t *testing.T) {
	r := NewRegistryManager().(*registry)
	kind := registryTestKind{id: 10}

	if got := r.GetGeneration(kind); got != nil {
		t.Fatalf("GetGeneration для незарегистрированного kind вернул %v, ожидается nil", got)
	}

	gen, closeFunc := r.GetGenerationAndCancelFunc(kind)
	if gen != nil || closeFunc != nil {
		t.Fatalf("ожидается (nil, nil) для незарегистрированного kind, получено (%v, %v)", gen, closeFunc)
	}
}

func TestRegistryRegisteredSlotIsInitiallyEmpty(t *testing.T) {
	r := NewRegistryManager().(*registry)
	kind := registryTestKind{id: 3}
	r.RegisterGeneration(kind)

	if got := r.GetGeneration(kind); got != nil {
		t.Fatalf("после RegisterGeneration ожидается nil поколение, получено %v", got)
	}

	gen, closeFunc := r.GetGenerationAndCancelFunc(kind)
	if gen != nil || closeFunc != nil {
		t.Fatalf("после RegisterGeneration ожидается (nil, nil), получено (%v, %v)", gen, closeFunc)
	}
}

func TestRegistryConcurrentRegisterGenerationWithGaps(t *testing.T) {
	r := NewRegistryManager().(*registry)
	ids := []int{1, 5, 2, 9, 4, 8, 3, 7, 6}

	var wg sync.WaitGroup
	for _, id := range ids {
		id := id
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.RegisterGeneration(registryTestKind{id: id})
		}()
	}
	wg.Wait()

	if len(r.slots) < 10 {
		t.Fatalf("длина слайса slots = %d, ожидается минимум 10", len(r.slots))
	}

	sorted := append([]int(nil), ids...)
	sort.Ints(sorted)
	for _, id := range sorted {
		kind := registryTestKind{id: id}
		r.SetGeneration(kind, registryTestGeneration{version: uint(id)}, func() int { return id })

		gen, closeFunc := r.GetGenerationAndCancelFunc(kind)
		if gen == nil || closeFunc == nil {
			t.Fatalf("для kind id=%d ожидается непустая пара", id)
		}

		if got := gen.(registryTestGeneration).Version(); got != uint(id) {
			t.Fatalf("версия для id=%d равна %d, ожидается %d", id, got, id)
		}
		if got := closeFunc.(func() int)(); got != id {
			t.Fatalf("callback для id=%d вернул %d, ожидается %d", id, got, id)
		}
	}
}

func TestRegistrySetGenerationWithoutRegisterPanics(t *testing.T) {
	r := NewRegistryManager().(*registry)
	kind := registryTestKind{id: 5}

	defer func() {
		if recover() == nil {
			t.Fatal("ожидается panic при SetGeneration без предварительного RegisterGeneration")
		}
	}()

	r.SetGeneration(kind, registryTestGeneration{version: 1}, nil)
}


