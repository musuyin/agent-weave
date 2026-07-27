# ph4 — Sub-Agent Scheduling (子 Agent 调度)

**Goal**: Orchestrator can spawn parallel sub-agents with dependency tracking.
**Prerequisite**: ph1 complete (tool registry). ph2 optional but recommended.

---

## 4.1 Thread Data Model

`threads` table already created in ph0. Phase 4 fully activates it.

Key fields:
- `agent_id = 'orchestrator'` — the main thread
- `agent_id = '<agent-name>'` — a sub-agent thread
- `status` — `pending | running | done | cancelled | error`
- `blocked_by` — JSON array of thread IDs that must be `done` before this thread starts

```go
// Thread lifecycle:
// pending → running (dispatch_to_agent called)
// running → done    (sub-agent Run() returns nil)
// running → error   (sub-agent Run() returns error)
// any     → cancelled (cancellation requested)
```

## 4.2 `dispatch_to_agent` Tool

Registered in `internal/tool/builtin/dispatch_to_agent.go`:

```go
type DispatchParams struct {
    AgentID     string   `json:"agent_id"`               // which agent to run
    Instruction string   `json:"instruction"`             // task description
    BlockedBy   []string `json:"blocked_by,omitempty"`   // thread IDs to wait for
}
```

Handler steps:
1. Create a new `Thread` row with status `pending`
2. **Immediately set status to `running`** (before goroutine launch) — invariant B
3. Launch goroutine: `go runSubAgent(ctx, thread, instruction)`
4. Register wake channel: when sub-agent completes, send signal to orchestrator
5. Return thread ID to the LLM

**Why mark running before launch (invariant B)**: if the goroutine is delayed (scheduler, GC pause), the orchestrator's task-graph check runs in the gap. If status is still `pending`, orchestrator sees "no active tasks" and incorrectly fires `round_done`.

## 4.3 Sub-Agent Execution (`internal/agent/subagent.go`)

```go
// runSubAgent executes a sub-agent thread.
// Sub-agents have NO tool-calling capability (security boundary).
// Their system prompt header explicitly states: "all real operations are
// executed by the orchestrator; you output text only."
func runSubAgent(ctx context.Context, thread model.Thread, instruction string, hub *Hub, db *gorm.DB, aiClient *anthropic.Client)
```

System prompt structure for sub-agents:
1. **Runtime header** (injected at execution time):
   - Agent ID, name, description
   - "No tool execution — text output only"
   - Sandbox path: `sandbox/{conversation_id}/`
   - Collaboration rules
2. Full skill content (not just metadata — sub-agents get the complete skill body)

Sub-agent **message IDs must be independently generated** (invariant D):
- Never reuse the user message ID that triggered the thread
- Frontend uses message ID as map key — reuse causes one message to overwrite another

## 4.4 Wake Mechanism

```go
// WakeRegistry maps conversationID → channel that receives sub-agent completion signals.
type WakeRegistry struct {
    mu      sync.RWMutex
    waiters map[string]chan SubAgentResult
}

type SubAgentResult struct {
    ThreadID string
    Summary  string
    Err      error
}

func (r *WakeRegistry) Register(conversationID string) <-chan SubAgentResult
func (r *WakeRegistry) Signal(conversationID string, result SubAgentResult)
```

Orchestrator loop step 2 (consume completions):
```go
// Non-blocking drain of all pending sub-agent results.
for {
    select {
    case result := <-wakeRegistry.Register(convID):
        // Inject summary into message history as a synthetic tool_result
        // Update thread status to "done" in DB
    default:
        goto doneConsuming
    }
}
doneConsuming:
```

## 4.5 Cancellation

Cancelling all threads for a conversation:

```go
// CancelConversationThreads marks all running/pending threads as cancelled.
// MUST use one short transaction per thread — NOT a single shared transaction.
// Reason: orchestrator's finally block holds a row lock on the main thread;
// a long transaction would trigger lock-wait timeout.
func CancelConversationThreads(ctx context.Context, db *gorm.DB, conversationID string) error {
    threads := // load all non-terminal threads
    for _, t := range threads {
        db.Transaction(func(tx *gorm.DB) error {  // separate short tx each
            return tx.Model(&t).Update("status", "cancelled").Error
        })
    }
}
```

## 4.6 API Extension

New endpoint:
```
DELETE /api/conversations/:id/threads   → cancel all active threads
```

SSE event `thread_status` already defined in ph0 — used to notify frontend of thread state changes:
```json
{"type": "thread_status", "data": {"thread_id": "...", "status": "done", "agent_id": "..."}}
```

## 4.7 `end_turn` Handling in Orchestrator Loop

When LLM returns `end_turn`:
```go
activeThreads := countActiveThreads(db.Session(&gorm.Session{NewDB: true}), convID)
if activeThreads > 0 {
    // park: wait on wake channel with timeout
    waitForSubAgentOrTimeout(wakeRegistry, convID, 30*time.Second)
    continue  // go to next round
}
// no active threads → truly done
hub.Push(SSEEvent{Type: EventRoundDone})
hub.Push(SSEEvent{Type: EventQueueDrained})
return nil
```

## 4.8 Files to Create/Modify

**New:**
- `internal/agent/subagent.go` — sub-agent runner + runtime header injection
- `internal/agent/wake.go` — `WakeRegistry`
- `internal/tool/builtin/dispatch_to_agent.go`
- `internal/api/thread.go` — cancel endpoint

**Modified:**
- `internal/agent/loop.go` — step 2 (consume events), `end_turn` parking, `CancelConversationThreads`
- `cmd/server/wire.go` + `wire_gen.go` — add `WakeRegistry` provider

---

## Architecture Invariants

| # | Invariant | Where enforced |
|---|---|---|
| B | Thread marked `running` before goroutine launch | `dispatch_to_agent` handler |
| C | Cancellation: per-thread short tx, not shared | `CancelConversationThreads` |
| D | Sub-agent message IDs independently generated | `runSubAgent` |
| Security | Sub-agents have no tool execution capability | System prompt header |

---

## Verification

```bash
# Send a message that asks the orchestrator to spawn a sub-agent:
# "Summarize the README using a sub-agent"

# Expected SSE stream:
# thread_status: {thread_id: X, status: "running", agent_id: "summarizer"}
# ... (sub-agent produces text output)
# thread_status: {thread_id: X, status: "done"}
# (orchestrator continues with summary injected)
# round_done
```
