package domain

import "context"

// Logger задаёт минимальный контракт логирования для библиотеки.
//
// Интерфейс сделан компактным, чтобы его можно было легко адаптировать
// под любой внешний логгер. По умолчанию библиотека использует slog,
// но допускается подмена на пользовательскую реализацию.
//
// Общие параметры методов:
//   - ctx: контекст текущей операции (для трассировки и связывания логов).
//   - msg: текст сообщения.
//   - args: дополнительные структурированные поля в формате ключ/значение.
//  		Библиотека не использует их, но требует для совместимости с внешними логгерами, которые
// 			могут их обрабатывать.
type Logger interface {
	// DebugContext пишет отладочное сообщение.
	DebugContext(ctx context.Context, msg string, args ...any)
	// InfoContext пишет информационное сообщение.
	InfoContext(ctx context.Context, msg string, args ...any)
	// WarnContext пишет предупреждение.
	WarnContext(ctx context.Context, msg string, args ...any)
	// ErrorContext пишет сообщение об ошибке.
	ErrorContext(ctx context.Context, msg string, args ...any)
}
