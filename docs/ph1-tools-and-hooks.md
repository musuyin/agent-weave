# ph1 — Tool Infrastructure (工具+Hook)

**Goal**: Give the agent tool-calling ability with a safe hook chain.
**Status**: ✓ Complete.

---

## 1.1 Tool Registry (`internal/tool/registry.go`)

Global map protected by `sync.RWMutex`. Populated via `init()` in each tool file.

```go
type Handler func(ctx context.Context, params json.RawMessage) (string, error)

type ToolDef struct {
    Name        string
    Description string
    InputSchema any     // must be map[string]any JSON Schema with "properties" / "required"
    Handler     Handler
}

func Register(def ToolDef)
func Get(name string) (ToolDef, bool)
func All() []ToolDef
```

**`InputSchema` contract:** must be a `map[string]any` JSON Schema object. Using a plain Go struct will silently drop the tool from `buildToolParams` (type assertion fails). Example:

```go
InputSchema: map[string]any{
    "type": "object",
    "properties": map[string]any{
        "path": map[string]any{
            "type":        "string",
            "description": "Path to the file.",
        },
    },
    "required": []string{"path"},
},
```

Tool result truncation helper: `tool.Truncate(s string, maxBytes int) string` in `truncate.go`.
- Hard cap: `MaxToolResultBytes = 16 * 1024`
- UTF-8 safe: scans back from `maxBytes` to find a valid rune start byte

## 1.2 Hook Chain (`internal/hook/`)

### hook.go — interfaces and Chain

```go
type ToolCallParams struct {
    Name   string
    Params json.RawMessage  // mutable — PreHooks may replace this
}

type PreHook interface {
    RunPre(ctx context.Context, params *ToolCallParams) error
}

type PostHook interface {
    RunPost(ctx context.Context, params ToolCallParams, result string, err error)
}

type Chain struct { pre []PreHook; post []PostHook }

func NewChain(pre []PreHook, post []PostHook) *Chain
func (c *Chain) FirePre(ctx context.Context, params *ToolCallParams) error
    // Runs serially. First error aborts — subsequent hooks not called.
func (c *Chain) FirePost(ctx context.Context, params ToolCallParams, result string, err error)
    // Spawns one goroutine per PostHook. Non-blocking.
```

### security_hook.go

Blocks tool calls whose name appears in a denylist (empty in Phase 1).

```go
func NewSecurityHook(denied []string) *SecurityHook
func ProvideSecurityHook() *SecurityHook    // Wire provider — empty denylist
func (h *SecurityHook) RunPre(_ context.Context, params *ToolCallParams) error
```

Returns `*ErrToolDenied{Name}` when blocked. Phase 5 populates the denylist for file ops.

### audit_hook.go

Logs tool invocations: name, param **keys only** (never values — invariant H), and outcome.

```go
func NewAuditHook(log *slog.Logger) *AuditHook
func (h *AuditHook) RunPost(_ context.Context, params ToolCallParams, _ string, err error)
```

### Wire assembly

```go
// ProvideHookChain in audit_hook.go
func ProvideHookChain(sec *SecurityHook, audit *AuditHook) *Chain {
    return NewChain(
        []PreHook{sec},           // Pre: SecurityHook → (Phase 5: ApprovalHook)
        []PostHook{audit},        // Post: AuditHook
    )
}
```

## 1.3 Agent Loop (`internal/agent/`)

`loop.go` has been split into four focused files:

| File | Contents |
|---|---|
| `service.go` | `Service` struct, `ProvideAgentService`, `Run` (public entry point) |
| `run.go` | `run` loop, `buildSystemPrompt`, `buildToolParams` |
| `tool_dispatch.go` | `dispatchTools`, `dispatchToolsFromBlocks`, `buildToolParams`, `DispatchToolsForTest` |
| `history.go` | `loadHistory`, `persistMessage`, `ProvideDB` |

### Run loop (`run.go`)

```
Each round (infinite loop until end_turn or error):
  1. ctx.Err() check — return early (Run wrapper pushes round_done+queue_drained)
  2. push agent_start (first round only — pushed once before loop)
  3. loadHistory() from DB
  4. NewStreaming() with buildSystemPrompt() + buildToolParams() + history
  5. Stream events → push block_start / block_delta / block_stop SSE
  6. Check stop_reason:
       end_turn  → persist assistant message, push round_done + queue_drained, return
       tool_use  → dispatchTools(), loop back
       default   → log warn, persist text if any, push round_done + queue_drained, return
```

### Tool dispatch (`tool_dispatch.go`) — Invariant A

```
dispatchToolsFromBlocks(ctx, conversationID, assistantBlocks):
  1. Collect tool_use blocks from assistantBlocks into pending list
  2. persistMessage("assistant", assistantBlocks)  ← ONCE, BEFORE any hook fires
  3. for each pending tool:
       a. FirePre(chain, &params)                  ← AFTER history write
       b. Execute tool (or record denial/unknown)
       c. Append tool_result block
       d. FirePost(chain, params, result, auditErr) ← auditErr = preErr ?? toolErr
  4. persistMessage("user", resultBlocks)           ← single message per Anthropic requirement
```

