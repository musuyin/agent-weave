# ph4b — Orchestrator Dispatch (1 → n → 1)

**Goal**: The orchestrator splits the user's request into tasks, dispatches them
to the subagents present in the chat, waits for all of them (errgroup fan-in),
then summarises the results back to the user.

Prerequisite: ph4a (agents/skills exist, `conversation_agents` populated,
`messages.agent_id` column present).

Status: **complete** (implemented in ph4b session).

---

## Flow

```
user → orchestrator
         │  splits into tasks, picks subagents by their `description`
         ├─ dispatch_to_agent(agent_a, task_1) ─┐
         ├─ dispatch_to_agent(agent_b, task_2) ─┤ run in parallel (errgroup)
         └─ dispatch_to_agent(agent_c, task_3) ─┘
                                                 │ all complete
         ← summary ← fan-in results
user ←
```

## Pieces

### `dispatch_to_agent` tool (`internal/tool/builtin/dispatch_to_agent.go`)
Params: `{agent_id, instruction}`. Handler:
1. Create a `Thread` row, set `running` **before** launching the goroutine (invariant B).
2. Launch the subagent runner in an `errgroup`.
3. On completion, mark the thread `done`/`error` in a short per-thread tx (invariant C).

### Subagent runner (`internal/agent/subagent.go`)
- Builds the subagent's system prompt: its `agents.prompt` + the bodies of all
  loaded skills (`agent_skills` → `skills.body`).
- Runs an LLM turn; persists produced messages with `agent_id` = that agent
  (never NULL, never the orchestrator).
- Message IDs generated independently (invariant D).

### Fan-in + summary
- Orchestrator round waits on the errgroup; when all subagents finish, their
  outputs are injected back into the orchestrator's context.
- Orchestrator emits a final summary message (`agent_id = NULL`).

### Membership gate
The orchestrator may only dispatch to agents in `conversation_agents` for the
current conversation.

### Cancellation
`DELETE /api/conversations/:id/threads` → mark all non-terminal threads
`cancelled`, one short tx each (invariant C).

---

## Implementation notes (actual code)

### Key design decisions made during implementation

**Import cycle avoidance**: `tool/builtin/dispatch_to_agent.go` accepts callback
types (`RunSubAgentFunc`, `AddDispatchFunc`, `ConvIDFromCtxFunc`) instead of
importing the `agent` package. `agent.ProvideAgentService` wires the closures
after constructing the `Service`, passing `svc.RunSubAgent` as the implementation.

**DispatchRegistry flow**: `addDispatch(convID)` is called inside the tool handler
(before the goroutine), and `reg.Done` is called from the `RunSubAgent` deferred
cleanup. The `RunSubAgentFunc` callback is called from a goroutine already launched
by the handler, so it does NOT use `go` again.

**Fan-in location**: After `end_turn` in `run.go`, if `dispatchReg.Pending > 0`:
wait → drain → persist synthetic `user` role message with results → `continue`.
The orchestrator loop then gets a new LLM turn to summarise.

**Dynamic system prompt**: `buildSystemPrompt(ctx, conversationID)` calls
`agentSvc.ListConversationAgents`. When agents are present, appends:
`## Available Subagents` section listing name, ID, description.

### New/modified files

| File | Change |
|---|---|
| `internal/agent/service.go` | Added `agentSvc`, `dispatchReg` fields; calls `builtin.RegisterDispatchTool` |
| `internal/agent/run.go` | `ctx = withConversationID(ctx, conversationID)` at top; `buildSystemPrompt(ctx, convID)`; fan-in after `end_turn` |
| `internal/agent/subagent.go` | `RunSubAgent` — single LLM turn, no tools, persists with `agentID` |
| `internal/agent/dispatch.go` | `DispatchRegistry` (WaitGroup + result accumulator) |
| `internal/agent/ctxkey.go` | `withConversationID` / `conversationIDFromCtx` |
| `internal/tool/builtin/dispatch_to_agent.go` | `RegisterDispatchTool` with callback types |
| `internal/service/thread.go` | `ThreadService.CancelAllThreads` (one tx per thread) |
| `internal/handler/thread.go` | `DELETE /api/conversations/:id/threads` → 204 |
| `internal/handler/router.go` | Added `threadH` parameter and route |
| `cmd/server/wire.go` + `wire_gen.go` | Added `NewDispatchRegistry`, `NewThreadService`, `NewThreadHandler` |
| `test/agent/dispatch_test.go` | Unit tests for `DispatchRegistry` |

| # | Invariant |
|---|---|
| B | Thread marked `running` before goroutine launch |
| C | Cancellation: per-thread short tx, never a shared long tx |
| D | Subagent message IDs independently generated |
| E | `round_done` / `queue_drained` never dropped |
