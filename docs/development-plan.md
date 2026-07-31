# Agent Weave — Development Plan

> Full spec: `docs/项目重写说明.md`.

---

## Implementation Roadmap

```
Phase 0  基础骨架          ✓ complete
Phase 1  工具+Hook         ✓ complete
Phase 2  MCP 接入          ✓ complete
Phase 3  手动报告          ✓ complete
Phase 4  多 Agent 平台     ✓ complete (ph4a + ph4b; ph4c frontend deferred)
Phase 7  上下文压缩        ✓ complete (compaction; long-term memory deferred)
Phase 5  文件操作+审批     deferred (see docs/deferred.md)
Phase 6  命令驱动可视化    deferred (see docs/deferred.md)
```

---

## Phase 0 — Foundation (基础骨架)

**Goal**: Runnable server — streaming text chat, persistent messages.

---

## Phase 1 — Tool Infrastructure (工具+Hook)

1. **Tool Registry** — global map, `init()`-based registration
   - `ToolDef{Name, Description, InputSchema, Handler}`
   - `InputSchema` is a Go struct → serialized to JSON Schema for Anthropic SDK
2. **Hook chain**
   - `PRE_TOOL_USE`: sync, serial, can abort (return error) or mutate params
   - `POST_TOOL_USE`: async goroutine, deep-copy context, pure observer
   - Chain order: `SecurityHook` (PRE), `AuditHook` (POST)
3. **Basic built-in tools**: `read_file`, `list_directory`, `fetch_url`
4. **Tool result truncation** — 16 KB cap, UTF-8 safe

---

## Phase 2 — MCP (MCP 接入)

1. **MCPClient interface** — HTTP Streamable transport (stdio/SSE deferred)
2. **Tool routing** — `prefix__toolname` separator; `Route(name)` → `(Client, strippedName, ok)`
3. **GitHub prefix deduplication** — `github-tools__<tool>` / `github-wdf__<tool>`
4. **Static PAT auth** — `Authorization: Bearer <token>` in `config.yaml` headers (OAuth token refresh deferred)
5. **Servers**: github-tools, github-wdf (+ Jira when network allows)

---

## Phase 3 — Manual Reports (手动报告)

1. `POST /api/reports/:type/run` — triggers an on-demand report (`daily` | `weekly`)
2. Fixed conversations by well-known title (`daily-report`, `weekly-report`), pre-created at startup
3. Date-stamped prompts built at request time; agent uses GitHub MCP tools to fetch activity
4. Streamed live over SSE; no scheduler, no cron

---

## Phase 4 — Multi-Agent Platform (多 Agent 平台)

A conversation is a multi-agent chat: an implicit built-in orchestrator plus zero or more
user-added subagents. The orchestrator splits requests and dispatches tasks (1 → n → 1).

### ph4a — Agent & Skill Hubs ✓
- `skills`, `agents`, `agent_skills`, `conversation_agents` tables + migrations 000004–000008
- `messages.agent_id` (nullable FK; NULL = orchestrator)
- SkillService + AgentService with `is_system` guard + startup seeding
- CRUD routes: `/api/skills`, `/api/agents`, `/api/agents/:id/skills`, `/api/conversations/:id/agents`
- System defaults: `code-review-guidelines` + `go-style` skills; `code-reviewer` agent

### ph4b — Dispatch + Fan-in ✓
- `dispatch_to_agent` tool: creates Thread (status=running before goroutine, invariant B), launches `RunSubAgent`
- `DispatchRegistry`: WaitGroup + result accumulator per conversation
- Fan-in in `run.go`: after `end_turn`, if pending > 0 → Wait → Drain → persist synthetic user message → continue loop
- Dynamic system prompt: `## Available Subagents` section lists agents in conversation
- `DELETE /api/conversations/:id/threads` — cancel all non-terminal threads (one short tx each, invariant C)
- Import cycle avoided via callback types (`RunSubAgentFunc`, `AddDispatchFunc`, `ConvIDFromCtxFunc`)

### ph4c — Agent Hub UI
Frontend pages for skill/agent hubs and per-chat agent selection. Deferred to `web/` workspace.

---

## Phase 7 — Context Compaction

**Pinned head + compacted middle + live tail** strategy. Triggered at ≥ 40 non-compacted messages.

- `messages.compacted` column (migration 000009); rows with `compacted=1` excluded from `loadHistory`
- `maybeCompact` in `history.go`: slices msgs into head (4) + middle + tail (10), calls LLM to summarise middle
- Summary persisted as a `role=user` message; middle rows batch-updated to `compacted=1`
- Non-fatal: compaction failure falls through with full history (agent loop never blocked)
- Constants exported: `CompactThreshold=40`, `PinnedHead=4`, `LiveTail=10`

Long-term memory (`save_memory`/`read_memory`, MEMORY.md index) deferred.

---

## Key Architecture Invariants (never break)

| # | Invariant |
|---|---|
| A | `PRE_TOOL_USE hook` → `write message history` → `execute tool` (never reorder) |
| B | Thread marked running **before** goroutine launch |
| C | Thread cancellation uses per-thread short transactions (never a shared long tx) |
| D | Subagent message IDs are independent (never reuse user message ID) |
| E | `round_done` / `queue_drained` are never dropped (drain queue, then push) |
| F | Approval: write DB first, signal channel second *(ph5, when implemented)* |
| G | Sandbox path validated at handler layer, not hook layer *(ph5, when implemented)* |
| H | Audit log: keys only, never values |
