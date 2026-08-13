# Phase 5 — File Operations + Approval

## Goal

Every high-risk tool call (file writes and shell execution) requires explicit user approval before
executing. Approvals are persisted to the database first, then signalled over an in-memory channel,
so the agent loop blocks until the user decides (or 120 s elapses and the call is auto-rejected).
All tool invocations are also logged to a structured `audit_logs` table (keys only, never values).

---

## Scope

### New tools

| Tool | Description |
|---|---|
| `edit_file` | Replace the first occurrence of `old_str` with `new_str` in a sandbox file |

(The existing `write_file` and `run_command` from ph8 are now gated by ApprovalHook.)

### High-risk tools (require approval)

`write_file`, `edit_file`, `run_command`

---

## Architecture

```
internal/hook/
  hook.go            ToolCallParams (+ BlockID), PushEventFunc, ConvIDFunc, RoundApprovalState
  approval_hook.go   ApprovalHook — PRE_TOOL_USE, DB record, SSE push, channel block
  audit_hook.go      AuditHook — POST_TOOL_USE, slog + audit_logs DB write

internal/handler/
  approval_handler.go  POST /api/conversations/:id/approvals/:block_id

internal/sandbox/
  container.go         + execRaw() + EditFile()
  manager.go           + edit_file tool registration

internal/model/repository/
  approval.go   Approval struct
  audit_log.go  AuditLog struct

db/migrations/
  000010_create_approvals.up/down.sql
  000011_create_audit_logs.up/down.sql
```

---

## Approval Flow

```
Agent calls write_file / edit_file / run_command
  │
  ▼
ApprovalHook.RunPre fires (PRE_TOOL_USE)
  │
  ├─ Check RoundApprovalState — if already approved/rejected this round, skip/deny
  │
  ├─ Create Approval{status: "pending"} in DB   ← fail-closed if DB fails
  │
  ├─ Store blockID → chan bool in sync.Map
  │
  ├─ Push SSE event: approval_requested {block_id, tool_name}
  │
  └─ Block on channel (120 s timeout → auto-reject)
         │
         ▼
   User calls POST /api/conversations/:id/approvals/:block_id
         │
         ├─ Write DB: status = "approved" | "rejected", decided_at = now   (invariant F: DB first)
         │
         └─ Signal channel   (invariant F: channel second)
                │
                ▼
         Hook receives decision → sets RoundApprovalState → returns nil or error
```

---

## Batch Approval (Round State)

When an agent round contains multiple high-risk tool calls (e.g., `write_file` + `run_command`):

1. `dispatchToolsFromBlocks` wraps `ctx` with a fresh `RoundApprovalState` before the loop.
2. First high-risk tool: creates approval record, blocks.
3. User approves → `RoundApprovalState.approved = true`.
4. Second high-risk tool: sees `approved = true` → runs without a new prompt.
5. If the first is rejected → `rejected = true` → all subsequent high-risk calls return an error.

This gives the user a single decision point per round rather than one prompt per tool.

---

## Database Tables

### `approvals`

| Column | Type | Notes |
|---|---|---|
| `block_id` | VARCHAR(64) PK | Anthropic tool_use block ID |
| `conversation_id` | VARCHAR(36) | FK to conversations |
| `tool_name` | VARCHAR(255) | |
| `status` | VARCHAR(20) | pending / approved / rejected |
| `created_at` | DATETIME | |
| `decided_at` | DATETIME nullable | Set on decision |

### `audit_logs`

| Column | Type | Notes |
|---|---|---|
| `id` | BIGINT AUTO_INCREMENT PK | |
| `conversation_id` | VARCHAR(36) | |
| `tool_name` | VARCHAR(255) | |
| `param_keys` | JSON | Key names only — never values (invariant H) |
| `success` | TINYINT(1) | |
| `error_message` | TEXT | |
| `created_at` | DATETIME | |

---

## Hook Changes

### `ToolCallParams` (hook/hook.go)

Added `BlockID string` — the Anthropic-assigned tool_use block ID, passed through from
`dispatchToolsFromBlocks` so the ApprovalHook can use it as the approval record key.

### `PushEventFunc` / `ConvIDFunc` (hook/hook.go)

Callback types defined in the hook package to avoid an import cycle with the agent package.
Injected via `Configure(...)` called from `agent.ProvideAgentService`.

### `RoundApprovalState` (hook/hook.go)

Mutex-protected `approved`/`rejected` booleans. Added to context via `hook.WithRoundApproval(ctx)`
before the dispatch loop. Unexported access helpers; unexported context key.

### `ProvideHookChain`

Updated signature: `ProvideHookChain(sec, audit, approval) *Chain`

Pre chain: `SecurityHook → ApprovalHook`
Post chain: `AuditHook`

### `AuditHook`

`NewAuditHook` now accepts `*gorm.DB`. `Configure(ConvIDFunc)` wires in the convID extractor.
`RunPost` continues logging to slog and additionally creates an `audit_logs` row using
`context.Background()` so DB writes survive request context cancellation.

---

## `edit_file` Tool

Registered by `sandbox.Manager.RegisterTools` alongside the ph8 tools.

**Input**: `{ path, old_str, new_str }`

**Implementation** (`Container.EditFile`):
1. Validate path (invariant I — same as all sandbox tools).
2. `execRaw` to read full file content (no 16 KB truncation — needed for correct replacement).
3. `strings.Replace(content, oldStr, newStr, 1)` — first occurrence only.
4. `WriteFile` to persist the new content.
5. Return `"ok: edited <path>"` or an error string.

`container.go` was refactored to expose `execRaw` (no truncation) used by `EditFile`, while
`exec` wraps it with `tool.Truncate` for user-visible outputs.

---

## Decision Endpoint

`POST /api/conversations/:id/approvals/:block_id`

Body: `{"decision": "approved" | "rejected"}`

- 204 No Content on success
- 400 if decision value is invalid
- 404 if no pending approval exists for the given (conv, block)
- 500 on DB error

---

## Wire Changes

New providers added to `wire.Build`:
- `hook.NewApprovalHook` — takes `*gorm.DB`
- `handler.NewApprovalHandler` — takes `(*gorm.DB, *hook.ApprovalHook)`

`ProvideAgentService` gains two new parameters: `*hook.ApprovalHook`, `*hook.AuditHook`.
`ProvideRouter` gains `*handler.ApprovalHandler`.

---

## Invariants Upheld

| # | Invariant | How |
|---|---|---|
| F | Approval: DB write first, signal second | `ApprovalHandler.Decide` writes DB before calling `Signal` |
| G | Sandbox path validated at handler layer | `validatePath` in `container.go` before every Docker call |
| H | Audit log: keys only, never values | `paramKeys` extracts map keys; values never stored |

---

## Verification

```bash
# Start server (Docker must be running)
cd server && go run ./cmd/server/

# In the web UI — create a conversation, then send:
# "Write a file at notes.txt with the content 'hello world'"

# Expected:
# 1. Agent calls write_file → approval_requested SSE event fires
# 2. Call POST /api/conversations/<id>/approvals/<block_id>  {"decision":"approved"}
# 3. write_file executes; agent responds with success

# Reject case:
# Call the endpoint with {"decision":"rejected"} instead
# Agent receives "tool denied: write_file was rejected"

# Check audit_logs:
# SELECT * FROM audit_logs ORDER BY created_at DESC LIMIT 5;

# All tests:
go test ./test/... -race
```
