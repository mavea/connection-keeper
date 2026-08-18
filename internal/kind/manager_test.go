package kind

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func collectKinds(t *testing.T, km *kindManager) []string {
	t.Helper()

	var got []string
	km.Kinds().Range(func(_, value any) bool {
		got = append(got, value.(interface{ String() string }).String())
		return true
	})

	return got
}

// newTestKindManager создаёт concrete manager для тестов внутреннего поведения.
func newTestKindManager() *kindManager {
	return NewKindManager().(*kindManager)
}

func TestNewKindManagerReturnsEmptyList(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	if got := collectKinds(t, km); len(got) != 0 {
		t.Fatalf("Kinds() после создания менеджера вернул %d элементов, ожидается 0", len(got))
	}
}

func TestRegisterNameRejectsEmptyName(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	if err := km.RegisterName("   \t\n  "); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("RegisterName() для пустого имени вернул ошибку %v, ожидается %v", err, ErrInvalidKind)
	}
}

func TestRegisterNameRejectsDuplicateIgnoringCaseAndSpaces(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	if err := km.RegisterName(" HTTP "); err != nil {
		t.Fatalf("RegisterName() вернул ошибку для первого имени: %v", err)
	}
	if err := km.RegisterName("http"); !errors.Is(err, ErrKindAlreadyRegistered) {
		t.Fatalf("RegisterName() для дубликата вернул ошибку %v, ожидается %v", err, ErrKindAlreadyRegistered)
	}
}

func TestGetOrCreateNormalizesNameAndReusesKind(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	first, created, err := km.GetOrCreate(" HTTP ")
	if err != nil {
		t.Fatalf("GetOrCreate() вернул ошибку при первом создании: %v", err)
	}
	if !created {
		t.Fatal("GetOrCreate() при первом вызове должен создавать kind")
	}
	if first.ID() < 0 {
		t.Fatalf("ID() = %d, ожидается неотрицательное значение", first.ID())
	}
	if first.String() != "HTTP" {
		t.Fatalf("String() = %q, ожидается %q", first.String(), "HTTP")
	}

	second, created, err := km.GetOrCreate("http")
	if err != nil {
		t.Fatalf("GetOrCreate() вернул ошибку при повторном запросе: %v", err)
	}
	if created {
		t.Fatal("GetOrCreate() при повторном вызове не должен создавать kind")
	}
	if second.ID() != first.ID() {
		t.Fatalf("повторный kind получил другой ID: %d != %d", second.ID(), first.ID())
	}
	if second.String() != first.String() {
		t.Fatalf("повторный kind получил другое имя: %q != %q", second.String(), first.String())
	}
}

func TestGetOrCreateUsesNameRegisteredEarlier(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	if err := km.RegisterName("HTTP Client"); err != nil {
		t.Fatalf("RegisterName() вернул ошибку: %v", err)
	}

	got, created, err := km.GetOrCreate("http client")
	if err != nil {
		t.Fatalf("GetOrCreate() вернул ошибку: %v", err)
	}
	if !created {
		t.Fatal("GetOrCreate() должен создать kind после одной только регистрации имени")
	}
	if got.String() != "HTTP Client" {
		t.Fatalf("String() = %q, ожидается %q", got.String(), "HTTP Client")
	}
}

func TestUnsaveGetOrCreateUsesCallerKeyAsIs(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	upper, created := km.UnsaveGetOrCreate("HTTP", "HTTP")
	if !created {
		t.Fatal("первый UnsaveGetOrCreate() должен создавать kind")
	}

	lower, created := km.UnsaveGetOrCreate("http", "http")
	if !created {
		t.Fatal("UnsaveGetOrCreate() с другим ключом должен создавать отдельный kind")
	}

	if upper.ID() == lower.ID() {
		t.Fatalf("ожидались разные ID для разных ключей, получено %d", upper.ID())
	}
	if got := collectKinds(t, km); len(got) != 2 {
		t.Fatalf("Kinds() вернул %d элементов, ожидается 2", len(got))
	}
}

