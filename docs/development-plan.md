# Agent Weave — Development Plan

> Generated 2026-07-27. Full spec: `docs/项目重写说明.md`.
> Current state: greenfield (zero Go source files, bare `go.mod`).

---

## Implementation Roadmap

```
Phase 0  基础骨架
Phase 1  工具+Hook
Phase 2  MCP 接入
Phase 3  手动报告
Phase 4  多 Agent 平台
Phase 5  文件操作+审批
Phase 6  命令驱动可视化
Phase 7  上下文压缩+长期记忆
```

---

## Phase 0 — Foundation (基础骨架)

**Goal**: Runnable server — login, streaming text chat, persistent messages.

---

## Phase 1 — Tool Infrastructure (工具+Hook)

1. **Tool Registry** — global map, `init()`-based registration
   - `ToolDef{Name, Description, InputSchema, Handler}`
   - `InputSchema` is a Go struct → serialized to JSON Schema for Anthropic SDK
2. **Hook chain**
   - `PRE_TOOL_USE`: sync, serial, can abort (return error) or mutate params
   - `POST_TOOL_USE`: async goroutine, deep-copy context, pure observer
   - Chain order: `SecurityHook → ApprovalHook` (PRE), `AuditHook` (POST)
3. **Basic built-in tools**: `read_file`, `list_directory`, `fetch_url`, `respond_to_user`
4. **System prompt pipeline** — all 6 layers wired up
   - Static: layers 1–4 built once at loop start
   - Dynamic: layers 5–6 rebuilt each round (layer 6 queries DB with `NewDB: true` to bypass GORM identity map)
5. **Tool result truncation** — 16 KB cap, UTF-8 safe (no mid-rune cut)

---

## Phase 2 — MCP (MCP 接入)

1. **MCPClient interface** — 3 transports: stdio, SSE, HTTP
2. **Tool routing table** — built at loop start, `toolName → MCPClient`
3. **GitHub name-prefix deduplication** — `github-tools-sap__<tool>` / `github-wdf__<tool>`
4. **`mcp_tokens` table** — DB-cached OAuth tokens, 30-s pre-expiry refresh
   - Client Credentials: auto-refresh
   - OIDC: return re-auth URL to user
5. **Mount**: GitHub (×2) + Jira MCP servers

---

## Phase 3 — Manual Reports (手动报告)

1. `POST /api/reports/:type/run` — triggers an on-demand report (`daily` | `weekly`)
2. Handler creates/reuses a fixed conversation by well-known title, injects a fixed prompt, calls `agent.Service.Run`
3. Agent uses GitHub MCP tools to fetch commits + PRs, filters bots, formats Markdown — streamed live over SSE
4. No scheduler, no cron, no background jobs

---

## Phase 4 — Multi-Agent Platform (多 Agent 平台)

A conversation is a multi-agent chat: an implicit built-in orchestrator plus
zero or more user-added subagents. The user talks only to the orchestrator,
which splits the request and dispatches tasks to subagents present in the chat
(1 → n → 1). Split into three sub-phases (see `docs/ph4-sub-agent-scheduling.md`):

- **ph4a** — Agent & Skill hubs + schema redesign (`skills`, `agents`,
  `agent_skills`, `conversation_agents`, `messages.agent_id`). CRUD APIs,
  agent↔skill loadout, conversation↔agent membership, system-default seeding.
  *(implemented)*
- **ph4b** — `dispatch_to_agent` tool, subagent runner, errgroup fan-in,
  summary injection, thread lifecycle (invariants B/C/D). *(future)*
- **ph4c** — Vue pages for the skill/agent hubs and per-chat agent selection.
  *(future)*

---

## Phase 5 — File Ops + Approval (文件操作+审批)

1. `create_file`, `edit_file` — handler-layer sandbox check (`filepath.Clean` + `filepath.Rel` against `sandbox/{conv_id}/`)
2. `ApprovalHook` — DB-first (`pending→approved/rejected`), then signal channel; fail-closed on DB error
3. Batch approval — ≥2 file writes in one round → single approval; context flag for subsequent pass-through
4. `AuditHook` — log keys only, never values
5. `message_appended` SSE for approval bubbles and diffs

---

## Phase 6 — Visualization (命令驱动可视化)

1. `deploy_app` tool — Docker SDK, 8-step deploy
2. `/preview/:conv_id/:path` reverse proxy via `httputil.ReverseProxy`
3. Frontend iframe with `?t=timestamp` cache-bust

---

## Phase 7 — Context Compression + Long-term Memory

1. Token-limit detection → summarize older messages via Anthropic API
2. `MEMORY.md` index surfaced in system prompt layer 5
3. Oncall alert fast-analysis (prompt only, no new tools)

---

## Key Architecture Invariants (never break)

| # | Invariant |
|---|---|
| A | `PRE_TOOL_USE hook` → `write message history` → `execute tool` (never reorder) |
| B | Thread marked running **before** goroutine delay |
| C | Thread cancellation uses per-thread short transactions |
| D | Sub-agent message IDs are independent (never reuse user message ID) |
| E | `round_done` / `queue_drained` are never dropped (drain queue, then push) |
| F | Approval: write DB first, signal channel second |
| G | Sandbox path validated at handler layer (not hook layer) |
| H | Audit log: keys only, never values |
