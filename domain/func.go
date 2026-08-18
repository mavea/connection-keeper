package domain

import "context"

// CancelFunc завершает использование ресурса/поколения и снимает одно удержание.
// Возвращаемое значение:
//   - error: ошибка повторного или некорректного освобождения.
type CancelFunc func() error

// ReadinessFunc проверяет готовность ресурса к использованию.
//
// Параметры:
//   - ctx: контекст проверки готовности.
//
// Возвращаемое значение:
//   - error: ошибка проверки готовности или nil.
type ReadinessFunc func(ctx context.Context) error

// DrainingFunc выполняет финальный шаг дренажа/закрытия поколения.
// Возвращаемое значение:
//   - error: ошибка дренажа или nil.
type DrainingFunc func() error

// HandoverConnectionFunc начинает передачу ресурсов на уровне конкретного соединения.
//
// Это низкоуровневый callback конструктора connection, который вызывается во внутреннем
// lifecycle поколения и работает именно с connection-объектами (*C), а не с самим Generation.
//
// Параметры:
//   - ctx: контекст выполнения операции передачи ресурсов.
//   - next: соединение следующего поколения; может быть nil, если передача выполняется в режим остановки.
//
// Возвращаемые значения:
//   - DrainingFunc: функция финального дренажа connection-ресурсов текущего поколения.
//   - error: ошибка старта передачи ресурсов.
//
// Важно: этот callback не равен HandoverGenerationFunc.
// HandoverGenerationFunc — это обёртка уровня Generation, которая координирует lifecycle шага,
// а HandoverConnectionFunc выполняет фактическую работу с ресурсом соединения.
type HandoverConnectionFunc[C any] func(ctx context.Context, next *C) (DrainingFunc, error)

// ShutdownConnectionFunc начинает остановку ресурсов на уровне конкретного соединения.
//
// Параметры:
//   - ctx: контекст выполнения операции остановки.
//
// Возвращаемые значения:
//   - DrainingFunc: функция финального дренажа connection-ресурсов текущего поколения.
//   - error: ошибка старта остановки ресурсов.
//
// Важно: этот callback не равен ShutdownGenerationFunc.
// ShutdownGenerationFunc — это обёртка уровня Generation, которая инициирует lifecycle-шаг,
// а ShutdownConnectionFunc выполняет низкоуровневую остановку connection-ресурса.
type ShutdownConnectionFunc func(ctx context.Context) (DrainingFunc, error)

type ShutdownSnapshotFunc func(ctx context.Context) (DrainingFunc, error)

// CreateInputConnectionFunc создаёт input-соединение и возвращает его callbacks.
//
// Параметры:
//   - ctx: контекст создания соединения.
//
// Возвращаемые значения:
//   - *C: созданное соединение.
//   - ReadinessFunc: callback проверки готовности соединения.
//   - HandoverConnectionFunc[C]: callback передачи ресурсов следующему поколению.
//   - error: ошибка создания соединения.
type CreateInputConnectionFunc[C any] func(ctx context.Context) (*C, ReadinessFunc, HandoverConnectionFunc[C], error)

// CreateOutputConnectionFunc создаёт output-соединение и возвращает его callbacks.
//
// Параметры:
//   - ctx: контекст создания соединения.
//
// Возвращаемые значения:
//   - *C: созданное соединение.
//   - ReadinessFunc: callback проверки готовности соединения.
//   - ShutdownConnectionFunc: callback остановки ресурсов текущего поколения.
//   - error: ошибка создания соединения.
type CreateOutputConnectionFunc[C any] func(ctx context.Context) (*C, ReadinessFunc, ShutdownConnectionFunc, error)

// CreateSnapshotFunc создаёт snapshot-соединение и возвращает его callbacks.
//
// Параметры:
//   - ctx: контекст создания snapshot-соединения.
//   - builder: объект SnapshotBuilder, предоставляющий доступ к ресурсам snapshot.
//
// Возвращаемые значения:
//   - *C: созданное snapshot-соединение.
//   - ShutdownSnapshotFunc: callback остановки ресурсов snapshot.
//   - error: ошибка создания snapshot-соединения.
type CreateSnapshotFunc[C any] func(ctx context.Context, builder SnapshotBuilder) (*C, ShutdownSnapshotFunc, error)

// ConnectionInvalidationFunc проверяет, нужно ли принудительно пересоздать текущее соединение.
//
// Возвращаемые значения:
//   - bool: true, если соединение нужно обновить.
//   - error: ошибка проверки состояния invalidation.
type ConnectionInvalidationFunc func() (bool, error)
