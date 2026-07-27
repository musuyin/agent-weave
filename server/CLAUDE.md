# Backend (Go)

## Commands

```bash
go build ./...
go test ./...
go test ./test/... -race          # all tests with race detector
go test ./test/handler/... -run TestConversation -v
wire ./cmd/server/                # regenerate DI wiring after changing providers
go mod tidy

# DB migrations (manual; auto-run at server startup too)
migrate -path db/migrations -database "mysql://..." up
migrate -path db/migrations -database "mysql://..." down 1
```

## Package layout

```
internal/
  config/       Config struct (nested YAML: llm_model, database, server, oidc)
  db/           ProvideDB — opens GORM, runs migrations
  model/        Conversation, Message, Thread (ContentBlocks JSON scanner)
  agent/        SSE Hub, HubRegistry, agent loop (Service.Run)
  service/      Business logic: ConversationService, MessageService
  handler/      Thin HTTP adapters: bind → call service → respond
test/           All test files (external _test packages, mirroring internal/)
  model/
  config/
  agent/
  handler/
```

## Architecture: handler → service → repository (GORM)

- **handler**: HTTP only — bind request, call service, write status code + JSON
- **service**: owns all business logic, DB access, validation errors
- **no separate repository layer** — GORM is the repository; service calls `db.WithContext` directly

When adding a new feature: write the service method first, then the thin handler that calls it.

## Wire (DI)

`cmd/server/wire.go` declares the provider graph (build tag `wireinject`).
`cmd/server/wire_gen.go` is the generated output — commit it, never edit it by hand.
After adding or changing a provider signature: run `wire ./cmd/server/` to regenerate.

## Writing tests

All tests live in `test/` as external `_test` packages (e.g. `package handler_test`).
This keeps test dependencies out of production imports and makes the public API explicit.

**Handler tests:**
- Use `service.NewXxxService(db)` + `handler.NewXxxHandler(svc)` — construct the full real stack
- Never mock services in handler tests; use a real SQLite `:memory:` DB instead
- Each test gets an isolated DB via a unique UUID DSN: `file:<uuid>?mode=memory&cache=shared&_loc=UTC`
- `_loc=UTC` is required so go-sqlite3 parses stored time strings back into `time.Time`
- Seed data via the `seedConversation` / `seedMessage` helpers in `testhelper_test.go`

**Service tests** (when added):
- Same SQLite pattern; call service methods directly, assert on returned values and DB state

**Unit tests** (model, config, agent):
- `model` tests: call `Value()` / `Scan()` directly — no DB needed
- `config` tests: write temp YAML files, call `config.Load(path)`
- `agent` tests: use `agent.NewHub(log)` + `h.Cap()` / `h.Len()` methods

**General rules:**
- No mocks — use real implementations backed by SQLite or in-memory structs
- Use `require.NoError` for setup failures that would make the test meaningless; `assert` for the actual assertions
- Run with `-race` before merging anything that touches goroutines (agent loop, Hub)

## Tech stack

| Layer | Choice |
|---|---|
| HTTP | `gin` |
| ORM | `gorm` (MySQL in prod, SQLite in tests) |
| DB migrations | `golang-migrate` |
| DI | `google/wire` (compile-time, generates `wire_gen.go`) |
| AI SDK | `anthropics/anthropic-sdk-go` |
| OIDC auth | `coreos/go-oidc/v3` + `golang.org/x/oauth2` — **deferred** (see `docs/deferred.md`) |
| MCP client | `mark3labs/mcp-go` — Phase 2 |
| Docker | `docker/docker/client` — Phase 6 |
| Scheduler | `robfig/cron/v3` — Phase 3 |
| Logging | `slog` |
| Config | `config.yaml` via `gopkg.in/yaml.v3` |

## Design docs

All implementation details live in `docs/`:

| File | Contents |
|---|---|
| `docs/ph0-foundation.md` | Current phase — config, DB, models, agent loop, handlers |
| `docs/ph1-tools-and-hooks.md` | Tool registry, hook chain, built-in tools |
| `docs/ph2-mcp.md` | MCP client, tool routing, OAuth token management |
| `docs/ph3-scheduled-reports.md` | Cron jobs, daily/weekly reports |
| `docs/ph4-sub-agent-scheduling.md` | Thread graph, dispatch, wake mechanism |
| `docs/ph5-file-ops-and-approval.md` | Sandbox, approval hook, audit log |
| `docs/ph6-visualization.md` | Docker deploy, reverse proxy |
| `docs/ph7-context-and-memory.md` | Context compression, MEMORY.md |
| `docs/deferred.md` | Features blocked on external dependencies |
| `docs/development-plan.md` | Full roadmap + architecture invariants |

## Doc-sync rule

**After every implementation session, update the relevant `docs/ph*.md` to reflect the actual code.**
Treat the docs as the ground truth for "what we decided to build and why" — if the implementation diverges from the plan, fix the doc, not just the code. Stale docs are worse than no docs.

## Key invariants (never break)

| # | Rule |
|---|---|
| A | `PRE_TOOL_USE hook` → write message history → execute tool (never reorder) |
| B | Thread marked `running` before goroutine launch (not after) |
| C | Thread cancellation: one short tx per thread, never a shared long tx |
| D | Sub-agent message IDs independently generated (never reuse user message ID) |
| E | `round_done` / `queue_drained` never dropped — Hub drains before pushing |
| F | Approval: write DB first, signal channel second |
| G | Sandbox path validated at handler layer, not hook layer |
| H | Audit log: record param keys only, never values |
