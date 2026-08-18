package kind

import (
	"connection-keeper/domain"
	intlDomain "connection-keeper/internal/domain"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	// ErrKindAlreadyRegistered возвращается при повторной регистрации имени через RegisterName.
	ErrKindAlreadyRegistered = errors.New("kind already registered")
)

// kindManager хранит сопоставление имени коннектора и его внутреннего kind.
//
// Реализация ориентирована на скорость и потокобезопасность без явных mutex:
// используется sync.Map для конкурентного доступа и atomic счётчик для ID.
// В гонке конкурентного создания допускаются дырки в последовательности ID.
// Это не считается ошибкой: на более высоком уровне повторная регистрация уже занятых
// идентификаторов не допускается, а сам manager обязан только быстро и стабильно
// выдавать неотрицательные ID для реально созданных kind.
type kindManager struct {
	next  atomic.Uint32
	kinds sync.Map
	names sync.Map
}

// NewKindManager создаёт менеджер kind.
// Менеджер предназначен для внутреннего использования библиотекой и оптимизирован
// под быстрый конкурентный доступ без дополнительных блокировок.
// Возвращаемое значение:
//   - domain.KindManager: инициализированный потокобезопасный менеджер.
func NewKindManager() intlDomain.KindManager {
	return &kindManager{
		next:  atomic.Uint32{},
		kinds: sync.Map{},
		names: sync.Map{},
	}
}

// normaliseName нормализует имя kind: обрезает пробелы и формирует ключ в нижнем регистре.
// Параметры:
//   - s: исходное имя kind.
//
// Возвращаемые значения:
//   - string: ключ kind в нижнем регистре.
//   - string: исходное имя после TrimSpace.
//   - error: ErrInvalidKind, если имя пустое.
func (km *kindManager) normaliseName(s string) (string, string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", "", ErrInvalidKind
	}
	return strings.ToLower(s), s, nil
}

// Kinds возвращает ссылку на внутреннюю карту kinds.
// Метод предназначен для внутреннего использования и оптимизирован под скорость.
// Возвращаемое значение:
//   - *sync.Map: карта, где ключ — нормализованное имя kind, а значение — domain.Kind.
func (km *kindManager) Kinds() *sync.Map {
	return &km.kinds
}

// Next возвращает текущее значение счётчика next и увеличивает его на 1.
// Возвращаемое значение:
//   - uint32: текущее значение next до увеличения.
func (km *kindManager) Next() uint32 {
	return km.next.Add(1) - 1
}

// RegisterName регистрирует имя kind без создания самого kind.
// Метод нужен, когда отображаемое имя должно быть закреплено заранее,
// а само создание kind может произойти позже в другом кодовом пути.
// Параметры:
//   - s: имя kind.
//
// Возвращаемое значение:
//   - error: ErrInvalidKind для пустого имени или ErrKindAlreadyRegistered при дубликате.
func (km *kindManager) RegisterName(s string) error {
	sk, sv, err := km.normaliseName(s)
	if err != nil {
		return err
	}

	if _, ok := km.names.LoadOrStore(sk, sv); ok {
		return ErrKindAlreadyRegistered
	}

	return nil
}

// GetOrCreateName возвращает нормализованный ключ и каноническое отображаемое имя.
// Если имя ранее было зарегистрировано через RegisterName, метод использует уже
// закреплённое отображаемое значение, чтобы все последующие kind были согласованы.
// Параметры:
//   - s: исходное имя kind.
//
// Возвращаемые значения:
//   - string: ключ kind в нижнем регистре.
//   - string: каноническое имя, которое хранится в names.
//   - error: ErrInvalidKind, если имя пустое.
func (km *kindManager) GetOrCreateName(s string) (string, string, error) {
	sk, sv, err := km.normaliseName(s)
	if err != nil {
		return "", "", err
	}
	svl, _ := km.names.LoadOrStore(sk, sv)

	return sk, svl.(string), nil
}

// GetOrCreate создаёт kind по имени или возвращает уже существующий.
// Поиск выполняется без учёта регистра.
// Созданный kind всегда получает неотрицательный ID, пригодный для быстрого индексного доступа.
// Параметры:
//   - s: имя kind.
//
// Возвращаемые значения:
//   - domain.Kind: найденный или созданный kind.
//   - bool: true, если kind создан в этом вызове; false, если уже существовал.
//   - error: ErrInvalidKind для пустого имени.
func (km *kindManager) GetOrCreate(s string) (domain.Kind, bool, error) {
	sk, sv, err := km.GetOrCreateName(s)
	if err != nil {
		return nil, false, err
	}

	k, b := km.UnsaveGetOrCreate(sk, sv)

	return k, b, nil
}

