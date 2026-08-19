package generation

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"log"
	"sync/atomic"
)

// NewInputGeneration создаёт поколение входящего коннектора и функцию запуска его вывода из активной эксплуатации.
//
// Параметры:
//   - ctx: контекст работы библиотеки и всего lifecycle этого поколения.
//     Он намеренно захватывается в замыкании handover и не заменяется локальным контекстом вызова.
//   - version: номер поколения.
//   - con: объект соединения этого поколения.
//   - readiness: функция проверки готовности поколения; может быть nil.
//   - handoverFunc: функция начала передачи ресурсов следующему поколению; может быть nil.
//
// Возвращаемые значения:
//   - domain.Generation[C]: созданное поколение.
//   - domain.HandoverGenerationFunc[C]: функция вывода поколения из активной эксплуатации.
//
// Важно:
//   - возвращаемая handover-функция не предназначена для ретраев;
//   - её задача — один раз отметить вывод поколения из активной эксплуатации и, при наличии,
//     запустить передачу ресурсов;
//   - после этого поколение может продолжать использоваться остаточными процессами,
//     пока retainCount не уменьшится до нуля.
func NewInputGeneration[C any](
	ctx context.Context,
	version uint,
	con *C,
	readiness domain.ReadinessFunc,
	handoverFunc domain.HandoverConnectionFunc[C],
) (intlDomain.Generation[C], intlDomain.HandoverGenerationFunc[C]) {
	g := &generation[C]{
		con:          con,
		version:      version,
		retainCount:  atomic.Int64{},
		readiness:    readiness,
		waitForClose: make(chan struct{}),
	}
	g.openChannelFlag.Store(true)
	g.retainCount.Add(1)

	closeFlag := atomic.Bool{}
	return g, func(next *C) (domain.DrainingFunc, error) {
		if !closeFlag.CompareAndSwap(false, true) {
			return nil, ErrGenerationAlreadyClose
		}
		// Снимаем базовое удержание поколения в момент вывода его из активной эксплуатации.
		// Это ожидаемое одноразовое действие и не рассматривается как сценарий для повторного запуска.
		log.Printf("[generation#%d] вывод из активной эксплуатации\n", g.version)
		g.releaseRef()

		if handoverFunc == nil {
			return func() error {
				if g.retainCount.Load() > 0 {
					return ErrGenerationIsUse
				}
				return nil
			}, nil
		}

		drainFunc, err := handoverFunc(ctx, next)
		if err != nil {
			return nil, err
		}

		drained := atomic.Bool{}
		return func() error {
			if g.retainCount.Load() > 0 {
				return ErrGenerationIsUse
			}

			if !drained.CompareAndSwap(false, true) {
				return ErrGenerationAlreadyDraining
			}

			if drainFunc == nil {
				return nil
			}
			errD := drainFunc()
			if errD != nil {
				drained.Store(false)
			}

			return errD
		}, nil
	}
}
