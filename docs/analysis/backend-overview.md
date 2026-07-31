# Agent Weave — Backend Technical Analysis

> Purpose: Resume-oriented summary of technologies, patterns, and engineering decisions used in the backend.

---

## Core Tech Stack

- **Go 1.25** — backend language
- **Gin** — HTTP framework; routing, JSON binding, SSE streaming
- **GORM** — ORM with MySQL in production, SQLite in tests
- **google/wire** — compile-time dependency injection; generates `wire_gen.go`
- **golang-migrate** — SQL schema migration runner; applied automatically at server startup
- **Anthropic Go SDK** — streaming LLM calls with structured tool use
- **MCP Go SDK (`modelcontextprotocol/go-sdk`)** — MCP client over Streamable HTTP transport
- **MySQL** — primary persistence store
- **SQLite (tests only)** — each test gets an isolated in-memory DB via unique UUID DSN

---

## Architecture Highlights

### Layered HTTP → Service → GORM

- **Handlers** are pure HTTP adapters: bind request → call service → write status + JSON. Zero business logic.
- **Services** own all business rules and call GORM directly (`db.WithContext(ctx)`). No separate repository layer.
- **Models** split cleanly into `repository` (GORM models with custom JSON value/scanner) and `dto` (Gin binding structs).

### Compile-time Dependency Injection (Wire)

Full provider graph declared in `cmd/server/wire.go`; generated output committed to `wire_gen.go`. Resource lifecycle managed via `(T, func(), error)` triples — cleanup functions are chained through the graph and called on shutdown.

### Agent Loop

The core of the system is an orchestrator-driven `while` loop:
- Loads conversation history from DB, triggers context compaction if needed
- Builds a layered system prompt (static layers constructed once; dynamic layers rebuilt each round)
- Issues a streaming LLM call via the Anthropic SDK
- On `tool_use`: enforces strict execution order — persist assistant message → fire pre-hooks → execute tool → persist result — before looping
- On `end_turn`: waits for any dispatched subagents via a fan-in registry, then exits
- Runs detached from the HTTP request context so it completes even if the SSE client disconnects

### SSE Streaming

Agent output is pushed to the frontend over Server-Sent Events. A `Hub` (buffered channel, cap 256) per conversation decouples the agent goroutine from the HTTP handler. A `HubRegistry` (RWMutex-protected map) brokers access by conversation ID. Terminal signals (`round_done`, `queue_drained`) use a drain-then-push invariant — the buffer is fully drained before the new event is enqueued, guaranteeing delivery even when the buffer is full.

### Subagent Dispatch

The `dispatch_to_agent` tool lets the orchestrator fan out work to specialized agents concurrently. Each subagent runs in its own goroutine, streams results to the shared SSE hub, and reports completion via a `DispatchRegistry` (per-conversation WaitGroup + result accumulator). The orchestrator waits for all pending subagents before exiting the loop. Subagents are text-only — they cannot call tools — ensuring all high-risk operations remain gated through the orchestrator.

### Hook System

A middleware chain wraps every tool call:
- **Pre-hooks** (synchronous, serial): can abort a tool call or modify its parameters. Chain: `SecurityHook → ApprovalHook`.
- **Post-hooks** (fire-and-forget goroutines): purely observational. `AuditHook` logs tool name and parameter key names only, never values.

### Context Compaction

When a conversation reaches ≥ 40 non-compacted messages, the middle slice is summarized by a dedicated LLM call and replaced with a single synthetic message. The compaction prompt passes both the middle (candidates) and the live tail (context), letting the LLM drop already-resolved content. Compacted rows are marked `compacted = 1` in DB and excluded from all future loads. Non-fatal: any failure falls through to the full history.

### MCP Tool Routing

At startup, the server enumerates all configured MCP servers and builds a flat `toolName → MCPClient` dispatch table. Builtin tools are registered via package `init()` functions. The agent loop presents the merged tool list to the LLM; `dispatchToolsFromBlocks` routes each call to either MCP or the builtin registry. Unreachable MCP servers are skipped at startup with a warning rather than aborting.

---

## Engineering Decisions Worth Highlighting

### Import Cycle Avoidance via Callback Injection

`dispatch_to_agent` (in `tool/builtin`) needs to call `agent.Service`. A direct import creates a cycle (`agent → tool/builtin → agent`). Solved by injecting three function-type parameters (`RunSubAgentFunc`, `AddDispatchFunc`, `ConvIDFromCtxFunc`) into `RegisterDispatchTool` at startup from `agent.ProvideAgentService`. The concrete implementations are closures capturing `*Service`.

### Race-Safe Find-or-Create

System skill and agent seeding uses an optimistic pattern: try-find → try-create → on duplicate-key re-fetch. A single `isDuplicateKeyError` helper matches both MySQL error 1062 and SQLite's "UNIQUE constraint failed" string, making the same service code work in production and tests without any conditional logic.

### Dual Wire Providers for Services with Init

Services with startup side effects (skill seeding, agent seeding, report conversation pre-creation) expose two constructors: `NewXxxService(db)` for tests (no seeding) and `ProvideXxxService(ctx, db)` for production (runs `Init()`). Tests use the plain constructor, keeping test data fully under test control.

### Typed Context Keys

Context values use unexported typed constants (`type ctxKey int`) to prevent collision with values from other packages — applied consistently for the conversation ID passed through tool handler context.

### is_system Guard

System-seeded rows (skills, agents) carry an `is_system` flag. Update and Delete check this flag and return `ErrSystemReadOnly`, mapped to HTTP 400 by a centralized `respondServiceError` helper.

### Compile-time Data Embedding

Seeding data (orchestrator system prompt, skill markdown files, agent YAML definitions) is embedded at compile time via `//go:embed`. The seeding package's `init()` parses and populates module-level vars at program start, panicking on malformed files — treating seed corruption as a development-time error rather than a runtime condition.

---

## Testing Approach

- **No mocks** — all tests use real implementations
- Handler tests construct the full stack (`service + handler`) backed by an isolated SQLite `:memory:` DB
- Each test gets its own DB via a unique UUID DSN (`file:<uuid>?mode=memory&cache=shared&_loc=UTC`); `_loc=UTC` is required for go-sqlite3 to parse stored time strings back to `time.Time`
- Agent and hook tests use real in-memory structs and channels
- Race detector (`-race`) used for all goroutine-touching packages
- All tests live in `test/` as external `_test` packages, enforcing public-API-only access and keeping test dependencies out of production imports
