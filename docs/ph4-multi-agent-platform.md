# ph4 — Multi-Agent Platform (多 Agent 平台)

**Goal**: Turn a conversation into a multi-agent chat. A conversation has an
implicit built-in **orchestrator** plus zero or more **subagents** the user
adds. The user only talks to the orchestrator; the orchestrator splits the
request into tasks and dispatches them to the subagents present in the chat
(1 → n → 1 fan-out/fan-in), then summarises their results back to the user.

The original single-doc plan (thin "spawn a sub-agent goroutine") was too small
for this scope, so Phase 4 is split into three sub-phases. This file is the
overview; each sub-phase has its own doc.

---

## Concepts

- **Orchestrator** — implicit, built-in (`prompts.Orchestrator`). Never a row in
  any table. It is the only agent the user talks to.
- **Subagent** — a reusable definition (`agents` row): name + description +
  agent-prompt + a set of loaded skills. Callable only by the orchestrator.
- **Skill** — a reusable Markdown instruction block (`skills` row). Text only —
  no tool bindings, no code. A skill body is injected into an agent's system
  prompt when that agent runs.
- Both agents and skills are **global** DB rows: system-seeded defaults
  (`is_system = true`) plus user-created ones. Managed via "hub" CRUD pages/APIs.
- A conversation links to the subagents it contains via a **join table**
  (`conversation_agents`). Definitions are **shared** — editing a def affects
  every chat that uses it (no snapshot copies).
- Every message links to the agent that produced it. **Orchestrator messages
  have `agent_id = NULL`** (the orchestrator has no `agents` row, so a real FK
  to a sentinel is impossible; NULL cleanly means "orchestrator").

---

## Sub-phase split

| Doc | Scope | Status |
|---|---|---|
| `docs/ph4a-agent-skill-hubs.md` | Schema redesign; `skills`/`agents` hub CRUD; agent↔skill loadout; conversation↔agent membership; message→agent link; default seeding. | implemented |
| `docs/ph4b-dispatch.md` | `dispatch_to_agent` orchestrator tool; subagent runner; 1→n→1 errgroup fan-in; summary injection; thread lifecycle (invariants B/C/D). | future |
| `docs/ph4c-agent-hub-ui.md` | Vue pages for the skill hub and agent hub; per-chat agent selection. | future |

ph4a is data + hubs only. It carries no dispatch/execution logic — the
message→agent column and the `conversation_agents`/`agent_skills` joins are
populated by ph4a's APIs but only *consumed* by ph4b.

---

## Architecture invariants (revived in ph4b)

| # | Invariant | Where enforced (ph4b) |
|---|---|---|
| B | Thread marked `running` before goroutine launch | `dispatch_to_agent` handler |
| C | Cancellation: per-thread short tx, not shared | `CancelConversationThreads` |
| D | Sub-agent message IDs independently generated | subagent runner |
| Security | Subagents have no direct user access — only the orchestrator dispatches to them | dispatch tool |
