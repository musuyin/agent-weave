# ph4c — Agent Hub UI (Vue)

Note. This document is only for archiving but not for implementing. The frontend will be implementted in other workspace.

**Goal**: Frontend for the skill hub, agent hub, and per-chat agent selection.

Prerequisite: ph4a APIs. Status: **future** (not yet implemented).

---

## Pages

### Skill hub
- List system + user skills.
- Create/edit skill (name, description, Markdown body — Markdown editor).
- Delete user skills; system skills are read-only (disable controls).
- Backed by `/api/skills`.

### Agent hub
- List system + user agents.
- Create/edit agent (name, description, prompt).
- Manage loadout: show loaded skills, add/remove from the skill list.
- Delete user agents; system agents read-only.
- Backed by `/api/agents` + `/api/agents/:id/skills`.

### Per-chat agent selection
- In a conversation view, a panel listing the subagents present in this chat.
- Add/remove agents from the current chat.
- Backed by `/api/conversations/:id/agents`.

## Notes
- Reuse existing API client patterns in `web/`.
- Messages rendered per producing agent (orchestrator vs subagent) once ph4b
  populates `messages.agent_id`.
