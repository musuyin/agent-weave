# ph0 — Foundation (基础骨架)

**Goal**: Runnable server — login, streaming text chat, persistent messages.
**Prerequisite**: none (start here).

---

## 0.1 Go module + dependencies

Add to `go.mod` (then `go mod tidy`):

```
github.com/gin-gonic/gin
gorm.io/gorm + gorm.io/driver/mysql
github.com/golang-migrate/migrate/v4
github.com/google/wire
github.com/anthropics/anthropic-sdk-go
github.com/coreos/go-oidc/v3
golang.org/x/oauth2
github.com/kelseyhightower/envconfig
github.com/google/uuid
```

## 0.2 Directory layout

```
server/
  cmd/server/          main.go, wire.go, wire_gen.go
  internal/
    config/            env struct via envconfig
    db/                gorm setup, migration runner
    model/             Conversation, Message, Thread (GORM models)
    auth/              OIDC PKCE flow, session middleware
    handler/               Gin handlers (conversations, messages, SSE, auth)
    agent/             loop.go, sse.go
  db/migrations/       SQL files (golang-migrate)
```

## 0.3 Config (`internal/config/config.go`)

Use `config.yaml` to store all configs. Add `config.yaml` to `.gitignore`. Provide `config.yaml.example` with empty values as a template.

| Key | Purpose |
|---|---|
| `anthropic_api_key` | Claude API key |
| `anthropic_model` | Model ID (default: `claude-sonnet-4-6`) |
| `database_url` | MySQL DSN (`mysql://user:pass@tcp(host)/db`) |
| `port` | HTTP port (default `8080`) |

OIDC keys (`oidc_provider_url`, `oidc_client_id`, `oidc_client_secret`, `session_secret`, `base_url`) are defined in the struct but left empty — see `deferred.md`.

```go
func Load() (*Config, error)
func ProvideConfig() (*Config, error)  // Wire provider
```

## 0.4 DB Migrations (`server/db/migrations/`)

golang-migrate format: `{N}_{description}.up.sql` / `.down.sql`

- `000001_create_conversations` — id VARCHAR(36) PK, user_id, title, timestamps; index `(user_id, created_at)`
- `000002_create_messages` — id, conversation_id FK, role VARCHAR(20), **content JSON NOT NULL**; composite index `(conversation_id, created_at, id)` for keyset pagination
- `000003_create_threads` — id, conversation_id FK, agent_id VARCHAR(255), status VARCHAR(50), **blocked_by JSON NOT NULL DEFAULT '[]'**, timestamps

Migrations run automatically inside `ProvideDB` at server startup (no separate CLI step required, though `migrate` CLI remains available for manual ops).

## 0.5 GORM Models (`internal/model/`)

**conversation.go**: `Conversation{ID, UserID, Title, CreatedAt, UpdatedAt}`

**message.go**:
- `ContentBlock{Type, Text}` — Phase 1 will add tool_use fields
- `ContentBlocks []ContentBlock` — implements `driver.Valuer` / `sql.Scanner` via `json.Marshal/Unmarshal`
- `Message{ID, ConversationID, Role, Content ContentBlocks, CreatedAt}`

**thread.go**:
- `ThreadStatus` constants: `pending | running | done | cancelled | error`
- `StringSlice []string` — implements Valuer/Scanner for `blocked_by`
- `Thread{ID, ConversationID, AgentID, Status ThreadStatus, BlockedBy StringSlice, CreatedAt, UpdatedAt}`

All IDs are `VARCHAR(36)` UUIDs generated in Go (`github.com/google/uuid`). No `AutoIncrement`. No `AutoMigrate` calls — schema is managed entirely by golang-migrate.

## 0.6 Auth — DEFERRED

OIDC auth is blocked on IDP registration. See `deferred.md`.

**Current bypass**: all API routes have no authentication middleware. `GetCurrentUser` returns a hardcoded stub user `{Sub: "dev", Email: "dev@local", Name: "Dev"}` so downstream code that reads user identity still compiles and works.

Add `AuthMiddleware` to the authenticated route group in `internal/handler/router.go` once OIDC credentials are available.

## 0.7 SSE Writer (`internal/agent/sse.go`)

**Hub** — one per active conversation SSE connection, buffered channel cap 256:
```go
func (h *Hub) Push(event SSEEvent)
    // if channel full → drain ALL, then push — guarantees round_done/queue_drained delivery

func (h *Hub) Chan() <-chan SSEEvent
func (h *Hub) Close()
```

**HubRegistry**: `map[conversationID]*Hub` with RWMutex; `GetOrCreate`, `Get`, `Delete`.

Event type constants: `agent_start`, `block_start`, `block_delta`, `block_stop`,
`message_appended`, `approval_requested`, `thread_status`, `round_done`, `queue_drained`

## 0.8 Agent Loop (`internal/agent/loop.go`)

Phase 0 loop — text only, no tools:

```
1. Check ctx cancellation (abort signal)
2. Build static system prompt (layer 1 hardcoded + layer 4 placeholder)
3. Load message history from DB ordered (created_at ASC, id ASC)
4. Call Anthropic API (streaming) with cfg.AnthropicModel
5. For each streaming event: push block_start/delta/stop SSE
6. Accumulate full message via accMsg.Accumulate(event)
7. Persist assistant message to DB  ← write-history step (constraint A)
8. Push round_done then queue_drained
```

