package generation

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"context"
	"errors"
	"sync/atomic"
)

func noopSnapshotReadiness(ctx context.Context) error {
	return nil
}

func NewSnapshotGeneration[C any](
	ctx context.Context,
	version uint,
	con *C,
	shutdownFunc domain.ShutdownSnapshotFunc,
	builderReleaseFunc domain.CancelFunc,
) (intlDomain.Generation[C], intlDomain.ShutdownGenerationFunc) {
	g := &generation[C]{
		con:          con,
		version:      version,
		retainCount:  atomic.Int64{},
		readiness:    noopSnapshotReadiness,
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
		g.retainCount.Add(-1)

		if shutdownFunc == nil {
			return func() error {
				if g.retainCount.Load() > 0 {
					return ErrGenerationIsUse
				}
				err := builderReleaseFunc()
				return err
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
				return builderReleaseFunc()
			}
			errD := drainFunc()
			if errD != nil {
				drained.Store(false)

				errD2 := builderReleaseFunc()
				if errD2 != nil {
					errD = errors.Join(errD, errD2)
				}

				return errD
			}

			return builderReleaseFunc()
		}, nil
	}
}
