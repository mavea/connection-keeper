package lifecycle

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"container/list"
	"errors"
	"time"
)

type drainGen struct {
	generation intlDomain.GenerationAny
	close      domain.DrainingFunc
	try        uint8
}
type drain struct {
	conf intlDomain.DrainConfig

	// pending - это очередь поколений, которые нужно отслеживать на завершение.
	//Поколение удаляется из очереди, если его WaitForClose() закрывается или если количество попыток ожидания
	//превышает cfg.MaxRetryWaitAttempts().
	pending *list.List
	target  *list.Element
}

// callDrain безопасно вызывает функцию дренажа.
// Параметры:
//   - close: функция финализации поколения; может быть nil.
//
// Возвращаемое значение:
//   - error: ошибка из close, либо nil, если close == nil или завершилась успешно.
func callDrain(close domain.DrainingFunc) error {
	if close == nil {
		return nil
	}

	return close()
}

// NewDrainManager создаёт менеджер дренажа поколений.
// Параметры:
//   - conf: конфигурация таймаута ожидания и лимита попыток.
//
// Возвращаемое значение:
//   - domain.DrainManager: инициализированный менеджер дренажа.
func NewDrainManager(
	conf intlDomain.DrainConfig,
) intlDomain.DrainManager {
	return &drain{
		conf:    conf,
		pending: list.New(),
	}
}

// Register добавляет поколение в очередь дренажа.
// Параметры:
//   - gen: поколение, которое нужно дождаться и затем завершить.
//   - close: callback финализации поколения; вызывается при закрытии gen или по исчерпанию попыток.
//
// Возвращаемые значения: отсутствуют.
func (d *drain) Register(gen intlDomain.GenerationAny, close domain.DrainingFunc) {
	d.pending.PushBack(&drainGen{
		generation: gen,
		close:      close,
		try:        0,
	})
}

// nextGen выбирает следующее поколение для обработки в циклическом порядке.
// Параметры: отсутствуют.
// Возвращаемые значения:
//   - *drainGen: выбранный элемент очереди, либо nil, если очередь пуста.
//   - *list.Element: узел списка для выбранного элемента, либо nil, если очередь пуста.
func (d *drain) nextGen() (*drainGen, *list.Element) {
	if d.pending.Len() == 0 {
		d.target = nil
		return nil, nil
	}

	if d.target == nil {
		d.target = d.pending.Front()
		if d.target == nil {
			return nil, nil
		}
	}

	current := d.target
	d.target = current.Next()
	if d.target == nil {
		d.target = d.pending.Front()
	}

	item, ok := current.Value.(*drainGen)
	if !ok {
		d.removeCurrent(current)
		return d.nextGen()
	}

	return item, current
}

// removeCurrent удаляет текущий элемент из очереди и корректирует курсор target.
// Параметры:
//   - element: удаляемый элемент списка; если nil, метод ничего не делает.
//
// Возвращаемые значения: отсутствуют.
func (d *drain) removeCurrent(element *list.Element) {
	if element == nil {
		return
	}

	if d.pending.Len() == 0 {
		d.target = nil
		return
	}

	if d.target == element {
		d.target = element.Next()
		if d.target == nil {
			d.target = d.pending.Front()
		}
	}

	d.pending.Remove(element)

	if d.pending.Len() == 0 {
		d.target = nil
	}
}

// DrainNext выполняет одну итерацию дренажа для одного поколения.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - error: ошибка callback дренажа, если она возникла; иначе nil.
func (d *drain) DrainNext() error {
	return d.drainNext()
}

// drainNext — внутренняя реализация DrainNext.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - error: ошибка callback дренажа, если callback был вызван и завершился с ошибкой; иначе nil.
func (d *drain) drainNext() error {
	drnGen, element := d.nextGen()
	if drnGen == nil || element == nil {
		return nil
	}

	timer := time.NewTimer(d.conf.CancelWaitTimeOut())
	defer timer.Stop()

	select {
	case <-drnGen.generation.WaitForClose():
		err := callDrain(drnGen.close)
		if err == nil {
			d.removeCurrent(element)
		}

		return err
	case <-timer.C:
		drnGen.try++
		if drnGen.try >= d.conf.MaxRetryWaitAttempts() {
			d.removeCurrent(element)
			return callDrain(drnGen.close)
		}

	}

	return nil
}

// DrainAll выполняет дренаж всех ожидающих поколений до полной очистки очереди.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - error: объединённая ошибка из всех callback дренажа, либо nil при отсутствии ошибок.
func (d *drain) DrainAll() error {
	var (
		errs, err error
	)

	for d.pending.Len() > 0 {
		err = d.drainNext()
		if err != nil {
			errs = errors.Join(errs, err)
		}
	}

	return errs
}
