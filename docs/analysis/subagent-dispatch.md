# Analysis: How the Orchestrator Dispatches Tasks to Subagents

---

## 1. Overview

The orchestrator (the main agent loop) delegates tasks to subagents by calling the built-in tool
`dispatch_to_agent`. From the model's point of view this is no different from any other tool call.
The infrastructure underneath it launches a goroutine, runs a second LLM turn with a different
system prompt, and feeds the result back to the orchestrator as a synthetic message.

---

## 2. End-to-end flow

```
User message → orchestrator LLM
                │
                │  stop_reason = "tool_use"
                │  tool: dispatch_to_agent
                │  params: {agent_id, instruction}
                ▼
dispatchToolsFromBlocks()          (tool_dispatch.go:74)
  │
  └─► dispatch_to_agent handler    (builtin/dispatch_to_agent.go:50)
        │  1. validate agent exists + is in this conversation
        │  2. INSERT thread (status=running)      ← invariant B: before goroutine
        │  3. dispatchReg.Add(convID)             ← increment pending count
        │  4. go runSubAgent(...)                 ← goroutine launched
        │  5. return {"thread_id": "...", "agent": "..."}
        ▼
tool_result persisted → orchestrator loops back → calls LLM again
  │
  │  stop_reason = "end_turn"
  │  dispatchReg.Pending(convID) > 0  → subagents still running
  │
  └─► dispatchReg.Wait(convID)         ← block until all subagents call Done()
        results := dispatchReg.Drain()
        persist fan-in results as user message
        continue  → orchestrator loops back to summarize
```

---

## 3. The `dispatch_to_agent` tool handler (`builtin/dispatch_to_agent.go`)

```go
// 1. Validate agent membership (gate: must be in this conversation)
members, _ := agentSvc.ListConversationAgents(ctx, convID)
// ...membership check...

// 2. Create Thread with status=running BEFORE goroutine (invariant B)
thread := repository.Thread{
    ID:      uuid.NewString(),
    AgentID: params.AgentID,
    Status:  repository.ThreadStatusRunning,
    ...
}
db.Create(&thread)

// 3. Register before launch so Pending() is accurate immediately
addDispatch(convID)                          // dispatchReg.Add(convID)
go runSubAgent(ctx, convID, thread, params.Instruction)

// 4. Return thread_id to the orchestrator immediately
return `{"thread_id": "...", "agent": "..."}`, nil
```

The handler returns **before the subagent finishes**. The orchestrator gets a thread ID as the
tool result and the LLM is called again with that context while the subagent runs in parallel.

**Invariant B** is critical: if the goroutine were launched before `db.Create(&thread)`, a crash
between those two lines would leave the goroutine running with no DB record. Writing first means
the thread is always observable.

---

## 4. The subagent runner (`agent/subagent.go`)

```go
func (s *Service) RunSubAgent(ctx, conversationID, thread, instruction, hub, reg) {
    defer func() {
        // Update thread status (done/error) — short per-thread transaction (invariant C)
        // Push EventThreadStatus to SSE hub
        // reg.Done(conversationID, result)  ← unblocks fan-in Wait()
    }()

    // Load agent definition and skills from DB
    agentDef, _ := s.agentSvc.Get(ctx, agentID)
    skills, _   := s.agentSvc.ListSkills(ctx, agentID)

    // System prompt = agent.Prompt + skill bodies concatenated
    systemPrompt := agentDef.Prompt + "\n\n" + skills[0].Body + ...

    // History = shared conversation history + synthetic user turn with the instruction
    history, _ := s.loadHistory(ctx, conversationID)
    history = append(history, anthropic.NewUserMessage(
        anthropic.NewTextBlock(instruction),
    ))

    // Single LLM call — no tools (subagents are text-only)
    stream := s.aiClient.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
        System:   systemPrompt,
        Messages: history,
        // no Tools field
    })

    // Stream SSE deltas to the same Hub (same frontend connection sees subagent tokens)
    // Accumulate into accMsg

    // Persist with agentID tagged (invariant D: new UUID, not orchestrator's message ID)
    s.persistMessage(ctx, conversationID, "assistant", blocks, &agentID)
}
```

Key differences from the orchestrator's own LLM call:

| | Orchestrator (`run.go`) | Subagent (`subagent.go`) |
|---|---|---|
| System prompt | orchestrator instructions + tool list | agent.Prompt + skill bodies |
| Tools available | all built-in + MCP | none (text-only) |
| History | shared conversation | same shared history + injected instruction |
| Can dispatch further | yes (recursive) | no |
| Message tagged with | `agentID = nil` | `agentID = agent UUID` |

---

## 5. Fan-in (`run.go:97–106`)

After the orchestrator's LLM returns `end_turn`, the loop checks whether any subagents are still
running:

```go
if s.dispatchReg.Pending(conversationID) > 0 {
    s.dispatchReg.Wait(conversationID)              // block until all Done()
    results := s.dispatchReg.Drain(conversationID) // collect SubAgentResult slice
    fanInBlocks := subAgentResultsToBlocks(results) // convert to tool_result-like blocks
    s.persistMessage(ctx, conversationID, "user", fanInBlocks, nil)
    continue  // loop back — orchestrator sees results and can summarize
}
```

`DispatchRegistry` (`dispatch.go`) is the synchronization primitive:

```
dispatchReg.Add(convID)    ← called before each goroutine launch
dispatchReg.Done(convID)   ← called by each subagent in its defer
dispatchReg.Wait(convID)   ← blocks on a sync.WaitGroup until all Done()
dispatchReg.Drain(convID)  ← returns accumulated results, clears state
```

The orchestrator never directly talks to subagent goroutines. It only interacts with the registry.

---

## 6. What the frontend sees

All subagents share the same `Hub` as the orchestrator (same conversation). The frontend receives
SSE events interleaved:

```
EventAgentStart         ← orchestrator starts
EventBlockDelta (...)   ← orchestrator thinking
EventThreadStatus       ← "subagent X is running"
EventBlockDelta (...)   ← subagent tokens (streamed live)
EventThreadStatus       ← "subagent X done"
EventBlockDelta (...)   ← orchestrator summary (after fan-in)
EventRoundDone
EventQueueDrained
```

The `EventThreadStatus` event carries `thread_id`, `status`, and `agent_name` so the frontend
can show which subagent produced which output.

---

## 7. Summary of invariants enforced

| Invariant | Where | What |
|---|---|---|
| B | `dispatch_to_agent.go:100` | Thread written to DB before goroutine launched |
| C | `subagent.go:42` | Thread status update is a short per-thread transaction |
| D | `history.go:54` | `persistMessage` always generates a new UUID — subagent never reuses orchestrator message IDs |

---

## 8. One-line summary of each file

| File | Role |
|---|---|
| `builtin/dispatch_to_agent.go` | Tool handler — validates, creates Thread, launches goroutine |
| `agent/dispatch.go` | `DispatchRegistry` — WaitGroup + result collection per conversation |
| `agent/subagent.go` | `RunSubAgent` — single LLM turn for one subagent, SSE streaming, DB persist |
| `agent/run.go:97–106` | Fan-in — wait for all subagents, feed results back to orchestrator |
| `agent/service.go:64–73` | Wires `RunSubAgent` into `dispatch_to_agent` at startup |
