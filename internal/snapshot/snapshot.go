package snapshot

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"connection-keeper/internal/generation"
	"context"
)

type snapshot[C any] struct {
	kind            domain.Kind
	constructorFunc domain.CreateSnapshotFunc[C]
	mgr             intlDomain.Manager

	version uint
}

func NewSnapshot[C any](
	kind domain.Kind,
	constructorFunc domain.CreateSnapshotFunc[C],
	mgr intlDomain.Manager,
) intlDomain.Connector {
	s := &snapshot[C]{
		kind:            kind,
		constructorFunc: constructorFunc,
		mgr:             mgr,
	}
	s.kind = kind
	s.mgr = mgr

	return s
}

func (s *snapshot[C]) ReconnectIfNeeded(
	ctx context.Context,
	force bool,
) (bool, error) {
	var (
		registryMgr       = s.mgr.RegistryManager()
		gen, shutdownFunc = s.getGenerationAndCancelFunc(registryMgr)
	)

	if !force {
		return false, nil
	}

	builderS := newBuilder(s.mgr)

	// обновление нужно, создаём соединение
	conn, connShutdownFunc, err := s.constructorFunc(
		ctx,
		builderS,
	)
	if err != nil {
		return false, err
	}

	// Соединение создано и первично проверено. Начинаем процесс замещения.
	// Увеличиваем версию. Мы тут не работаем с гонкой, так что пишем напрямую.
	s.version++
	newGen, newCancel := generation.NewSnapshotGeneration(ctx, s.version, conn, connShutdownFunc, builderS.release)
	// Публикуем новое поколение до shutdown старого по контракту этого шага lifecycle:
	// в случае ошибки shutdown выполнение поднимает ошибку выше, что приводит к завершению
	// работы библиотеки и общему пути очистки системы.
	registryMgr.SetGeneration(s.kind, newGen, newCancel)

	// По внутреннему контракту registry shutdownFunc не может быть задан без gen.
	if shutdownFunc != nil {
		var (
			drainFunc domain.DrainingFunc
		)
		drainFunc, err = shutdownFunc()
		if err != nil {
			return false, err
		}
		drainMgr := s.mgr.DrainManager()
		drainMgr.Register(gen, drainFunc)
	}

	return true, nil
}

func (s *snapshot[C]) getGenerationAndCancelFunc(
	registryMgr intlDomain.RegistryManager,
) (intlDomain.Generation[C], intlDomain.ShutdownGenerationFunc) {
	var (
		genT, cancelFuncT = registryMgr.GetGenerationAndCancelFunc(s.kind)
		gen               intlDomain.Generation[C]
		shutdownFunc      intlDomain.ShutdownGenerationFunc
	)
	// Fast path без лишних проверок типа: registry хранит значения строго в согласованном формате
	// внутри библиотеки, поэтому assertion выполняется напрямую ради скорости.
	if genT != nil {
		gen = genT.(intlDomain.Generation[C])
	}
	if cancelFuncT != nil {
		shutdownFunc = cancelFuncT.(intlDomain.ShutdownGenerationFunc)
	}

	return gen, shutdownFunc
}
func (s *snapshot[C]) Close() error {
	var (
		registryMgr       = s.mgr.RegistryManager()
		gen, shutdownFunc = s.getGenerationAndCancelFunc(registryMgr)
	)

	// По внутреннему контракту registry shutdownFunc не может быть задан без gen.
	if shutdownFunc != nil {
		drainMgr := s.mgr.DrainManager()
		drainFunc, err := shutdownFunc()
		if err != nil {
			return err
		}
		drainMgr.Register(gen, drainFunc)
	}

	return nil
}

// Version возвращает текущую версию коннектора.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - uint: номер версии, увеличиваемый при успешной смене поколения.
func (s *snapshot[C]) Version() uint {
	return s.version
}

// Kind возвращает тип коннектора.
// Параметры: отсутствуют.
// Возвращаемое значение:
//   - domain.Kind: идентификатор типа коннектора.
func (s *snapshot[C]) Kind() domain.Kind {
	return s.kind
}
