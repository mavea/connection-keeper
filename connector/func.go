package connector

import (
	"connection-keeper/domain"
	intlConnector "connection-keeper/internal/connector"
	intlDomain "connection-keeper/internal/domain"
	intlKind "connection-keeper/internal/kind"
	"context"
	"errors"
)

var (
	// ErrKindAlreadyCreated возвращается, если имя уже занято существующим kind.
	ErrKindAlreadyCreated = errors.New("kind already created")
)

// registerKindConnector выполняет общий сценарий регистрации kind и его коннектора.
//
// Порядок шагов важен: сначала резервируется kind и слот registry, затем регистрируется
// connector. При ошибке регистрации коннектора выполняется rollback только через kind.Delete.
// Slot в registry при этом может остаться, что допустимо в текущем дизайне lifecycle.
//
// Параметры:
//   - name: имя kind, по которому выполняется GetOrCreate.
//   - mgr: общий lifecycle-менеджер, предоставляющий доступ к kind/registry/connector менеджерам.
//   - newConnector: фабрика конкретного коннектора (input или output) для созданного kind.
//
// Возвращаемые значения:
//   - domain.Kind: созданный kind при успешной регистрации.
//   - error: ошибка GetOrCreate/RegisterConnector или ErrKindAlreadyCreated, если kind уже существовал.
func registerKindConnector[C any](
	ctx context.Context,
	name string,
	mgr intlDomain.Manager,
	newConnector func(kind domain.Kind) intlDomain.Connector,
) (domain.KindType[C], error) {
	kindMgr := mgr.KindManager()
	kind, err := intlKind.NewKind[C](name, kindMgr)
	if err != nil {
		return nil, err
	}
	registryMgr := mgr.RegistryManager()
	registryMgr.RegisterGeneration(kind)

	connectorMgr := mgr.ConnectorManager()

	connector := newConnector(kind)
	err = connectorMgr.RegisterConnector(kind, connector)
	if err != nil {
		// Допускаем возможный orphan-slot в registry при rollback: на этой стадии внешнее соединение ещё не создано,
		// поэтому нет утечки внешних ресурсов; это осознанный компромисс текущего lifecycle-дизайна.
		kindMgr.Delete(kind)
		return nil, err
	}

	_, err = connector.ReconnectIfNeeded(ctx, true)
	if err != nil {
		// Допускаем возможный orphan-slot в registry при rollback: на этой стадии внешнее соединение ещё не создано,
		// поэтому нет утечки внешних ресурсов; это осознанный компромисс текущего lifecycle-дизайна.
		kindMgr.Delete(kind)
		return nil, err
	}

	return kind, nil
}

// NewInput регистрирует input-коннектор по имени и связывает его с manager.
//
// Параметры:
//   - name: отображаемое имя kind входящего коннектора.
//   - mgr: менеджер, в котором регистрируются kind, registry-слот и connector.
//   - conf: конфигурация retry/readiness/disable-readiness для input-коннектора.
//   - needsUpdate: callback проверки необходимости пересоздания соединения.
//   - constructorFunc: callback создания input-соединения и его lifecycle-функций.
//
// Возвращаемые значения:
//   - domain.KindType[C]: зарегистрированный kind input-коннектора.
//   - error: ошибка регистрации или ErrKindAlreadyCreated при повторном имени.
func NewInput[C any](
	ctx context.Context,
	name string,
	mgr intlDomain.Manager,
	conf intlDomain.ConnectorConfig,
	needsUpdate domain.ConnectionInvalidationFunc,
	constructorFunc domain.CreateInputConnectionFunc[C],
) (domain.KindType[C], error) {
	return registerKindConnector[C](ctx, name, mgr, func(kind domain.Kind) intlDomain.Connector {
		return intlConnector.NewInput(kind, conf, needsUpdate, constructorFunc, mgr)
	})
}

// NewOutput регистрирует output-коннектор по имени и связывает его с manager.
//
// Параметры:
//   - name: отображаемое имя kind исходящего коннектора.
//   - mgr: менеджер, в котором регистрируются kind, registry-слот и connector.
//   - conf: конфигурация retry/readiness/disable-readiness для output-коннектора.
//   - needsUpdate: callback проверки необходимости пересоздания соединения.
//   - constructorFunc: callback создания output-соединения и его lifecycle-функций.
//
// Возвращаемые значения:
//   - domain.KindType[C]: зарегистрированный kind output-коннектора.
//   - error: ошибка регистрации или ErrKindAlreadyCreated при повторном имени.
func NewOutput[C any](
	ctx context.Context,
	name string,
	mgr intlDomain.Manager,
	conf intlDomain.ConnectorConfig,
	needsUpdate domain.ConnectionInvalidationFunc,
	constructorFunc domain.CreateOutputConnectionFunc[C],
) (domain.KindType[C], error) {
	return registerKindConnector[C](ctx, name, mgr, func(kind domain.Kind) intlDomain.Connector {
		return intlConnector.NewOutput(kind, conf, needsUpdate, constructorFunc, mgr)
	})
}
