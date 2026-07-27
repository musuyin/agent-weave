# ph2 — MCP Integration (MCP 接入)

**Goal**: Connect the agent to external MCP servers (two GitHub instances + Jira).
**Prerequisite**: ph1 complete (tool registry + hook chain).

---

## 2.1 MCPClient Interface (`internal/mcp/client.go`)

```go
type Tool struct {
    Name        string
    Description string
    InputSchema  json.RawMessage  // JSON Schema object
}

type Client interface {
    ListTools(ctx context.Context) ([]Tool, error)
    CallTool(ctx context.Context, name string, params json.RawMessage) (string, error)
    Close() error
}
```

Three transport implementations (using `mark3labs/mcp-go`):

| Transport | Constructor | Use case |
|---|---|---|
| stdio | `NewStdioClient(cmd string, args []string)` | Local MCP server process |
| SSE | `NewSSEClient(url string, token string)` | Remote MCP over HTTP/SSE |
| HTTP | `NewHTTPClient(url string, token string)` | Remote MCP over HTTP streamable |

## 2.2 Tool Routing Table (`internal/mcp/router.go`)

Built once at loop start — maps `prefixedToolName → Client`:

```go
type Router struct {
    routes map[string]Client  // prefixed tool name → MCP client
    log    *slog.Logger
}

// Build enumerates all registered MCP servers, lists their tools,
// applies name prefixes where configured, and populates routes.
// Built-in tools (from ph1 registry) are NOT in this map.
func Build(ctx context.Context, servers []ServerConfig, log *slog.Logger) (*Router, error)

// Route returns the MCPClient for a tool name, or (nil, false) if not an MCP tool.
func (r *Router) Route(toolName string) (Client, bool)

// AllTools returns all MCP tools as []tool.ToolDef for injection into the Anthropic SDK call.
func (r *Router) AllTools() []tool.ToolDef
```

Loop tool dispatch updated:
```go
if client, ok := mcpRouter.Route(toolName); ok {
    result, err = client.CallTool(ctx, toolName, params)
} else {
    result, err = tool.Get(toolName).Handler(ctx, params)
}
```

## 2.3 Name-Prefix Deduplication

Both GitHub MCP servers expose identically named tools (e.g. `search_commits`, `list_pull_requests`).
Prefix assignment in `ServerConfig`:

```go
type ServerConfig struct {
    Name      string   // human label
    Prefix    string   // prepended to all tool names, e.g. "github-tools-sap"
    Transport string   // "stdio" | "sse" | "http"
    // ...transport-specific fields
}
```

Resulting tool names seen by the LLM:
- `github-tools-sap__search_commits`
- `github-wdf__search_commits`
- `jira__search_issues`

The two GitHub instances:
- `github.tools.sap` — prefix `github-tools-sap`, orgs: `hci`, `common-service-infrastructure`
- `github.wdf.sap.corp` — prefix `github-wdf`, orgs: `DBaaS`, `hanadatalake`, `delphi`

## 2.4 MCP OAuth Token Management (`internal/mcp/token.go`)

DB table: `mcp_tokens` — unique constraint `(user_id, server_id)`, columns: `access_token`, `expires_at`.

```go
type TokenManager struct {
    db  *gorm.DB
    log *slog.Logger
}

// GetToken returns a valid token for the given server.
// Priority: DB cache (valid if expires_at > now+30s) → auto-refresh → error.
func (m *TokenManager) GetToken(ctx context.Context, userID, serverID string) (string, error)

// StoreToken persists a new token (upsert on user_id+server_id).
func (m *TokenManager) StoreToken(ctx context.Context, userID, serverID, token string, expiresAt time.Time) error
```

Two token refresh modes (configured per server):

**Client Credentials** (automatic):
```go
// RefreshClientCredentials exchanges client_id+secret for a new token and stores it.
func (m *TokenManager) RefreshClientCredentials(ctx context.Context, serverID string, cfg OAuthConfig) error
```

**OIDC** (user re-auth required):
- Return a structured error containing the authorization URL
- Loop surfaces this as a `message_appended` SSE event prompting the user to re-authorize

## 2.5 DB Migration

New migration: `000004_create_mcp_tokens.up.sql`

```sql
CREATE TABLE mcp_tokens (
    id          VARCHAR(36)  NOT NULL PRIMARY KEY,
    user_id     VARCHAR(255) NOT NULL,
    server_id   VARCHAR(255) NOT NULL,
    access_token TEXT        NOT NULL,
    expires_at  DATETIME(3)  NOT NULL,
    created_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at  DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    UNIQUE KEY uq_mcp_tokens_user_server (user_id, server_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

## 2.6 Files to Create/Modify

**New:**
- `internal/mcp/client.go` — interface + transport implementations
- `internal/mcp/router.go` — tool routing table
- `internal/mcp/token.go` — token manager
- `internal/mcp/stdio.go`, `sse.go`, `http.go` — transport implementations
- `db/migrations/000004_create_mcp_tokens.{up,down}.sql`

**Modified:**
- `internal/agent/loop.go` — inject `*mcp.Router`, extend tool dispatch
- `cmd/server/wire.go` + `wire_gen.go` — add MCP router + token manager providers

---

## Verification

```bash
go build ./...

# With a real MCP server running locally:
# Confirm tool list includes prefixed GitHub tools
# Send a message asking for recent commits → agent calls github-tools-sap__list_commits
```