```go
type Service struct {
    db       *gorm.DB     // carry for Phase 1+ db.Session(&gorm.Session{NewDB:true})
    aiClient *anthropic.Client
    registry *HubRegistry
    log      *slog.Logger
}

func ProvideAgentService(db *gorm.DB, cfg *config.Config, registry *HubRegistry, log *slog.Logger) *Service
func (s *Service) Run(ctx context.Context, conversationID string, hub *Hub) error
```

## 0.9 API Handlers (`internal/api/`)

**router.go** — `ProvideRouter(...)`:
```
GET  /health                              (no auth)
GET  /auth/oidc/login
GET  /auth/oidc/callback
POST /auth/logout

[authenticated group]
GET  /api/conversations
POST /api/conversations
GET  /api/conversations/:id/messages      (keyset cursor pagination)
POST /api/conversations/:id/messages      (202 Accepted, triggers goroutine)
GET  /api/conversations/:id/stream        (SSE)
```

**message.go** — `MessageHandler.List` keyset pagination:
- cursor params: `after_created_at` + `after_id`
- validate `after_id` belongs to this conversation before use
- query: `WHERE conversation_id=? AND (created_at > ? OR (created_at=? AND id > ?)) ORDER BY created_at,id LIMIT n+1`

**message.go** — `MessageHandler.Send`:
1. Save user message to DB (synchronous)
2. `hub := registry.GetOrCreate(convID)`
3. `go agentSvc.Run(ctx, convID, hub)`
4. Return 202 with saved user message

**stream.go** — `StreamHandler.Stream`:
- Headers: `Cache-Control: no-cache`, `X-Accel-Buffering: no`
- Range `hub.Chan()`, `c.SSEvent(eventType, jsonData)` + `c.Writer.Flush()`
- Return when `EventQueueDrained` received or client disconnects

## 0.10 Wire DI (`cmd/server/`)

```go
// wire.go (build tag: //go:build wireinject)
func InitializeApp(ctx context.Context, log *slog.Logger) (*gin.Engine, func(), error) {
    wire.Build(
        config.ProvideConfig,
        db.ProvideDB,
        auth.ProvideAuthService,
        agent.ProvideAgentService,
        agent.NewHubRegistry,
        api.NewConversationHandler,
        api.NewMessageHandler,
        api.NewStreamHandler,
        api.NewAuthHandler,
        api.ProvideRouter,
    )
    return nil, nil, nil
}
```

Cleanup function closes the underlying `*sql.DB`. `wire_gen.go` is generated by `wire ./cmd/server/` and committed.

**main.go**: load app → HTTP server → graceful shutdown on SIGINT/SIGTERM (30 s timeout).

---

## File List (22 files)

**SQL (6):** `000001..000003` × `{up,down}.sql`

**Go (16):**

| File | Key exports |
|---|---|
| `cmd/server/main.go` | `main()` |
| `cmd/server/wire.go` | `InitializeApp()` stub |
| `cmd/server/wire_gen.go` | generated |
| `internal/config/config.go` | `Config`, `Load`, `ProvideConfig` |
| `internal/db/db.go` | `ProvideDB` |
| `internal/db/migrate.go` | `RunMigrations`, `//go:embed` |
| `internal/model/conversation.go` | `Conversation` |
| `internal/model/message.go` | `Message`, `ContentBlock`, `ContentBlocks` |
| `internal/model/thread.go` | `Thread`, `ThreadStatus`, `StringSlice` |
| `internal/auth/service.go` | `Service`, `PKCEState`, `UserClaims`, `ProvideAuthService` |
| `internal/auth/middleware.go` | `AuthMiddleware`, `GetCurrentUser` |
| `internal/agent/sse.go` | `Hub`, `HubRegistry`, `SSEEvent`, event constants |
| `internal/agent/loop.go` | `Service`, `ProvideAgentService`, `Run` |
| `internal/api/router.go` | `ProvideRouter` |
| `internal/api/auth.go` | `AuthHandler` |
| `internal/api/conversation.go` | `ConversationHandler` |
| `internal/api/message.go` | `MessageHandler` |
| `internal/api/stream.go` | `StreamHandler` |

---

## Architecture Invariants Pre-wired

| # | Invariant | Where |
|---|---|---|
| A | PRE_TOOL_USE → write history → execute (no tools yet; `persistAssistantMessage` position is already correct) | `loop.go` |
| E | `round_done`/`queue_drained` never dropped | `Hub.Push` drain-then-push |
| C_cursor | Cursor `after_id` validated against conversation | `MessageHandler.List` |
| D_bypass | `s.db` threaded through agent.Service for future `NewDB:true` use | `loop.go` |
| PKCE | In-memory map + `time.AfterFunc` 10-min TTL | `auth.Service` |
| Cookie | httponly + secure + SameSite=Lax, hand-rolled HMAC | `SetSessionCookie` |

---

## Verification

```bash
cd server
go mod tidy
wire ./cmd/server/
go build ./...
go test ./...

# integration (requires MySQL + env vars):
export DATABASE_URL="mysql://root:pass@tcp(localhost:3306)/agentweave"
export ANTHROPIC_API_KEY="sk-..."
export ANTHROPIC_MODEL="claude-sonnet-4-6"
export OIDC_PROVIDER_URL="https://..."
export OIDC_CLIENT_ID="..."
export OIDC_CLIENT_SECRET="..."
export SESSION_SECRET="at-least-32-chars-random-string"
export BASE_URL="http://localhost:8080"

./server &
curl http://localhost:8080/health           # 200 OK
curl http://localhost:8080/auth/oidc/login  # 302 → IDP
```
