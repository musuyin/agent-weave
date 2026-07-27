# ph1 — Tool Infrastructure (工具+Hook)

**Goal**: Give the agent tool-calling ability with a safe hook chain.
**Prerequisite**: ph0 complete and running.

---

## 1.1 Tool Registry (`internal/tool/registry.go`)

Global map populated via `init()` functions in each tool file:

```go
type Handler func(ctx context.Context, params json.RawMessage) (string, error)

type ToolDef struct {
    Name        string
    Description string
    InputSchema  any    // Go struct → serialized to JSON Schema for Anthropic SDK
    Handler      Handler
}

var globalRegistry = map[string]ToolDef{}

func Register(name string, def ToolDef)
func Get(name string) (ToolDef, bool)
func All() []ToolDef
```

Tool result truncation (enforced inside `Handler` wrappers):
- Hard cap: 16 KB
- UTF-8 safe: never cut inside a multi-byte rune (scan back from byte 16384 to find a valid rune boundary)

## 1.2 Hook Chain (`internal/hook/`)

**hook.go** — interfaces:

```go
type ToolCallParams struct {
    Name   string
    Params json.RawMessage  // mutable — hooks may replace this
}

// PreHook runs before a tool call, synchronously and serially.
// Return error to abort the tool call.
// Mutate params.Params to change the arguments the tool sees.
type PreHook interface {
    RunPre(ctx context.Context, params *ToolCallParams) error
}

// PostHook runs after a tool call, asynchronously (goroutine + deep-copy context).
// Pure observer — return value is ignored.
type PostHook interface {
    RunPost(ctx context.Context, params ToolCallParams, result string, err error)
}

type Chain struct {
    pre  []PreHook
    post []PostHook
}

func NewChain(pre []PreHook, post []PostHook) *Chain

// FirePre runs all PreHooks serially. First error aborts the chain.
// Called BEFORE writing message history (invariant A).
func (c *Chain) FirePre(ctx context.Context, params *ToolCallParams) error

// FirePost spawns a goroutine per PostHook with a deep-copied context.
func (c *Chain) FirePost(ctx context.Context, params ToolCallParams, result string, err error)
```

Chain registration order (hard-coded in wire setup):
- PRE: `SecurityHook` → `ApprovalHook` (Phase 5)
- POST: `AuditHook`

**security_hook.go** — Phase 1 placeholder:
- Block tools on a hard-coded denylist (empty for now; populated in Phase 5 with file ops)
- Path traversal check deferred to Phase 5 handler layer

**audit_hook.go**:
```go
// Logs: timestamp, tool name, param keys (NOT values), success/error.
// Keys only — values may contain file content or secrets.
func (h *AuditHook) RunPost(ctx context.Context, params ToolCallParams, result string, err error)
```

## 1.3 Agent Loop Update (`internal/agent/loop.go`)

Extend Phase 0 loop to handle `tool_use` stop reason:

```
Each round:
  1. Check ctx cancellation
  2. Consume sub-agent completion events (Phase 4; no-op in Phase 1)
  3. Rebuild dynamic system prompt (layers 5+6)
  4. Call Anthropic API (streaming)
  5. Handle stop reason:
     - end_turn → wait for sub-agents or finish
     - tool_use → for each tool block:
         a. FirePre(hook chain)          ← BEFORE write (invariant A)
         b. Write tool_use to message history
         c. Execute tool
         d. Write tool_result to message history
         e. FirePost(hook chain)
  6. Compress context if needed (Phase 7; no-op here)
```

The critical ordering — `FirePre → write history → execute` — is enforced structurally (not by comment).

## 1.4 System Prompt Pipeline — All 6 Layers

Extend `buildSystemPrompt` to wire all layers:

**Static (built once at loop start):**
- Layer 1: core orchestrator instructions (read from `prompts/orchestrator.md`)
- Layer 2: tool list (passed via SDK `tools` param, not written into prompt text)
- Layer 3: skill metadata — names + descriptions only (Progressive Disclosure; full content loaded on `load_skill` tool call)
- Layer 4: project instructions (`AGENTHUB.md` from conversation's working dir, or empty)

**Dynamic (rebuilt each round):**
- Layer 5: long-term memory index (`MEMORY.md` summary lines, or empty)
- Layer 6: dynamic context (recent message summary + available agent list + current task graph state)
  - Task graph query **must** use `db.Session(&gorm.Session{NewDB: true})` to bypass GORM identity map

## 1.5 Built-in Tools (`internal/tool/builtin/`)

Each file has an `init()` that calls `registry.Register(...)`.

| Tool | File | Notes |
|---|---|---|
| `read_file` | `read_file.go` | Path validated in handler (sandbox check placeholder until Phase 5) |
| `list_directory` | `list_directory.go` | Returns JSON array of entries |
| `fetch_url` | `fetch_url.go` | HTTP GET, result truncated to 16 KB |
| `respond_to_user` | `respond_to_user.go` | Signals the loop to emit a final `message_appended` SSE event and end the turn |

## 1.6 Files to Create/Modify

**New:**
- `internal/tool/registry.go`
- `internal/tool/truncate.go` — 16 KB UTF-8-safe truncation helper
- `internal/hook/hook.go`
- `internal/hook/security_hook.go`
- `internal/hook/audit_hook.go`
- `internal/tool/builtin/read_file.go`
- `internal/tool/builtin/list_directory.go`
- `internal/tool/builtin/fetch_url.go`
- `internal/tool/builtin/respond_to_user.go`
- `prompts/orchestrator.md` — layer 1 system prompt text

**Modified:**
- `internal/agent/loop.go` — add tool dispatch, hook firing, 6-layer prompt
- `internal/agent/service.go` (or loop.go) — accept `*hook.Chain` dependency
- `cmd/server/wire.go` + `wire_gen.go` — add hook chain providers

---

## Architecture Invariants

| # | Invariant | Where enforced |
|---|---|---|
| A | `FirePre` → write message history → execute tool | `loop.go` tool dispatch block |
| H | Audit log: keys only, never values | `audit_hook.go` |
| Truncate | Tool results ≤ 16 KB, UTF-8 safe | `truncate.go` called by every handler |
| Layer 6 | Task graph via `NewDB:true` session | `buildDynamicPrompt` in loop.go |

---

## Verification

```bash
go build ./...
go test ./...

# smoke test: send a message that triggers read_file
curl -X POST /api/conversations/$ID/messages \
  -d '{"content": "List the files in the current directory"}'
# SSE stream should show block_delta tokens + tool_use block + tool_result
```
