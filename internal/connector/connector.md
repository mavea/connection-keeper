# Пакет `internal/connector`

`internal/connector` содержит реализацию коннекторов, которые управляют жизненным циклом поколений соединений.

## Концепт

В пакете есть три ключевых части:

- `connector[C]` — общая база с общей логикой проверок и принятия решения о переподключении;
- `input[C]` — реализация для входящих соединений (handover между поколениями);
- `output[C]` — реализация для исходящих соединений (shutdown старого поколения).

Базовый `connector[C]` не используется самостоятельно: он встраивается в `input` и `output`.

## Роли файлов

- `connector.go`
  - общая логика: `EnsureConnectionReady`, `IsConnectionInvalidated`, `shouldReconnect`, `waitRetryInterval`, `Version`, `Kind`.
- `input.go`
  - создание input-коннектора `NewInput`;
  - retry-логика создания input-соединения `retryConnect`;
  - переключение поколений через `ReconnectIfNeeded`;
  - закрытие через `Close` с постановкой старого поколения в дренаж.
- `output.go`
  - создание output-коннектора `NewOutput`;
  - retry-логика создания output-соединения `retryConnect`;
  - переключение поколений через `ReconnectIfNeeded`;
  - закрытие через `Close` с постановкой старого поколения в дренаж.
- `../../connector/config.go`
  - простая внутренняя реализация `domain.ConnectorConfig` для быстрого ("ленивого") создания конфигурации,
    в том числе в тестовых сценариях.

## Основной рабочий сценарий

1. Менеджер вызывает `ReconnectIfNeeded(ctx, force)`.
2. Коннектор через `shouldReconnect(...)` решает, нужно ли обновление:
   - проверяет `IsConnectionInvalidated()`;
   - учитывает `force`;
   - проверяет readiness текущего поколения.
3. Если обновление нужно:
   - `retryConnect(...)` создаёт новое соединение с повторами;
   - выполняется первичная readiness-проверка;
   - формируется новое `generation` и публикуется в `registry`.
4. Старое поколение переводится в дренаж через `DrainManager.Register(...)`.

## Особенности input и output

- `input`
  - использует `HandoverConnectionFunc`: ресурсы передаются в новое соединение.
- `output`
  - использует `ShutdownConnectionFunc`: ресурсы старого соединения останавливаются.

Оба пути публикуют и дренируют поколения через `internal/generation` и `internal/lifecycle/registry`.

## Договорённости и ограничения

- Пакет внутренний: прямое внешнее использование не предполагается.
- Приоритет — быстрый рабочий путь, поэтому:
  - в некоторых местах применяются прямые type assertion без дополнительных проверок;
  - часть контрактов обеспечивается верхним уровнем (`manager` + `registry`).
- `waitRetryInterval` при `retryInterval <= 0` не ждёт и возвращает текущее состояние `ctx`.
- `kind.ID()` используется как быстрый индекс во внутренних структурах.

## Что важно для сопровождения

- `input` и `output` похожи по структуре, но имеют разную семантику callback (`handover` vs `shutdown`).
- Логи retry/readiness должны оставаться точными по причине сбоя (constructor, readiness, interval wait, context cancel).
- Изменения в `domain`-контрактах (`Generation`, `ConnectorConfig`, callback-типы) нужно синхронно отражать в этом пакете.

