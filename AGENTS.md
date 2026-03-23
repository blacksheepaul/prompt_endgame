# AGENTS.md

Guidance for agentic coding assistants working in `prompt_endgame`.

## Scope and Priority

- Follow this file first for repo-specific workflow and style.
- Preserve existing architecture and naming patterns over personal preference.
- Prefer minimal, targeted changes over broad refactors.

## Rule Sources (Cursor/Copilot)

- Checked for Cursor/Copilot rule files in:
  - `.cursor/rules/`
  - `.cursorrules`
  - `.github/copilot-instructions.md`
- No such files currently exist in this repository.
- If those files are added later, treat them as additional constraints.

## Project Snapshot

- Language: Go (`go 1.25.4`)
- Module: `github.com/blacksheepaul/prompt_endgame`
- Architecture: clean/hexagonal
- Entry point: `cmd/server`
- Main packages:
  - `internal/domain`: entities, value objects, domain errors
  - `internal/port`: interfaces/contracts
  - `internal/app`: use-case services and runtime orchestration
  - `internal/adapter`: HTTP/provider/store/scenery implementations
  - `internal/wiring`: dependency wiring

## Build, Run, Test, Lint Commands

Use Make targets when possible; they encode project defaults.

```bash
# Build
make build                      # local binary -> bin/server
make build-linux                # linux build for docker flow

# Run
make run                        # docker compose flow (depends on build-linux)
go run ./cmd/server             # direct local run

# Formatting
make fmt                        # go fmt ./...
go fmt ./...                    # equivalent

# Tests
make test                       # go test ./...
go test ./...                   # all packages
go test -race ./...             # race detector
go test -cover ./...            # coverage snapshot

# Single test (important)
go test -run TestRoomService_CreateRoom ./internal/app/...
go test -run '^TestRoomService_CreateRoom$' ./internal/app
go test -run 'TestRoomService_CreateRoom/.*' ./internal/app   # subtests

# Package-focused test loops
go test ./internal/app
go test ./internal/adapter/store/inmem

# Quality / CI
make ci                         # fmt + test
golangci-lint run               # if golangci-lint is installed

# Cleanup
make clean
```

## Coding Style

### Imports

- Group imports into 3 blocks:
  1) standard library
  2) third-party
  3) internal (`github.com/blacksheepaul/prompt_endgame/...`)
- Keep import blocks gofmt-compliant (gofmt handles ordering inside blocks).

### Formatting

- Always run `go fmt ./...` (or `make fmt`) on changed packages before finishing.
- Keep functions small and readable; no strict line length limit.
- Avoid unnecessary comments; prefer clear names.

### Types and Domain Modeling

- Prefer strong domain types over raw strings where already established:
  - `type RoomID string`
  - `type RoomState string`
  - `type TurnID string`
- Keep domain invariants in domain/app layers, not HTTP handlers.
- Use constructors like `NewXxx(...)` for non-trivial initialization.

### Naming

- Exported identifiers: PascalCase (`RoomService`, `SubmitAnswer`).
- Unexported identifiers: camelCase (`roomRepo`, `turnRuntime`).
- Interface names are noun-like and often `-er` suffixed (`RoomRepository`).
- Enum-like constants use descriptive prefixes (`RoomStateIdle`, `TurnStateDone`).

### Error Handling

- Define sentinel errors near ownership boundary (`internal/domain/errors.go` or package `errors.go`).
- Wrap errors with context using `%w` when propagating:
  - `fmt.Errorf("load config: %w", err)`
- Return domain errors for domain failures (e.g. `domain.ErrRoomNotFound`).
- Do not swallow errors silently; avoid ignoring error returns in production paths.

### Context Usage

- `ctx context.Context` is the first argument for request-scoped operations.
- Respect cancellation/timeouts in long-running streams and provider calls.
- In goroutines, use explicit context strategy (`context.Background()` only when intentional).

### Concurrency and Repositories

- Protect shared in-memory state with `sync.RWMutex`.
- Keep lock scope tight.
- Never perform blocking I/O while holding repository locks.
- Repository pattern in this repo:
  - `Get` returns a value copy for thread safety.
  - `Update` applies a callback under lock for atomic mutation.

### Events and Streaming

- State changes should emit domain events through `EventSink`.
- Room/turn lifecycle should remain consistent with the state machine.
- Track streaming metrics such as TTFT and provider error classes where applicable.

### JSON and Struct Tags

- Use snake_case JSON tags on externally relevant structs.
- Keep wire format stable unless explicitly changing API contract.

## Testing Conventions

- Place tests in `*_test.go` files alongside code under test.
- Prefer table-driven tests for business logic branches.
- Use helper setup functions to reduce duplication.
- Use mock providers from `internal/adapter/provider/mock` for deterministic streaming tests.
- Use `zap.NewNop()` in tests unless logs are part of assertions.
- For concurrency-sensitive code, run targeted race tests (`go test -race ./internal/...`).

## Architecture Guardrails

- `internal/domain`: pure domain objects and errors; avoid infrastructure coupling.
- `internal/port`: interfaces only; no implementation logic.
- `internal/app`: orchestration/use-cases; depends on ports, not concrete adapters.
- `internal/adapter`: concrete implementations (HTTP, providers, store implementations).
- `internal/wiring`: composition root and dependency injection.

## Project-Specific Behavioral Rules

- Room state progression is strict: idle -> streaming -> (cancelled or idle/done path).
- Cancel flow must stop active streaming quickly and emit cancellation events.
- Submit flow should reject parallel turns when room is busy.
- Event stream behavior should support replay/reconnect semantics.
- Observability is a product goal: keep metrics/events/logging intact when modifying flows.

## Storage Configuration

The application supports two storage backends, controlled by `STORE_TYPE` environment variable:

### Memory Store (`STORE_TYPE=memory`, default)
- Fast, no persistence
- Data lost on restart
- Suitable for development and testing

### SQLite Store (`STORE_TYPE=sqlite`)
- Persistent storage using SQLite database
- Supports memory offloading for idle rooms
- Suitable for production deployments

**SQLite Configuration:**
- `STORE_SQLITE_PATH`: Database file path (default: `./data/prompt_endgame.db`)
- `STORE_OFFLOAD_ENABLED`: Enable automatic memory offloading (default: `false`)
- `STORE_MAX_CACHED_ROOMS`: Maximum rooms to keep in memory (default: `100`)
- `STORE_IDLE_TIMEOUT`: Time before offloading idle rooms (default: `5m`)

**Offload Behavior:**
- Only `idle` state rooms can be offloaded
- Offloaded rooms remain in database but are removed from memory cache
- Accessing an offloaded room triggers lazy reload from database
- Background task periodically checks for idle rooms to offload

## Change Management for Agents

- Keep diffs focused; avoid unrelated renames and format churn.
- Do not introduce new dependencies without clear justification.
- Update tests when behavior changes.
- If command/tooling assumptions change, update this file in the same PR.
