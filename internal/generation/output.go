package generation

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"sync/atomic"
)

// NewOutputGeneration создаёт поколение исходящего коннектора и функцию запуска его вывода из активной эксплуатации.
//
// Параметры:
//   - ctx: контекст работы библиотеки и всего lifecycle этого поколения.
//     Он намеренно захватывается в замыкании shutdown и не заменяется локальным контекстом вызова.
//   - version: номер поколения.
//   - con: объект соединения этого поколения.
//   - readiness: функция проверки готовности поколения; может быть nil.
//   - shutdownFunc: функция начала остановки ресурсов поколения; может быть nil.
//
// Возвращаемые значения:
//   - domain.Generation[C]: созданное поколение.
//   - domain.ShutdownGenerationFunc: функция вывода поколения из активной эксплуатации.
//
// Важно:
//   - возвращаемая shutdown-функция не предназначена для ретраев;
//   - её задача — один раз отметить вывод поколения из активной эксплуатации и, при наличии,
//     запустить остановку ресурсов;
//   - после этого поколение может продолжать использоваться остаточными процессами,
//     пока retainCount не уменьшится до нуля.
func NewOutputGeneration[C any](
	ctx context.Context,
	version uint,
	con *C,
	readiness domain.ReadinessFunc,
	shutdownFunc domain.ShutdownConnectionFunc,
) (intlDomain.Generation[C], intlDomain.ShutdownGenerationFunc) {
	g := &generation[C]{
		con:          con,
		version:      version,
		retainCount:  atomic.Int64{},
		readiness:    readiness,
		waitForClose: make(chan struct{}),
	}
	g.retainCount.Add(1)

	closeFlag := atomic.Bool{}
	return g, func() (domain.DrainingFunc, error) {
		if !closeFlag.CompareAndSwap(false, true) {
			return nil, ErrGenerationAlreadyClose
		}
		// Снимаем базовое удержание поколения в момент вывода его из активной эксплуатации.
		// Это ожидаемое одноразовое действие и не рассматривается как сценарий для повторного запуска.
		if g.retainCount.Add(-1) == 0 {
			close(g.waitForClose)
		}

		if shutdownFunc == nil {
			return func() error {
				if g.retainCount.Load() > 0 {
					return ErrGenerationIsUse
				}
				return nil
			}, nil
		}

		drainFunc, err := shutdownFunc(ctx)
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
