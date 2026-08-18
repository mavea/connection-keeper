package lifecycle

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"sync"
	"sync/atomic"
)

type registryValue struct {
	generation intlDomain.GenerationAny
	close      any
}

// registrySlot хранит одно актуальное значение слота.
// Значение публикуется атомарно, чтобы чтения выполнялись без блокировок.
type registrySlot struct {
	value atomic.Pointer[registryValue]
}

// newRegistrySlot создаёт слот и записывает в него пустое значение.
// Возвращаемое значение:
//   - *registrySlot: инициализированный слот для дальнейших атомарных чтений/записей.
func newRegistrySlot() *registrySlot {
	slot := &registrySlot{}
	slot.value.Store(&registryValue{})

	return slot
}

// store атомарно публикует новую пару "поколение + callback".
// Параметры:
//   - generation: актуальное поколение для публикации в слоте.
//   - close: callback завершения, связанный с этим поколением.
//
// Возвращаемые значения: отсутствуют.
func (s *registrySlot) store(generation intlDomain.GenerationAny, close any) {
	s.value.Store(&registryValue{
		generation: generation,
		close:      close,
	})
}

// load атомарно читает текущее значение слота.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - *registryValue: текущая опубликованная пара; nil, если слот не инициализирован.
func (s *registrySlot) load() *registryValue {
	if s == nil {
		return nil
	}

	return s.value.Load()
}

// registry — быстрое внутреннее хранилище актуальных версий поколений.
// Структура оптимизирована под скорость доступа по индексу и опирается на контракт
// domain.Kind: ID всегда положительный, поэтому проверки знака не нужны.
// Так же предполагается, что регистрация слотов выполняется заранее и в правильном порядке.
// После регистрации слота доступ к нему идёт без блокировок.
type registry struct {
	slots []*registrySlot

	// mu защищает массив при регистрации новых слотов.
	// После регистрации слота доступ к нему идёт без блокировок.
	mu sync.Mutex
}

// NewRegistryManager создаёт быстрое внутреннее хранилище поколений.
// Возвращаемое значение:
//   - domain.RegistryManager: менеджер, ориентированный на быстрый доступ по индексу.
func NewRegistryManager() intlDomain.RegistryManager {
	return &registry{
		slots: make([]*registrySlot, 0),
		mu:    sync.Mutex{},
	}
}

// ensureSlot возвращает уже зарегистрированный слот для kind.
// Параметры:
//   - kind: ключ, чьё значение ID используется как индекс.
//
// Возвращаемое значение:
//   - *registrySlot: слот для записи/чтения.
//
// Важно: метод является быстрым путём и предполагает, что слот заранее
// зарегистрирован через RegisterGeneration.
func (r *registry) ensureSlot(kind domain.Kind) *registrySlot {
	return r.slots[kind.ID()]
}

// slot возвращает слот по kind, если он уже зарегистрирован.
// Параметры:
//   - kind: ключ, чьё значение ID используется как индекс.
//
// Возвращаемое значение:
//   - *registrySlot: слот при наличии; nil, если слот ещё не зарегистрирован.
func (r *registry) slot(kind domain.Kind) *registrySlot {
	id := kind.ID()
	if id >= len(r.slots) {
		return nil
	}

	return r.slots[id]
}

// RegisterGeneration заранее расширяет внутренние слоты под указанный kind.
// Параметры:
//   - kind: ключ поколения, по ID которого резервируется слот.
//
// Возвращаемые значения: отсутствуют.
//
// Важно: метод рассчитан на внутренний вызов из библиотеки и не защищает от
// некорректного порядка вызовов. Метод предполагает, что kind.ID() всегда >= 0
// и что регистрация слотов выполняется заранее. Метод потокобезопасен и защищает массив слотов
// от одновременного расширения. Использование поколений на момент вызова RegisterGeneration, не предполагается.
func (r *registry) RegisterGeneration(kind domain.Kind) {
	id := kind.ID()
	if id < 0 {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if id < len(r.slots) {
		return
	}

	oldLen := len(r.slots)
	r.slots = append(r.slots, make([]*registrySlot, id-oldLen+1)...)
	for i := oldLen; i <= id; i++ {
		r.slots[i] = newRegistrySlot()
	}
}

// SetGeneration быстро записывает актуальное поколение и его callback в уже подготовленный слот.
// Параметры:
//   - kind: ключ поколения, по ID которого выполняется запись.
//   - generation: актуальное поколение для данного kind.
//   - close: callback завершения поколения; хранится как есть без дополнительной валидации.
//
// Возвращаемые значения: отсутствуют.
//
// Важно: метод предполагает, что RegisterGeneration для этого kind уже был вызван.
func (r *registry) SetGeneration(
	kind domain.Kind,
	generation intlDomain.GenerationAny,
	close any,
) {
	r.ensureSlot(kind).store(generation, close)
}

// GetGeneration возвращает актуальное поколение для указанного kind.
// Параметры:
//   - kind: ключ поколения.
//
// Возвращаемое значение:
//   - domain.GenerationAny: текущее поколение, хранящееся в registry.
//
// Важно: метод не проверяет знак ID и рассчитан на корректное использование внутри библиотеки.
func (r *registry) GetGeneration(kind domain.Kind) intlDomain.GenerationAny {
	slot := r.slot(kind)

	value := slot.load()
	if value == nil {
		return nil
	}

	return value.generation
}

// GetGenerationAndCancelFunc возвращает актуальное поколение и callback его завершения.
// Параметры:
//   - kind: ключ поколения.
//
// Возвращаемые значения:
//   - domain.GenerationAny: текущее поколение для указанного kind.
//   - any: callback завершения поколения, сохранённый в registry без дополнительной проверки типа.
//
// Важно: метод оптимизирован под скорость и не проверяет знак ID.
func (r *registry) GetGenerationAndCancelFunc(kind domain.Kind) (intlDomain.GenerationAny, any) {
	slot := r.slot(kind)

	value := slot.load()
	if value == nil {
		return nil, nil
	}

	return value.generation, value.close
}
