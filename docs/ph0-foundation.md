# ph0 — Foundation (基础骨架)

**Goal**: Runnable server — streaming text chat, persistent messages.
**Status**: ✓ Complete.

---

## 0.1 Go module + dependencies

`go.mod` direct dependencies:

```
github.com/gin-gonic/gin
gorm.io/gorm + gorm.io/driver/mysql + gorm.io/driver/sqlite
github.com/golang-migrate/migrate/v4
github.com/google/wire
github.com/anthropics/anthropic-sdk-go
github.com/google/uuid
gopkg.in/yaml.v3
github.com/stretchr/testify  (test only)
```

OIDC (`coreos/go-oidc/v3`, `golang.org/x/oauth2`) is **not** in `go.mod` — deferred entirely. See `docs/deferred.md`.

## 0.2 Directory layout

```
server/
  cmd/server/
    main.go          graceful HTTP server, SIGINT/SIGTERM shutdown
    wire.go          Wire injector stub (build tag: wireinject)
    wire_gen.go      generated — commit, never edit
  internal/
    config/          config.go — Config struct, Load, ProvideConfig
    db/              db.go — ProvideDB; migrate.go — RunMigrations
    model/
      repository/    GORM models: Conversation, Message, Thread + scanners
      dto/           HTTP request/response structs (no GORM tags)
    agent/           loop.go — Service.Run; sse.go — Hub, HubRegistry
    service/         ConversationService, MessageService
    handler/         ConversationHandler, MessageHandler, StreamHandler, router
  db/migrations/     000001..000003 × {up,down}.sql
  test/              external _test packages mirroring internal/
    agent/
    config/
    handler/
    model/
```

## 0.3 Config (`internal/config/config.go`)

`config.yaml` is the config source. It is git-ignored; `config.yaml.example` is the committed template.

```yaml
llm_model:
  anthropic:
    api_key: ""
    base_url: ""        # optional; omit to use the default Anthropic endpoint
    model: ""           # default: claude-sonnet-4-6

database:
  database_url: "mysql://user:pass@tcp(host:3306)/db"

server:
  port: "8080"

oidc:                   # deferred — leave empty
  oidc_provider_url: ""
  oidc_client_id: ""
  oidc_client_secret: ""
  session_secret: ""
  base_url: ""
```

Key exports:
```go
func Load(path string) (*Config, error)
func ProvideConfig() (*Config, error)   // Wire provider — loads "config.yaml" from cwd
```

Defaults applied in `Load`: `model → "claude-sonnet-4-6"`, `port → "8080"`.

**Known issue:** `main.go` reads port from `$PORT` env var (falling back to `"8080"`) and ignores `cfg.Server.Port`. Port from config is currently unused at startup.

## 0.4 DB (`internal/db/`)

**db.go — `ProvideDB`:**
- Converts `mysql://...` URL to GORM DSN via `mysqlDSN()` (strips `mysql://` prefix, appends `?parseTime=true&charset=utf8mb4&loc=UTC` when no query params are present).
- Opens GORM with `gormlogger.Warn` level.
- Calls `RunMigrations` before returning.
- Returns a cleanup func that closes `*sql.DB`.

**migrate.go — `RunMigrations`:**
- Locates `db/migrations/` by trying `runtime.Caller(0)`-relative path first, then cwd-relative fallback.
- Uses `golang-migrate` with `iofs` source driver.
- Swallows `migrate.ErrNoChange`.

## 0.5 DB Migrations (`db/migrations/`)

golang-migrate format. Run automatically at startup; also usable via `migrate` CLI.

**000001_create_conversations:**
```sql
id VARCHAR(36) PK, title VARCHAR(500), created_at DATETIME(3), updated_at DATETIME(3)
INDEX idx_conversations_created (created_at)
```

**000002_create_messages:**
```sql
id VARCHAR(36) PK, conversation_id VARCHAR(36) FK → conversations(id) ON DELETE CASCADE,
role VARCHAR(20), content JSON NOT NULL, created_at DATETIME(3)
INDEX idx_messages_cursor (conversation_id, created_at, id)   ← keyset pagination
```

**000003_create_threads:**
```sql
id VARCHAR(36) PK, conversation_id VARCHAR(36) FK → conversations(id) ON DELETE CASCADE,
agent_id VARCHAR(255), status VARCHAR(50) DEFAULT 'pending',
blocked_by JSON NOT NULL DEFAULT (JSON_ARRAY()),
created_at DATETIME(3), updated_at DATETIME(3)
INDEX idx_threads_conversation (conversation_id)
```

All tables: `ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`.

## 0.6 Models (`internal/model/`)

Split into two sub-packages:

### `model/repository` — GORM models

**conversation.go**: `Conversation{ID, Title, CreatedAt, UpdatedAt}`
- No `UserID` — auth is deferred (see `docs/deferred.md`).
- Time tags: `autoCreateTime:milli` / `autoUpdateTime:milli` (not `type:datetime(3)` — the precision suffix breaks SQLite scanning in tests).