**`DispatchToolsForTest`** is a package-level export that allows external `_test` packages to call `dispatchToolsFromBlocks` directly for invariant testing.

### System prompt (`run.go: buildSystemPrompt`)

Layer 1: `prompts.Orchestrator` (embedded from `internal/prompts/orchestrator.md`)
Layer 3: dynamic `## Tool Reference` section appended from `tool.All()`
Layers 2, 4–6: deferred (no-op placeholders)

Tool list is **not** hardcoded in `orchestrator.md` — the dynamic section is authoritative.

### Tool schema (`tool_dispatch.go: buildToolParams`)

`InputSchema` is cast with `.(map[string]any)` — tools with non-map schemas are silently skipped. `properties` and `required` are extracted and passed to `anthropic.ToolInputSchemaParam`.

## 1.4 Built-in Tools (`internal/tool/builtin/`)

Each file has an `init()` that calls `tool.Register(...)`. Import the package via blank import in `run.go`:

```go
import _ "github/musuyin/agent-weave/internal/tool/builtin"
```

| Tool | File | Status |
|---|---|---|
| `read_file` | `read_file.go` | Stub — returns sandbox-not-available message; Phase 5 will implement |
| `list_directory` | `list_directory.go` | Stub — same |
| `fetch_url` | `fetch_url.go` | Implemented — HTTP GET, 30 s timeout (uses caller ctx), result truncated to 16 KB |

`respond_to_user` is **not implemented** — deferred; the loop ends on `end_turn` stop reason instead.

### `fetch_url` details

- Validates scheme: `http` or `https` only; returns error string for others
- Timeout: `context.WithTimeout(ctx, 30*time.Second)` — uses caller's context as parent so agent cancellation propagates
- Reads `MaxToolResultBytes+1` bytes via `io.LimitReader`, then calls `tool.Truncate`
- Network/HTTP errors returned as error strings (not Go errors) — model sees them as tool results

## 1.5 System Prompt (`internal/prompts/`)

```go
//go:embed orchestrator.md
var Orchestrator string
```

`orchestrator.md` contains the static layer 1 text. Tool list is **not** in this file — it is appended dynamically by `buildSystemPrompt`.

## 1.6 Wire DI updates (`cmd/server/`)

New providers added to `wire.go`:

```go
wire.Build(
    ...
    hook.ProvideSecurityHook,   // → *SecurityHook
    hook.NewAuditHook,          // → *AuditHook
    hook.ProvideHookChain,      // (*SecurityHook, *AuditHook) → *Chain
    agent.ProvideAgentService,  // now accepts *hook.Chain
    ...
)
```

---

## Architecture Invariants

| # | Rule | Where enforced |
|---|---|---|
| A | assistant message persisted **once before** any `FirePre` fires; `FirePre` runs after history write, before execution | `tool_dispatch.go:67–77` |
| H | Audit log records param **keys only**, never values | `audit_hook.go:paramKeys` |
| Truncate | Tool results ≤ 16 KB, UTF-8 safe | `truncate.go`, called in `fetch_url.go` |

---

## File list

| File | Key exports |
|---|---|
| `internal/tool/registry.go` | `ToolDef`, `Handler`, `Register`, `Get`, `All` |
| `internal/tool/truncate.go` | `Truncate`, `MaxToolResultBytes` |
| `internal/tool/builtin/fetch_url.go` | registers `fetch_url` |
| `internal/tool/builtin/read_file.go` | registers `read_file` (stub) |
| `internal/tool/builtin/list_directory.go` | registers `list_directory` (stub) |
| `internal/hook/hook.go` | `ToolCallParams`, `PreHook`, `PostHook`, `Chain`, `NewChain`, `FirePre`, `FirePost` |
| `internal/hook/security_hook.go` | `SecurityHook`, `ErrToolDenied`, `ProvideSecurityHook` |
| `internal/hook/audit_hook.go` | `AuditHook`, `NewAuditHook`, `ProvideHookChain` |
| `internal/agent/service.go` | `Service`, `ProvideAgentService`, `Run` |
| `internal/agent/run.go` | `run`, `buildSystemPrompt` |
| `internal/agent/tool_dispatch.go` | `dispatchTools`, `dispatchToolsFromBlocks`, `buildToolParams`, `DispatchToolsForTest` |
| `internal/agent/history.go` | `loadHistory`, `persistMessage`, `ProvideDB` |
| `internal/prompts/prompts.go` | `Orchestrator` (embedded) |
| `internal/prompts/orchestrator.md` | layer 1 system prompt text |

---

## Tests

| File | Coverage |
|---|---|
| `test/tool/truncate_test.go` | no-op, exact limit, ASCII cut, UTF-8 boundary, MaxToolResultBytes cap |
| `test/hook/hook_test.go` | `FirePre` abort chain, `AuditHook` keys-only log, failure log |
| `test/agent/invariant_test.go` | `TestDispatchTools_InvariantA` (2-tool response, assistant row in DB before both FirePre calls), `TestFetchURL_ContextCancellation` |

---

## Verification

```bash
go build ./...
go test ./...
go test ./test/... -race
```
