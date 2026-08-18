# connection-keeper

Библиотека для управления долгоживущими внешними соединениями **без перезапуска хост-приложения**.

## Идея

Проект строится вокруг двух типов соединений:

- **Output** — соединения, которыми пользуется приложение (БД, HTTP-клиенты, очереди/producer).
- **Input** — внешние точки входа в приложение (HTTP-сервер, consumer очереди и т.д.).

Ключевая модель — **generation/snapshot**:

- конкретный клиент живет в `Generation[T]`;
- при смене конфигурации/health создается новое поколение;
- бизнес-логика работает через `Snapshot`, чтобы в рамках одной операции видеть консистентный набор соединений.

## Статус проекта

- Проект сейчас **concept-first**.
- Реализация постепенно расширяется, часть поведения пока в стадии проработки.
- В корне нет `main.go`; рабочие примеры находятся в `example/`.
- Версия Go в `go.mod`: **1.26**.

## Основные пакеты

- `connector` — публичная регистрация соединений: `NewInput`, `NewOutput`.
- `manager` — lifecycle-менеджер: запуск, периодические проверки, shutdown.
- `snapshot` — сборка и получение снэпшотов зависимостей.
- `group` — композиция коннекторов (`NewList`, `NewNeighbors`, `NewDependency`).
- `domain` — контракты и интерфейсы.
- `internal/*` — реализация lifecycle, generation, kind, connector.

## Примеры

- `example/input/http/basic/main.go` — Input-side HTTP lifecycle.
- `example/output/db/basic/main.go` — Output-side пример.

Проверка компиляции примеров:

```bash
make build-examples
```

## Команды разработки

Все команды выполняются через Docker-инструменты из `Makefile`.

```bash
make test
make lint
make generate
make generate-compare
make build-examples
make prepush
```

Что делает `prepush`:

- генерация диаграмм;
- проверка, что сгенерированные файлы актуальны;
- линт;
- юнит-тесты;
- проверка компиляции примеров.

## Диаграммы и документация

- Диаграммы: `doc/diagram/`
- Публичный flow регистрации: `connector/connector.md`
- Внутренний lifecycle: `internal/connector/connector.md`

Генерация диаграмм:

```bash
make generate
```

## Быстрый старт

1. Посмотри runnable sketch в `example/input/http/basic/main.go`.
2. Прогони базовые проверки:

```bash
make test
make lint
make build-examples
```

## Примечание для Windows + WSL2

В проекте рекомендуется выполнять команды внутри WSL2 Ubuntu. Пример вызова из PowerShell:

```powershell
wsl -d Ubuntu -- bash -lc 'cd /mnt/d/projects/lib/connection-keeper && make test'
```
