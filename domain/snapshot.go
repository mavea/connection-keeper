package domain

// SnapshotBuilder — одноразовый объект сборки snapshot-поколения.
//
// Builder используется только внутри CreateSnapshotFunc.
// Все соединения, полученные через snapshot.Connection(...), автоматически
// удерживаются до drain/закрытия snapshot-поколения.
type SnapshotBuilder interface {
	Connection(kind Kind) (any, error)
}
