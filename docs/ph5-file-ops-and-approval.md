# ph5 — File Operations + Approval (文件操作+审批)

**Goal**: Agent can read/write files in a sandboxed directory, with human approval for writes.
**Prerequisite**: ph1 complete (hook chain). ph4 optional.

---

## 5.1 Sandbox Path Model

Every conversation has an isolated sandbox at `sandbox/{conversation_id}/`.

All file operation handlers enforce the boundary **at handler layer** (not hook layer — security must not rely on hooks):

```go
// ValidateSandboxPath resolves the path and checks it stays inside the sandbox.
// Returns cleaned absolute path or error.
func ValidateSandboxPath(conversationID, userPath string) (string, error) {
    base := filepath.Join("sandbox", conversationID)
    abs := filepath.Clean(filepath.Join(base, userPath))
    rel, err := filepath.Rel(base, abs)
    if err != nil || strings.HasPrefix(rel, "..") {
        return "", ErrPathOutsideSandbox
    }
    return abs, nil
}
```

This runs before any hook. Even if all hooks are bypassed, the path check still fires.

## 5.2 File Operation Tools

**`read_file`** (already in ph1 — sandbox check is a no-op placeholder there; activate it here):
- Validate path via `ValidateSandboxPath`
- Read and return content (truncated to 16 KB)

**`create_file`** (`internal/tool/builtin/create_file.go`):
```go
type CreateFileParams struct {
    Path    string `json:"path"`
    Content string `json:"content"`
}
```
- Validate sandbox path
- Triggers ApprovalHook (high-risk)
- `os.MkdirAll` + `os.WriteFile`

**`edit_file`** (`internal/tool/builtin/edit_file.go`):
```go
type EditFileParams struct {
    Path    string `json:"path"`
    OldStr  string `json:"old_str"`
    NewStr  string `json:"new_str"`
}
```
- Validate sandbox path
- Triggers ApprovalHook (high-risk)
- Read → replace first occurrence → write back
- Push diff as `message_appended` SSE event (not `block_start`)

## 5.3 ApprovalHook (`internal/hook/approval_hook.go`)

Fires in PRE_TOOL_USE chain for `create_file`, `edit_file`, `deploy_app`.

**Single-approval flow**:
```go
func (h *ApprovalHook) RunPre(ctx context.Context, params *ToolCallParams) error {
    if !isHighRiskTool(params.Name) {
        return nil
    }
    if isBatchApproved(ctx) {
        return nil  // batch pass-through flag set in context
    }

    blockID := uuid.NewString()

    // 1. Write DB record FIRST (fail-closed: if this fails, reject)
    err := h.db.Create(&model.Approval{ID: blockID, Status: "pending", ...}).Error
    if err != nil {
        return ErrApprovalDBFailed  // fail-closed
    }

    // 2. Push SSE approval_requested event (after DB write)
    h.hub.Push(SSEEvent{Type: EventApprovalRequested, Data: ApprovalData{BlockID: blockID, ...}})

    // 3. Register channel and wait (120 s timeout → auto-reject)
    ch := h.registerPending(blockID)
    defer h.unregisterPending(blockID)  // always cleanup (invariant: no memory leak)

    select {
    case decision := <-ch:
        if decision == "approved" {
            return nil
        }
        return ErrApprovalRejected
    case <-time.After(120 * time.Second):
        return ErrApprovalTimeout
    case <-ctx.Done():
        return ctx.Err()
    }
}
```

**Approval decision API** (`internal/api/approval.go`):
```
POST /api/conversations/:id/approvals/:block_id
Body: {"decision": "approved" | "rejected"}
```

Handler:
```go
// 1. Write decision to DB FIRST
err := db.Model(&approval).Update("status", decision).Error
if err != nil {
    return 500  // fail-closed: don't signal if DB write failed
}
// 2. Signal channel SECOND
h.hook.Signal(blockID, decision)
```

**Ordering invariant F enforced structurally**: DB write comes before channel signal in a single sequential code path with no way to invert.

## 5.4 Batch Approval

When the same round contains **≥2 file write operations**:

1. ApprovalHook detects this by checking context for `pendingWriteCount >= 2`
2. Merges all into a single `approval_requested` SSE event listing all files
3. On approval: set `batchApproved = true` in context (a context value key)
4. Subsequent `RunPre` calls check `isBatchApproved(ctx)` and skip individual approval

```go
type contextKey int
const batchApprovedKey contextKey = iota

func WithBatchApproved(ctx context.Context) context.Context {
    return context.WithValue(ctx, batchApprovedKey, true)
}

func isBatchApproved(ctx context.Context) bool {
    v, _ := ctx.Value(batchApprovedKey).(bool)
    return v
}
```

## 5.5 AuditHook Enhancement (`internal/hook/audit_hook.go`)

Already created in ph1. In Phase 5, add structured DB logging:

```go
// AuditLog record written for every tool call (POST hook — async goroutine):
type AuditLog struct {
    ID             string    // UUID
    ConversationID string
    ToolName       string
    ParamKeys      []string  // JSON array of input param KEYS only — never values
    Success        bool
    ErrorMessage   string
    CreatedAt      time.Time
}
```

DB migration: `000006_create_audit_logs.up.sql`

## 5.6 Diff SSE Push

After `edit_file` completes, push the diff as a `message_appended` event (NOT `block_start`):

```go
diff := computeUnifiedDiff(oldContent, newContent, path)
hub.Push(SSEEvent{
    Type: EventMessageAppended,
    Data: MessageAppendedData{
        Role: "system",
        Content: []ContentBlock{{Type: "code", Language: "diff", Text: diff}},
    },
})
```

Using `message_appended` (not `block_start`) keeps the diff as an independent message bubble, not embedded inside the current streaming assistant response.

## 5.7 Files to Create/Modify

**New:**
- `internal/tool/builtin/create_file.go`
- `internal/tool/builtin/edit_file.go`
- `internal/hook/approval_hook.go`
- `internal/api/approval.go` — decision endpoint
- `db/migrations/000006_create_audit_logs.{up,down}.sql`

**Modified:**
- `internal/tool/builtin/read_file.go` — activate sandbox check
- `internal/hook/audit_hook.go` — add DB logging
- `internal/hook/security_hook.go` — activate path traversal check
- `internal/api/router.go` — add approval route
- `cmd/server/wire.go` + `wire_gen.go`

---

## Architecture Invariants

| # | Invariant | Where enforced |
|---|---|---|
| F | Approval: write DB → then signal channel | `approval.go` decision handler |
| Fail-closed | DB write fail → reject, never allow | `ApprovalHook.RunPre` |
| G | Sandbox path validated at handler layer | `ValidateSandboxPath` in each tool handler |
| H | Audit log: keys only | `AuditHook.RunPost` |
| No-leak | `defer unregisterPending(blockID)` always runs | `ApprovalHook.RunPre` defer |

---

## Verification

```bash
# 1. Single file write — approval bubble should appear in SSE
#    curl approval endpoint with "approved" → file created
#    curl approval endpoint with "rejected" → tool returns error

# 2. Two file writes in one round — single merged approval bubble

# 3. Path traversal attempt: path="../../../etc/passwd" → 400 error immediately
#    (before any hook fires)

# 4. DB write failure simulation → tool rejected (fail-closed), no hang
```