**message.go**:
- `ContentBlock{Type string, Text string}` — `Type: "text"` only in Phase 0; Phase 1 adds `"tool_use"` / `"tool_result"`.
- `ContentBlocks []ContentBlock` — implements `driver.Valuer` / `sql.Scanner` via `json.Marshal/Unmarshal`. Handles `[]byte` and `string` from the DB driver.
- `Message{ID, ConversationID, Role, Content ContentBlocks, CreatedAt}`

**thread.go**:
- `ThreadStatus` constants: `pending | running | done | cancelled | error`
- `StringSlice []string` — implements Valuer/Scanner for `blocked_by` JSON column.
- `Thread{ID, ConversationID, AgentID, Status ThreadStatus, BlockedBy StringSlice, CreatedAt, UpdatedAt}`

All IDs are `VARCHAR(36)` UUIDs generated in Go (`uuid.NewString()`). No `AutoIncrement`. Schema is managed entirely by golang-migrate, not `AutoMigrate` (tests use `AutoMigrate` against SQLite for speed).

### `model/dto` — HTTP layer structs

- `CreateConversationRequest{Title string}`
- `SendMessageRequest{Content string}` (`binding:"required"`)

## 0.7 Auth — DEFERRED

No auth middleware exists. All API routes are unprotected. See `docs/deferred.md`.

## 0.8 SSE (`internal/agent/sse.go`)

**`Hub`** — one per active conversation, buffered channel cap 256:
```go
func NewHub(log *slog.Logger) *Hub
func (h *Hub) Push(event SSEEvent)   // drain-then-push when full; no-op after Close
func (h *Hub) Chan() <-chan SSEEvent
func (h *Hub) Close()                // idempotent
func (h *Hub) Cap() int              // for tests
func (h *Hub) Len() int              // for tests
```

`Push` invariant (E): if the buffer is full, all pending events are drained first, then the new event is pushed. This guarantees `round_done` / `queue_drained` are never lost.

**`HubRegistry`** — `map[conversationID]*Hub` with `sync.RWMutex`:
```go
func NewHubRegistry(log *slog.Logger) *HubRegistry
func (r *HubRegistry) GetOrCreate(conversationID string) *Hub
func (r *HubRegistry) Get(conversationID string) *Hub
func (r *HubRegistry) Delete(conversationID string)   // closes + removes
```

**Event types:**
```
agent_start, block_start, block_delta, block_stop,
message_appended, approval_requested, thread_status,
round_done, queue_drained
```

**`SSEEvent`:**
```go
type SSEEvent struct {
    Type EventType `json:"type"`
    Data any       `json:"data,omitempty"`
}
```

Payload structs: `BlockStartData{BlockID, BlockType, Index}`, `BlockDeltaData{BlockID, Text, Index}`, `BlockStopData{BlockID, Index}`.

## 0.9 Agent Loop (`internal/agent/loop.go`)

Phase 0 loop — text only, no tools:

```
1. ctx.Err() check — return early (Run wrapper pushes round_done+queue_drained)
2. push agent_start
3. loadHistory() — SELECT messages WHERE conversation_id=? ORDER BY created_at ASC, id ASC
4. aiClient.Messages.NewStreaming() — MaxTokens: 8096, system prompt hardcoded
5. for each stream event:
     ContentBlockStartEvent  → generate block_id (UUID), push block_start
     ContentBlockDeltaEvent  → push block_delta (text deltas only)
     ContentBlockStopEvent   → push block_stop
   accMsg.Accumulate(event) on every event
6. persistMessage() — INSERT assistant message (text blocks only from accMsg.Content)
7. push round_done
8. push queue_drained
```

On any error: `Run()` logs the error, then pushes `round_done` + `queue_drained` so the SSE stream always terminates.

```go
type Service struct {
    db       *gorm.DB          // for Phase 1+ NewDB:true sub-queries
    aiClient *anthropic.Client
    registry *HubRegistry
    cfg      *config.Config
    log      *slog.Logger
}

func ProvideAgentService(db *gorm.DB, cfg *config.Config, registry *HubRegistry, log *slog.Logger) *Service
func (s *Service) Run(ctx context.Context, conversationID string, hub *Hub)
```

The agent runs on `context.Background()` (detached from the HTTP request) — it completes even if the client disconnects.

## 0.10 Services (`internal/service/`)

**ConversationService:**
```go
func (s *ConversationService) List(ctx) ([]repository.Conversation, error)
    // ORDER BY created_at DESC LIMIT 50 — no pagination yet
func (s *ConversationService) Create(ctx, title string) (repository.Conversation, error)
    // default title: "New conversation"
```

**MessageService:**
```go
func (s *MessageService) ConversationExists(ctx, convID string) (bool, error)
func (s *MessageService) List(ctx, convID string, p ListParams) ([]repository.Message, error)
    // keyset pagination: after_created_at + after_id; validates after_id belongs to convID
func (s *MessageService) SaveUserMessage(ctx, convID, text string) (repository.Message, error)
```

