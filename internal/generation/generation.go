package generation

import (
	"connection-keeper/domain"
	"context"
	"errors"
	"log"
	"sync/atomic"
)

var (
	// ErrGenerationAlreadyRelease возвращается, когда попытка
	// закрыть использование поколения была выполнена повторно.
	ErrGenerationAlreadyRelease = errors.New("использование поколения уже закрыто")

	// ErrGenerationAlreadyClose возвращается, когда попытка
	// закрыть поколение была выполнена повторно.
	ErrGenerationAlreadyClose = errors.New("поколение уже закрыто")

	// ErrGenerationAlreadyDraining возвращается, когда попытка
	// начать дренаж поколения была выполнена повторно.
	ErrGenerationAlreadyDraining = errors.New("поколение уже находится в состоянии дренажа")

	// ErrGenerationIsUse возвращается, когда попытка
	// начать дренаж поколения была выполнена, но при этом поколение всё ещё используется.
	ErrGenerationIsUse = errors.New("поколение используется, дренаж не может быть выполнен")
)

type generation[C any] struct {
	version uint
	// retainCount хранит суммарное количество удержаний поколения.
	//
	// В счётчик входит:
	//   - базовое удержание самого поколения, выставляемое при создании;
	//   - дополнительные удержания, полученные через Conn/Any для остаточных процессов.
	//
	// Поколение может быть выведено из активной эксплуатации, но при этом оставаться
	// доступным для уже начатых процессов, пока retainCount не уменьшится до нуля.
	retainCount atomic.Int64

	// waitForClose закрывается в момент, когда снято последнее удержание поколения.
	waitForClose    chan struct{}
	openChannelFlag atomic.Bool

	con       *C
	readiness domain.ReadinessFunc
}

// ************************ Метаданные ************************

// Version возвращает номер поколения, который может быть использован для идентификации
// поколения и мониторинга его состояния. Номера поколений не должны повторяться.
func (g *generation[C]) Version() uint {
	return g.version
}

// ************************ Доступ к соединению ************************

// releaseRef снимает одно внутреннее удержание поколения.
//
// Метод используется только внутренним кодом пакета и не содержит собственной
// координации сценариев вызова: корректность его применения обеспечивается верхним уровнем lifecycle.
//
// Параметры: отсутствуют.
// Возвращаемые значения: отсутствуют.
func (g *generation[C]) releaseRef() {
	r := g.retainCount.Add(-1)
	log.Printf("[generation#%d] снято удержание, осталось %d\n", g.version, r)
	if r == 0 {
		if g.openChannelFlag.CompareAndSwap(true, false) {
			close(g.waitForClose)
		}
	}
}

// Conn возвращает объект connection поколения и функцию снятия удержания этого использования.
//
// Важно: вывод поколения из активной эксплуатации не запрещает его дальнейшее использование
// уже начатыми остаточными процессами. Пока retainCount больше нуля, поколение считается живым.
//
// Параметры: отсутствуют.
// Возвращаемые значения:
//   - *C: объект соединения поколения.
//   - domain.CancelFunc: функция снятия удержания, полученного этим вызовом Conn.
func (g *generation[C]) Conn() (*C, domain.CancelFunc) {
	drained := atomic.Bool{}
	f := func() error {
		if !drained.CompareAndSwap(false, true) {
			return ErrGenerationAlreadyRelease
		}
		log.Printf("[generation#%d] снимаем удержание Conn, осталось %d\n", g.version, g.retainCount.Load())
		g.releaseRef()

		return nil
	}

	if !g.openChannelFlag.Load() {
		return nil, nil
	}
	g.retainCount.Add(1)

	return g.con, f
}

// Any возвращает объект connection поколения в виде any и функцию снятия удержания этого использования.
// Параметры: отсутствуют.
// Возвращаемые значения:
//   - any: объект поколения в не типизированном виде.
//   - domain.CancelFunc: функция снятия удержания, полученного этим вызовом.
func (g *generation[C]) Any() (any, domain.CancelFunc) {
	return g.Conn()
}

// ************************ Проверка и lifecycle ************************

// Refs возвращает текущее количество удержаний поколения.
//
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - int64: число активных удержаний, включая базовое удержание поколения и удержания от Conn/Any.
func (g *generation[C]) Refs() int64 {
	return g.retainCount.Load()
}

// WaitForClose возвращает канал, который будет закрыт после снятия последнего удержания поколения.
//
// Сам факт вывода поколения из активной эксплуатации ещё не означает закрытие этого канала:
// он закрывается только тогда, когда завершены все остаточные процессы.
//
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - <-chan struct{}: канал завершения поколения.
func (g *generation[C]) WaitForClose() <-chan struct{} {
	return g.waitForClose
}

// Readiness проверяет готовность поколения к использованию.
//
// Если у поколения больше нет удержаний, оно считается завершённым и возвращает ErrGenerationAlreadyClose.
// Пока retainCount больше нуля, поколение может считаться рабочим даже после вывода из активной эксплуатации,
// так как остаточные процессы всё ещё могут его использовать.
//
// Параметры:
//   - ctx: контекст проверки готовности.
//
// Возвращаемое значение:
//   - error: ErrGenerationAlreadyClose для завершённого поколения, ошибка readiness callback или nil.
func (g *generation[C]) Readiness(ctx context.Context) error {
	if g.retainCount.Load() <= 0 {
		return ErrGenerationAlreadyClose
	}

	if g.readiness == nil {
		return nil
	}

	return g.readiness(ctx)
}
