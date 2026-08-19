# AGENTS.md

## Project status
- This repo is **concept-first**: the real source of truth is `ignore/articles/01 - concept.ru.md`; Go code is still minimal.
- There is **no root `main.go`**; use `example/input/http/basic/main.go` and `example/output/mock/basic/main.go` as the current lifecycle sketches.
- `go.mod` currently targets `go 1.26`.

## Big picture
- Goal: manage long-lived external connections **without restarting the host app**.
- The architecture revolves around two connection classes from the concept doc:
  - **Output**: app-initiated calls to DBs, HTTP services, queues/producers.
  - **Input**: externally initiated work such as HTTP handlers or queue consumers.
- Consistency is built around a **generation/snapshot** model: a concrete client lives inside a `Generation[T]`; generations can be replaced on config/health changes; business logic uses a `Snapshot` to see a stable set of active outputs for one unit of work.
- Intended flow from the pseudo-API: `NewOutput` / `NewInput` -> `Register` -> `Run` manager -> `GetSnapshot(ctx)`.

## Read these files first
- `README.md` — concise Russian overview plus the current Makefile-based developer workflow (`make test`, `make lint`, `make build-examples`, `make prepush`).
- `ignore/articles/01 - concept.ru.md` — requirements, lifecycle rules, pseudo-code for Output/Input creation, manager registration, and snapshot usage.
- `doc/diagram/class/generation.plant` — type model for `Generation[T]`, `Output[T]`, `Input[T]`, `Snapshot[T]`, plus methods like `Conn()`, `Check()`, `Refs()`, `WaitClose()`.
- `connector/connector.md` — public `NewInput` / `NewOutput` registration flow, rollback behavior, and the current order of registration steps.
- `connector/config.go` and `manager/config.go` — public config constructors used throughout examples/tests: `connector.NewConfig(...)` and `manager.NewConf(...)`.
- `domain/func.go` and `domain/snapshot.go` — callback signatures (`CreateInputConnectionFunc`, `CreateOutputConnectionFunc`, `CreateSnapshotFunc`, `ConnectionInvalidationFunc`, `ShutdownSnapshotFunc`) plus the `SnapshotBuilder` contract.
- `domain/logger.go` — `domain.Logger` interface (DebugContext/InfoContext/WarnContext/ErrorContext); manager defaults to `slog` JSON handler; override via `manager.SetLogger(logger)` (returns `ErrLoggerIsNil` if nil).
- `internal/connector/connector.md` — shared connector base plus the separate `input` / `output` lifecycles, including retry, readiness, and drain handling.
- `internal/generation/generation.md` — retain-count semantics, the base retain held on creation, and why generations may outlive active use until the last retain is released.
- `snapshot/func.go` and `internal/snapshot/{snapshot.go,builder.go}` — the public `NewSnapshot` / `Connection` API plus the builder logic that captures and releases retained generations for snapshot lifetimes.
- `example/input/http/basic/main.go` — current Input-side HTTP lifecycle sketch wired through `connector.NewInput` and `manager.Run`.
- `example/output/mock/basic/main.go` — current Output-side sketch using a mock DB client; still demonstrates transitional direct `RegistryManager().GetGeneration(kind)` access before a dedicated public snapshot example exists.
- `ignore/generation.drawio` — supporting diagram source if terminology between docs needs reconciliation.
- `internal/*/*.md` (for example, `internal/lifecycle/connector.md`, `internal/kind/manager.md`) — package-level behavior notes that match current implementation and tests.

## Project-specific implementation patterns
- Prefer **generic wrappers** over raw clients, matching the doc/diagram names exactly: `Generation`, `Output`, `Input`, `Snapshot`, `Manager`.
- Keep `domain/*.go` as contract-level interfaces/types and place concrete implementations under `internal/*` (notably `internal/connector`, `internal/generation`, `internal/kind`, `internal/lifecycle`).
- The `group/` package contains existing composition connectors (`NewList`, `NewNeighbors`, `NewDependency`) that aggregate `domain.Kind` versions and reconnect behavior through `domain.Connector`: `NewList` sums child versions, `NewNeighbors` refreshes sibling connectors when one member changes, and `NewDependency` forces the dependent connector to refresh when the source version changes.
- Lifecycle is **callback-driven**. The concept doc models constructors that receive functions for config-change detection, connection creation, readiness checking, and close/handover behavior.
- Distinguish **connection-level** callbacks from **generation-level** lifecycle wrappers: `domain.HandoverConnectionFunc` / `domain.ShutdownConnectionFunc` do low-level work on the concrete connection object, while internal `HandoverGenerationFunc` / `ShutdownGenerationFunc` coordinate the one-shot lifecycle step around a `Generation`.
- Treat `input` and `output` connector callbacks differently: `input` hands resources over to the next generation, while `output` shuts down the old generation.
- Generation lifetime is retain-count based: a generation starts with a base retain, drops that retain when it leaves active use, and only closes after the last retain is released.
- Connector registration is a two-step public flow: `NewInput` / `NewOutput` register the `kind` first, reserve a generation slot, then register the connector and immediately call `ReconnectIfNeeded(ctx, true)` for the first generation; rollback can leave an orphan registry slot if connector registration or that initial reconnect fails.
- `NewInput` / `NewOutput` return `domain.KindType[C]` (a generic alias over `domain.Kind`); `snapshot.NewSnapshot` returns the plain `domain.Kind`. Use `domain.KindType[C]` when calling `snapshot.Connection(builder, kind)` for type-safe retrieval.
- Output failures are reported back into lifecycle management (`MarkConnectionUnhealthy` in pseudo-code).
- Input reconnection is modeled as **handover to a new generation**, not in-place mutation.
- Snapshot acquisition is ownership-based: `GetSnapshot(ctx)` returns a cancel/release function in the pseudo-code; preserve that pattern.
- Snapshot constructors are builder-driven: register them via `snapshot.NewSnapshot(...)`, and inside `CreateSnapshotFunc` use `snapshot.Connection(builder, kind)` / `builder.Connection(kind)` to capture typed dependencies instead of reading registry state directly.
- The current output mock example intentionally shows a transitional internal access path (`keeper.RegistryManager().GetGeneration(dbKind)` + `internal/domain.Generation[T]` type assertion); prefer the public snapshot/builder API for new work.