Errors: `ErrConversationNotFound`, `ErrInvalidCursor`.

## 0.11 API Handlers (`internal/handler/`)

**router.go — `ProvideRouter`:**
```
GET  /health
GET  /api/conversations
POST /api/conversations
GET  /api/conversations/:id/messages      keyset cursor pagination
POST /api/conversations/:id/messages      202 Accepted, launches agent goroutine
GET  /api/conversations/:id/stream        SSE
```

Middleware: `gin.Recovery()`, `ginLogger` (logs method/path/status after handler returns).
No auth middleware — deferred.

**conversation.go — `ConversationHandler`:**
- `List`: returns `[]repository.Conversation` as JSON.
- `Create`: binds `CreateConversationRequest`; tolerates empty body (`EOF` error ignored).

**message.go — `MessageHandler`:**
- `List`: cursor params `after_created_at` (RFC3339Nano) + `after_id`; both required together or neither.
- `Send`:
  1. `requireConversation` — 404 if not found.
  2. `SaveUserMessage` — persists user message synchronously.
  3. `hub := registry.GetOrCreate(convID)`.
  4. `go agentSvc.Run(context.Background(), convID, hub)`.
  5. Returns 202 with the saved user message.

**stream.go — `StreamHandler`:**
- Checks conversation exists (404 if not) before creating a Hub.
- Sets `Cache-Control: no-cache`, `X-Accel-Buffering: no`, `Content-Type: text/event-stream`.
- `c.Stream(...)` ranges over `hub.Chan()`, serialises each event as `c.SSEvent(eventType, jsonData)`.
- Closes on `EventQueueDrained` (calls `registry.Delete`) or client disconnect.
- Calls `registry.Delete` again after `c.Stream` returns to handle disconnect-before-drained.

**Correct client usage sequence:**
1. `POST /api/conversations` → get `convID`
2. `GET /api/conversations/:convID/stream` (keep open)
3. `POST /api/conversations/:convID/messages` with `{"content": "..."}`
4. Read SSE events from step 2 until `queue_drained`

## 0.12 Wire DI (`cmd/server/`)

**wire.go** (build tag `//go:build wireinject`):
```go
func InitializeApp(ctx context.Context, log *slog.Logger) (*gin.Engine, func(), error) {
    wire.Build(
        config.ProvideConfig,
        db.ProvideDB,
        agent.NewHubRegistry,
        agent.ProvideAgentService,
        service.NewConversationService,
        service.NewMessageService,
        handler.NewConversationHandler,
        handler.NewMessageHandler,
        handler.NewStreamHandler,
        handler.ProvideRouter,
    )
    return nil, nil, nil
}
```

`ctx` is declared but not threaded to any provider (dead parameter — kept for future use).
`wire_gen.go` is the generated output — committed, never edited by hand.

**main.go**: JSON slog → `InitializeApp` → HTTP server → SIGINT/SIGTERM → 30 s graceful shutdown.

---

## File list

| File | Key exports |
|---|---|
| `cmd/server/main.go` | `main()` |
| `cmd/server/wire.go` | `InitializeApp()` stub |
| `cmd/server/wire_gen.go` | generated |
| `internal/config/config.go` | `Config`, `Load`, `ProvideConfig` |
| `internal/db/db.go` | `ProvideDB`, `mysqlDSN` |
| `internal/db/migrate.go` | `RunMigrations` |
| `internal/model/repository/conversation.go` | `Conversation` |
| `internal/model/repository/message.go` | `Message`, `ContentBlock`, `ContentBlocks` |
| `internal/model/repository/thread.go` | `Thread`, `ThreadStatus`, `StringSlice` |
| `internal/model/dto/conversation.go` | `CreateConversationRequest` |
| `internal/model/dto/message.go` | `SendMessageRequest` |
| `internal/agent/sse.go` | `Hub`, `HubRegistry`, `SSEEvent`, event constants |
| `internal/agent/loop.go` | `Service`, `ProvideAgentService`, `Run` |
| `internal/service/conversation.go` | `ConversationService` |
| `internal/service/message.go` | `MessageService`, `ListParams`, errors |
| `internal/handler/router.go` | `ProvideRouter`, `Health` |
| `internal/handler/conversation.go` | `ConversationHandler` |
| `internal/handler/message.go` | `MessageHandler` |
| `internal/handler/stream.go` | `StreamHandler` |

---

## Architecture invariants pre-wired

| # | Rule | Where |
|---|---|---|
| A | `persistMessage` position is after stream completes — pre-wired for Phase 1 hook ordering | `loop.go:134` |
| E | `round_done`/`queue_drained` never dropped — `Hub.Push` drain-then-push | `sse.go:72` |
| cursor | `after_id` validated against conversation before use | `service/message.go:57` |

---

## Verification

```bash
cd server
go build ./...
wire ./cmd/server/
go test ./...
go test ./test/... -race
```
