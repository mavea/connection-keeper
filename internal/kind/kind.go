package kind

import (
	"errors"
)

var (
	ErrInvalidKind = errors.New("invalid kind text identifier")
)

// kind — внутреннее представление типа коннектора.
// Оно хранит быстрый числовой ID и нормализованное строковое имя.
type kind[C any] struct {
	id int
	s  string
}

// ID возвращает числовой идентификатор коннектора.
// Возвращаемое значение:
//   - int: неотрицательный ID, который используется как индекс во внутренних структурах.
func (k *kind[C]) ID() int {
	return k.id
}

// String возвращает текстовый идентификатор коннектора
// Возвращаемое значение:
//   - string: нормализованное имя коннектора.
func (k *kind[C]) String() string {
	return k.s
}
