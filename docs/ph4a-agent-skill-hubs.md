# ph4a — Agent & Skill Hubs + Schema Redesign

**Goal**: Data model + CRUD hubs for the multi-agent platform. No dispatch or
execution logic (that is ph4b).

Prerequisite: ph0 (models, services, handlers, wire). See `docs/ph4-sub-agent-scheduling.md`
for the overall concept.

---

## 1. Schema

New tables + a nullable FK on `messages`. Migrations 000004–000008.

### `skills` (000004)
Reusable Markdown instruction blocks.

| column | type | notes |
|---|---|---|
| id | VARCHAR(36) PK | |
| name | VARCHAR(255) | unique |
| description | VARCHAR(1000) | default '' |
| body | MEDIUMTEXT | the Markdown |
| is_system | TINYINT(1) | default 0; system rows are read-only |
| created_at / updated_at | DATETIME(3) | |

### `agents` (000005)
Subagent definitions.

| column | type | notes |
|---|---|---|
| id | VARCHAR(36) PK | |
| name | VARCHAR(255) | unique |
| description | VARCHAR(1000) | default ''; the orchestrator reads this to decide routing |
| prompt | MEDIUMTEXT | the agent's system prompt |
| is_system | TINYINT(1) | default 0 |
| created_at / updated_at | DATETIME(3) | |

### `agent_skills` (000006)
Join: which skills an agent has loaded. `PRIMARY KEY (agent_id, skill_id)`,
both FKs `ON DELETE CASCADE`.

### `conversation_agents` (000007)
Join: which subagents are present in a chat. `PRIMARY KEY (conversation_id,
agent_id)`, both FKs `ON DELETE CASCADE`.

### `messages.agent_id` (000008)
`ADD COLUMN agent_id VARCHAR(36) NULL AFTER role` + FK to `agents(id)`
`ON DELETE SET NULL`. NULL = orchestrator (implicit, no row).

All tables follow the existing style: `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
`DATETIME(3)` with `CURRENT_TIMESTAMP(3)`, explicit indexes. Down files
`DROP TABLE IF EXISTS ...`; 000008 down drops the FK then the column.

---

## 2. Models — `internal/model/repository/`

New `skill.go`, `agent.go`. Explicit join structs (matches the codebase's
explicit style — no gorm `many2many` magic).

- `Skill{ID, Name, Description, Body, IsSystem, CreatedAt, UpdatedAt}`
- `Agent{ID, Name, Description, Prompt, IsSystem, CreatedAt, UpdatedAt}`
- `AgentSkill{AgentID, SkillID}` (composite PK)
- `ConversationAgent{ConversationID, AgentID, CreatedAt}` (composite PK)

`message.go`: add `AgentID *string` (pointer = nullable; NULL for orchestrator).

DTOs in `internal/model/dto/`: `skill.go` (create/update requests) and
`agent.go` (create/update + `LoadSkillRequest{SkillID}` + `AddAgentRequest{AgentID}`).

---

## 3. Services — `internal/service/`

New `skill.go`, `agent.go`. Pattern: `NewXxxService(db)`, methods take `ctx`,
use `s.db.WithContext(ctx)`, reuse `isDuplicateKeyError` on unique-name clash.

**SkillService**: `List`, `Get`, `Create(name, desc, body)`,
`Update(id, ...)`, `Delete(id)`. `is_system` rows are read-only → `Update`/`Delete`
return `ErrSystemReadOnly` (mapped to 400).

**AgentService**:
- CRUD (same `is_system` guard).
- Loadout: `ListSkills(agentID)`, `LoadSkill(agentID, skillID)`,
  `UnloadSkill(agentID, skillID)`.
- Membership: `ListConversationAgents(convID)`,
  `AddAgentToConversation(convID, agentID)`,
  `RemoveAgentFromConversation(convID, agentID)`.
- `Init(ctx)` — idempotent seeding of system defaults (find-or-create by unique
  name), mirroring `ReportService.Init`.

`ErrSystemReadOnly` and `ErrNotFound` live in `internal/service/errors.go`.

Providers `ProvideSkillService(ctx, db)` / `ProvideAgentService(ctx, db)` call
`Init` at startup (same shape as `service.ProvideReportService`).

> Naming note: `agent.ProvideAgentService` (package `agent`) already exists;
> the new one is `service.ProvideAgentService` — different package.

### Seeding
System skills: `code-review-guidelines`, `go-style`. System agent:
`code-reviewer` (loaded with `code-review-guidelines`). All `is_system = true`.

---

## 4. Handlers + routes — `internal/handler/`

New `skill.go`, `agent.go`. Thin: bind → service → JSON. Errors: 400 bind/validation
or `is_system`, 404 not-found, 500 db.

```
GET    /api/skills                            200  list
POST   /api/skills                            201  create
GET    /api/skills/:id                        200  get (404)
PUT    /api/skills/:id                         200  update (400 if is_system)
DELETE /api/skills/:id                        204  delete (400 if is_system)

GET    /api/agents                            200  list
POST   /api/agents                            201  create
GET    /api/agents/:id                        200  get
PUT    /api/agents/:id                          200  update
DELETE /api/agents/:id                        204  delete
GET    /api/agents/:id/skills                 200  list loaded skills
POST   /api/agents/:id/skills                 200  {skill_id} → load
DELETE /api/agents/:id/skills/:skillId        204  unload

GET    /api/conversations/:id/agents          200  list subagents in chat
POST   /api/conversations/:id/agents          200  {agent_id} → add
DELETE /api/conversations/:id/agents/:agentId 204  remove
```

Registered in `ProvideRouter` (new params `skillH`, `agentH`).

---

## 5. Wire — `cmd/server/wire.go`

Add `service.ProvideSkillService`, `service.ProvideAgentService`,
`handler.NewSkillHandler`, `handler.NewAgentHandler`; update `ProvideRouter`.
Regenerate with `wire ./cmd/server/`.

---

## 6. Tests — `test/handler/`

Add the four new models to the AutoMigrate list in `testhelper_test.go`
(and `test/agent/invariant_test.go`).

- `skill_test.go` — create→list→get→update→delete; delete/update system skill → 400.
- `agent_test.go` — CRUD; load/unload skill reflected in `GET /agents/:id/skills`;
  add/remove agent to conversation reflected in `GET /conversations/:id/agents`.

Real stack (real services + SQLite), no mocks.

---

## Verification

```bash
go build ./...
wire ./cmd/server/
go test ./test/... -race

curl -s localhost:8080/api/skills                        # seeded system skills
curl -s -X POST localhost:8080/api/agents -d '{"name":"researcher","description":"web research","prompt":"You research topics."}'
curl -s -X POST localhost:8080/api/agents/<id>/skills -d '{"skill_id":"<sid>"}'
curl -s localhost:8080/api/agents/<id>/skills            # shows loaded skill
CID=$(curl -s -X POST localhost:8080/api/conversations -d '{"title":"t"}' | jq -r .ID)
curl -s -X POST localhost:8080/api/conversations/$CID/agents -d '{"agent_id":"<id>"}'
curl -s localhost:8080/api/conversations/$CID/agents      # agent in chat
curl -s -o /dev/null -w '%{http_code}' -X DELETE localhost:8080/api/skills/<system-skill-id>  # 400
```

Message→agent link + orchestrator=NULL is schema-only in ph4a (consumed in ph4b).