// UnsaveGetOrCreate создаёт kind по уже нормализованному ключу или возвращает существующий.
// Метод не делает дополнительных проверок и нормализации, потому что рассчитан на
// быстрый внутренний путь, где вызывающий код уже гарантирует корректность sk и sv.
//
// Параметры:
//   - sk: ключ в нижнем регистре.
//   - sv: отображаемое имя kind.
//
// Возвращаемые значения:
//   - domain.Kind: найденный или созданный kind.
//   - bool: true, если kind создан впервые; false, если он уже существовал.
//
// Важно: из-за конкурентных гонок LoadOrStore счётчик next может увеличиться больше,
// чем фактическое количество созданных kind. Это допустимо и не считается ошибкой.
// Все реально сохранённые kind при этом получают только неотрицательные ID.
func (km *kindManager) UnsaveGetOrCreate(sk, sv string) (domain.Kind, bool) {
	value, ok := km.kinds.Load(sk)
	if ok {
		return value.(domain.Kind), false
	}

	value, ok = km.kinds.LoadOrStore(sk, &kind[any]{
		id: int(km.Next()),
		s:  sv,
	})

	return value.(domain.Kind), !ok
}

// Delete удаляет kind из хранилища. Метод предназначен ТОЛЬКО для внутреннего использования
// в пакете connector/func.go при откате регистрации, если регистрация коннектора завершилась ошибкой.
//
// Метод удаляет kind из карты kinds, делая его потенциально доступным для переиспользования
// при новом GetOrCreate с тем же именем. Однако ради скорости никаких проверок на наличие
// активных ссылок (в registry, в connector'е и т.д.) не выполняется.
//
// ОПАСНО: если вызвать этот метод после успешной регистрации kind в ConnectorManager/RegistryManager,
// состояние библиотеки рассинхронизируется, и дальнейшая работа с этим kind'ом приведёт к краше.
//
// Параметры:
//   - kind: удаляемый kind.
func (km *kindManager) Delete(kind domain.Kind) {
	sk, _, err := km.normaliseName(kind.String())
	if err != nil {
		return
	}
	km.kinds.Delete(sk)
}

// NewKind создаёт новый kind с указанным именем и регистрирует его в менеджере.
// Если kind с таким именем уже существует, возвращается ошибка ErrKindAlreadyRegistered.
// Параметры:
//   - name: имя нового kind.
//   - km: менеджер kind, в котором будет зарегистрирован новый kind.
//
// Возвращаемые значения:
//   - domain.KindType[C]: созданный или найденный kind.
//   - error: ErrInvalidKind для пустого имени или ErrKindAlreadyRegistered при дубликате.
func NewKind[C any](name string, km intlDomain.KindManager) (domain.KindType[C], error) {
	sk, sv, err := km.GetOrCreateName(name)
	if err != nil {
		return nil, err
	}

	k, b := unsaveCreate[C](sk, sv, km)
	if !b {
		return nil, ErrKindAlreadyRegistered
	}

	return k, nil

}

// unsaveCreate создаёт kind по уже нормализованному ключу или возвращает существующий.
// Метод не делает дополнительных проверок и нормализации, потому что рассчитан на
// быстрый внутренний путь, где вызывающий код уже гарантирует корректность sk и sv.
//
// Параметры:
//   - sk: ключ в нижнем регистре.
//   - sv: отображаемое имя kind.
//   - km: менеджер kind.
//
// Возвращаемые значения:
//   - domain.KindType[C]: найденный или созданный kind.
//   - bool: true, если kind создан впервые; false, если он уже существовал.
func unsaveCreate[C any](sk, sv string, km intlDomain.KindManager) (domain.KindType[C], bool) {
	value, ok := km.Kinds().Load(sk)
	if ok {
		return value.(domain.Kind), false
	}

	value, ok = km.Kinds().LoadOrStore(sk, &kind[C]{
		id: int(km.Next()),
		s:  sv,
	})

	return value.(domain.Kind), !ok
}