## Dependencies and workflow
- `go.mod` declares `github.com/jackc/pgx/v5`, but no pgx/pgxpool usage is currently present in tracked Go code.
- The concrete runnable sketch shown in code is HTTP Input handling in `example/input/http/basic/main.go`; snapshot composition is exposed as a public package in `snapshot/func.go`, but has no dedicated runnable example yet.
- `golang.org/x/sync` is present indirectly and likely relevant for future ref-count/lifecycle coordination.
- The `preset/` directory is a stub for future integration presets (REST/gRPC clients and servers, Kafka/RabbitMQ, PostgreSQL, Redis, S3, OAuth2, OpenTelemetry, etc.); tracked in `preset/todo`.
- Execute project commands in **WSL2 Ubuntu** (not native Windows shell tools).
- From Windows terminals, use the wrapper form:
  ```powershell
  wsl -d Ubuntu -- bash -lc 'cd /mnt/d/projects/lib/connection-keeper && <command>'
  ```
- Inside WSL, use Linux paths (for this repo: `/mnt/d/projects/lib/connection-keeper`).
- `Makefile` is the maintained automation entrypoint: `make test`, `make lint`, `make generate`, `make generate-compare`, `make build-examples`, `make prepush`; these targets run inside the Docker tool container defined by `docker/tools.Dockerfile`.
- Validation command:
  ```bash
  go test ./...
  ```
- `scripts/build-examples.sh` compiles every `example/**/main.go` with `GOFLAGS=-buildvcs=false`, so `make build-examples` is the fastest way to catch sketch regressions.
- Diagram build helpers are in `Makefile`: `make generate` (SVG + PNG), `make generate-svg` (SVG), `make generate-png` (PNG), `make generate-compare`, `make clean`. `PLANT_DIR` already points at `doc/diagram`.
- Current automated coverage is concentrated in `connector/*_test.go`, `group/*_test.go`, `manager/manager_test.go`, `internal/lifecycle/*_test.go`, and `internal/kind/*_test.go`.

## Guidance for agents
- Align new packages and APIs with the Russian concept doc before coding; many semantics exist only there.
- Expect to create packages the pseudo-code already references (`domain`, manager/generation abstractions) instead of extending the example only.
- Current manager wiring is split: `manager.New(ctx, drainConf)` accepts only the drain config (`CancelWaitTimeOut`, `MaxRetryWaitAttempts`), while `manager.Run(ctx, manager.NewConf(connectionCheckInterval, disableReadinessTimeout, stopTimeout))` receives the runtime loop config.
- `manager.Run` rejects `ConnectionCheckInterval() <= 0` with `ErrCheckIntervalIsNotValid`; if you provide dynamic config, keep that value strictly positive.
- Manager exposes a readiness flag: `Readiness() bool`, `EnableReadiness()`, `DisableReadiness()`, `SetReadiness(bool)`. `Shutdown()` automatically calls `DisableReadiness()` first, then stops the run loop, then closes connectors and drains.
- `example/input/http/basic/main.go` is still best treated as a lifecycle sketch: reuse its HTTP handover flow, but verify any production wiring against the current `manager`, `connector`, and `snapshot` package APIs.
- Do not assume this repo already contains a runnable app; if you need compilable code, first decide whether to align `example/input/http/basic/main.go` with current manager APIs or isolate sketches from default builds.
- Check the root `todo` file before changing lifecycle/group behavior: it currently tracks unresolved edges around connector creation during `manager.Run()`, business-logic complaints on connections, and `group/neighbors` completeness.
