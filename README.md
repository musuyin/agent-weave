# Agent Weave

A self-hosted multi-agent AI chat platform. Users interact with an orchestrator powered by Claude that can fan work out to specialized subagents, call external tools via MCP servers, and stream all output back to the browser in real time.

Built as a learning/portfolio project demonstrating production-grade agentic patterns in Go.

---

## Features

- **Streaming chat** — Server-Sent Events deliver each token as it arrives; the agent loop runs detached from the HTTP connection so it completes even if the client drops
- **Multi-agent dispatch** — The orchestrator has a built-in `dispatch_to_agent` tool that fans tasks to named subagents concurrently and fans the results back in
- **Hook system** — Every tool call passes through a `PRE_TOOL_USE` chain (sync, serial, can abort) then a `POST_TOOL_USE` chain (async, observer-only)
- **MCP tool routing** — Connect any MCP server (e.g. GitHub); tools are merged into the LLM's tool list at startup and dispatched automatically
- **Context compaction** — At ≥40 messages the middle slice is summarised by a dedicated LLM call and compacted in the database, keeping context windows manageable
- **On-demand reports** — `POST /api/reports/daily/run` and `POST /api/reports/weekly/run` trigger GitHub activity reports streamed back over SSE
- **Skill & Agent hubs** — CRUD endpoints for managing skills (system-prompt fragments) and named agents, with DB-backed seeding from embedded markdown/YAML files

---

## Architecture

```
web/  (Vue 3)        →  REST + SSE  →  server/  (Go)  →  Claude API
                                            │
                                      MCP servers (GitHub, ...)
                                            │
                                          MySQL
```

**Backend** is a layered Go service: `handler` (HTTP only) → `service` (business logic) → GORM. Dependency injection is compile-time via `google/wire`.

**Frontend** is a Vue 3 SPA. A `useSSE.ts` composable handles the event stream (`block_start/delta/stop`, `message_appended`, `round_done`, `queue_drained`).

---

## Tech Stack

| Layer | Technology |
|---|---|
| Go version | 1.25 |
| HTTP | Gin |
| ORM | GORM (MySQL prod / SQLite in tests) |
| DB migrations | golang-migrate (9 migration files, applied at startup) |
| DI | google/wire (compile-time, `wire_gen.go` committed) |
| LLM | Anthropic SDK for Go (Claude) |
| MCP client | modelcontextprotocol/go-sdk (Streamable HTTP) |
| Frontend | Vue 3, TypeScript, Vite, Naive UI, Pinia, TanStack Query, pnpm |

---

## Getting Started

### Prerequisites

- Go 1.25+
- MySQL 8.0+
- Node.js + pnpm
- An Anthropic API key

### Backend

```bash
# Create the database (schema is applied automatically at startup)
mysql -u root -p -e "CREATE DATABASE IF NOT EXISTS agentweave CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"

cd server
cp config.yaml.example config.yaml
# Edit config.yaml — set anthropic.api_key and database.database_url

go run ./cmd/server/
```

Minimal `config.yaml`:

```yaml
llm_model:
  anthropic:
    api_key: "sk-ant-..."
    model: "claude-sonnet-4-6"
database:
  database_url: "mysql://root:password@tcp(localhost:3306)/agentweave"
server:
  port: "8080"
```

Run tests (SQLite in-memory, no MySQL needed):

```bash
go test ./test/... -race -v
```

### Frontend

```bash
cd web
pnpm install
pnpm dev      # dev server — proxies /api to localhost:8080
pnpm build    # production build
pnpm test     # Vitest
```

---

## Key API Endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/health` | Health check |
| GET / POST | `/api/conversations` | List / create conversations |
| GET / POST | `/api/conversations/:id/messages` | List messages / send a message (triggers agent loop) |
| GET | `/api/conversations/:id/stream` | SSE stream for agent output |
| POST | `/api/reports/:type/run` | Trigger on-demand report (`daily` or `weekly`) |
| GET / POST | `/api/skills` | Skill CRUD |
| GET / POST | `/api/agents` | Agent CRUD |
| GET / POST | `/api/conversations/:id/agents` | Manage agents assigned to a conversation |
| DELETE | `/api/conversations/:id/threads` | Cancel all running subagent threads |

---

## Project Structure

```
server/
  cmd/server/          Entry point, Wire provider graph
  internal/
    agent/             SSE hub, agent loop, compaction, subagent runner, dispatch registry
    handler/           Thin HTTP adapters
    hook/              Pre/post tool-use hook chains
    mcp/               MCP client — flat toolName→client dispatch table
    model/             GORM models (Conversation, Message, Thread)
    seeding/           Embedded orchestrator prompt, skill markdown, agent YAML
    service/           Business logic
    tool/              Builtin tool registry (read_file, list_directory, fetch_url, dispatch_to_agent)
  test/                Integration tests against SQLite in-memory

web/
  src/
    views/             HomeView, ChatView, AgentsView, SkillsView
    components/        MessageBubble, Sidebar
    composables/       useSSE.ts
    stores/            conversations.ts (Pinia)
    api.ts             All REST calls
    types.ts           TypeScript interfaces

docs/                  Spec and per-phase implementation plans
```

---

## Implementation Status

| Phase | Description | Status |
|---|---|---|
| 0 | Foundation — streaming chat, persistent messages | Done |
| 1 | Tool registry + hook chain + builtin tools | Done |
| 2 | MCP client integration | Done |
| 3 | On-demand daily/weekly reports | Done |
| 4a | Skill + Agent hubs (CRUD + seeding) | Done |
| 4b | Orchestrator dispatch + subagent fan-in | Done |
| 7 | Context compaction | Done |
| 4c | Agent Hub UI (frontend pages) | Deferred |
| 5 | File operations + approval workflow | Deferred |
| 6 | Command-driven Docker visualisation | Deferred |

---

## License

MIT
