package domain

import (
	rootDomain "connection-keeper/domain"
	"time"
)

// RegistryManager описывает хранилище актуальных поколений коннекторов.
// Реализация оптимизируется под быстрый доступ по kind.ID().
type RegistryManager interface {
	// RegisterGeneration заранее резервирует слот под указанное поколение.
	// Параметры:
	//   - kind: ключ поколения, по которому будет выполняться запись.
	// Возвращаемые значения: отсутствуют.
	RegisterGeneration(kind rootDomain.Kind)

	// SetGeneration публикует актуальное поколение и callback его завершения.
	// Параметры:
	//   - kind: ключ поколения, для которого выполняется запись.
	//   - generation: актуальное поколение.
	//   - close: callback завершения поколения; хранится без дополнительной типизации.
	// Возвращаемые значения: отсутствуют.
	SetGeneration(kind rootDomain.Kind, generation GenerationAny, close any)

	// GetGeneration возвращает текущее поколение для указанного kind.
	// Параметры:
	//   - kind: ключ поколения.
	// Возвращаемое значение:
	//   - GenerationAny: текущее поколение, либо nil если оно ещё не установлено.
	GetGeneration(kind rootDomain.Kind) GenerationAny

	// GetGenerationAndCancelFunc возвращает текущее поколение и callback его завершения.
	// Параметры:
	//   - kind: ключ поколения.
	// Возвращаемые значения:
	//   - GenerationAny: текущее поколение, либо nil если оно ещё не установлено.
	//   - any: callback завершения поколения, либо nil если он ещё не установлен.
	GetGenerationAndCancelFunc(kind rootDomain.Kind) (GenerationAny, any)
}

// ConnectorManager описывает хранилище зарегистрированных коннекторов.
// Реализация ориентирована на быстрый доступ по kind.ID().
type ConnectorManager interface {
	// RegisterConnector регистрирует коннектор для указанного kind.
	// Параметры:
	//   - kind: ключ типа коннектора.
	//   - connector: коннектор, который будет возвращаться по этому kind.
	// Возвращаемое значение:
	//   - error: ошибка регистрации, если коннектор уже существует или регистрация невозможна.
	RegisterConnector(kind rootDomain.Kind, connector Connector) error

	// GetConnector возвращает ранее зарегистрированный коннектор по kind.
	// Параметры:
	//   - kind: ключ типа коннектора.
	// Возвращаемые значения:
	//   - Connector: найденный коннектор.
	//   - error: ошибка поиска, если коннектор не найден.
	GetConnector(kind rootDomain.Kind) (Connector, error)
	List() []Connector
}

// DrainConfig определяет конфигурацию менеджера дренажа.
// Все методы конфигурации должны быть потокобезопасными.
type DrainConfig interface {
	// CancelWaitTimeOut возвращает максимальное время ожидания закрытия поколения
	// в рамках одной попытки дренажа.
	// Возвращаемое значение: длительность ожидания перед увеличением счётчика попыток.
	CancelWaitTimeOut() time.Duration

	// MaxRetryWaitAttempts возвращает максимальное количество попыток ожидания
	// завершения поколения перед принудительным вызовом функции дренажа.
	// Возвращаемое значение: лимит попыток ожидания для одного поколения.
	MaxRetryWaitAttempts() uint8
}

// DrainManager управляет отложенным завершением старых поколений.
// Он нужен для гарантированного дренажа перед окончательной остановкой процесса.
type DrainManager interface {
	// Register добавляет поколение в очередь дренажа.
	// Параметры:
	//   - gen: поколение, которое нужно дождаться и затем завершить.
	//   - close: callback дренажа, который вызывается после завершения ожидания.
	// Возвращаемые значения: отсутствуют.
	Register(gen GenerationAny, close rootDomain.DrainingFunc)

	// DrainNext выполняет одну попытку дренажа следующего поколения из очереди.
	// Возвращаемое значение:
	//   - error: ошибка callback дренажа, если она возникла; иначе nil.
	DrainNext() error

	// DrainAll завершает дренаж всех зарегистрированных поколений.
	// Возвращаемое значение:
	//   - error: объединённая ошибка всех callback дренажа, либо nil.
	DrainAll() error
}
