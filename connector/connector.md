# connector package

## Назначение
Пакет `connector` предоставляет публичные функции регистрации:
- `NewInput`
- `NewOutput`

Обе функции используют общий внутренний сценарий `registerKindConnector`.

## Сценарий регистрации
1. `KindManager.GetOrCreate(name)` создаёт или возвращает `kind`.
2. Если `kind` уже существовал, возвращается `ErrKindAlreadyCreated`.
3. `RegistryManager.RegisterGeneration(kind)` резервирует слот для поколения.
4. `ConnectorManager.RegisterConnector(kind, connector)` регистрирует конкретный коннектор.

## Rollback при ошибке
Если `RegisterConnector` возвращает ошибку, выполняется rollback через `KindManager.Delete(kind)`.

Важно:
- На этом этапе внешнее соединение ещё не создано (создание происходит позже во внутреннем lifecycle).
- Поэтому утечки внешних ресурсов нет.
- При rollback может остаться orphan-slot в `RegistryManager`; в текущем lifecycle-дизайне это допустимый компромисс.

## Почему NewInput/NewOutput разделены
Публичные функции остаются раздельными, потому что принимают разные конструкторы:
- `CreateInputConnectionFunc[C]`
- `CreateOutputConnectionFunc[C]`

Общая часть регистрации вынесена в private helper, чтобы не дублировать шаги и rollback-логику.

