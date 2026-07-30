# ph4b — Orchestrator Dispatch (1 → n → 1)

**Goal**: The orchestrator splits the user's request into tasks, dispatches them
to the subagents present in the chat, waits for all of them (errgroup fan-in),
then summarises the results back to the user.

Prerequisite: ph4a (agents/skills exist, `conversation_agents` populated,
`messages.agent_id` column present).

Status: **future** (not yet implemented).

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

## Invariants (from the original doc, revived here)

| # | Invariant |
|---|---|
| B | Thread marked `running` before goroutine launch |
| C | Cancellation: per-thread short tx, never a shared long tx |
| D | Subagent message IDs independently generated |
| E | `round_done` / `queue_drained` never dropped |