func TestGetListReturnsOnlyCreatedKinds(t *testing.T) {
	t.Parallel()

	km := newTestKindManager()

	if err := km.RegisterName("http"); err != nil {
		t.Fatalf("RegisterName() вернул ошибку: %v", err)
	}
	if _, _, err := km.GetOrCreate("grpc"); err != nil {
		t.Fatalf("GetOrCreate() вернул ошибку: %v", err)
	}
	if _, _, err := km.GetOrCreate("kafka"); err != nil {
		t.Fatalf("GetOrCreate() вернул ошибку: %v", err)
	}

	got := map[string]struct{}{}
	km.Kinds().Range(func(_, value any) bool {
		got[value.(interface{ String() string }).String()] = struct{}{}
		return true
	})
	if len(got) != 2 {
		t.Fatalf("Kinds() вернул %d элементов, ожидается 2", len(got))
	}

	if _, ok := got["grpc"]; !ok {
		t.Fatal("Kinds() не содержит kind grpc")
	}
	if _, ok := got["kafka"]; !ok {
		t.Fatal("Kinds() не содержит kind kafka")
	}
	if _, ok := got["http"]; ok {
		t.Fatal("Kinds() не должен содержать только зарегистрированное, но не созданное имя")
	}
}

func TestRegisterNameConcurrentSingleSuccess(t *testing.T) {
	km := newTestKindManager()

	const workers = 32

	var success atomic.Int32
	var duplicates atomic.Int32
	var wg sync.WaitGroup
	start := make(chan struct{})

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start

			err := km.RegisterName("http")
			switch {
			case err == nil:
				success.Add(1)
			case errors.Is(err, ErrKindAlreadyRegistered):
				duplicates.Add(1)
			default:
				t.Errorf("RegisterName() вернул неожиданную ошибку: %v", err)
			}
		}()
	}

	close(start)
	wg.Wait()

	if got := success.Load(); got != 1 {
		t.Fatalf("успешных регистраций = %d, ожидается 1", got)
	}
	if got := duplicates.Load(); got != workers-1 {
		t.Fatalf("дубликатов = %d, ожидается %d", got, workers-1)
	}
}

func TestGetOrCreateConcurrentReturnsSingleKind(t *testing.T) {
	km := newTestKindManager()

	const workers = 64

	var createdCount atomic.Int32
	ids := make([]int, workers)
	stringsGot := make([]string, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start

			got, created, err := km.GetOrCreate("HTTP")
			if err != nil {
				t.Errorf("GetOrCreate() вернул ошибку: %v", err)
				return
			}
			if created {
				createdCount.Add(1)
			}

			ids[index] = got.ID()
			stringsGot[index] = got.String()
		}(i)
	}

	close(start)
	wg.Wait()

	if got := createdCount.Load(); got != 1 {
		t.Fatalf("число созданий = %d, ожидается 1", got)
	}

	firstID := ids[0]
	if firstID < 0 {
		t.Fatalf("ID() = %d, ожидается неотрицательное значение", firstID)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] != firstID {
			t.Fatalf("goroutine %d получила ID %d, ожидается %d", i, ids[i], firstID)
		}
	}
	for i := range stringsGot {
		if stringsGot[i] != "HTTP" {
			t.Fatalf("goroutine %d получила имя %q, ожидается %q", i, stringsGot[i], "HTTP")
		}
	}
	if got := collectKinds(t, km); len(got) != 1 {
		t.Fatalf("Kinds() вернул %d элементов, ожидается 1", len(got))
	}
}

func TestGetOrCreateConcurrentUniqueKindsHaveNonNegativeUniqueIDs(t *testing.T) {
	km := newTestKindManager()

	const workers = 64

	ids := make([]int, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})

	for i := range workers {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start

			got, created, err := km.GetOrCreate(fmt.Sprintf("kind-%d", index))
			if err != nil {
				t.Errorf("GetOrCreate() вернул ошибку: %v", err)
				return
			}
			if !created {
				t.Errorf("kind-%d не был создан в своём первом вызове", index)
				return
			}

			ids[index] = got.ID()
		}(i)
	}

	close(start)
	wg.Wait()

	seen := make(map[int]struct{}, workers)
	for i, id := range ids {
		if id < 0 {
			t.Fatalf("kind-%d получил отрицательный ID %d", i, id)
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("обнаружен повторный ID %d", id)
		}
		seen[id] = struct{}{}
	}

	if got := collectKinds(t, km); len(got) != workers {
		t.Fatalf("Kinds() вернул %d элементов, ожидается %d", len(got), workers)
	}
}
