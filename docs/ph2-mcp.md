# ph2 — MCP Integration (MCP 接入)

**Goal**: Connect the agent to external MCP servers (two GitHub instances + Jira).
**Status**: ✓ Core routing complete. Token management deferred (see §2.4).
**Prerequisite**: ph1 complete (tool registry + hook chain).

---

## 2.1 MCPClient Interface (`internal/mcp/client.go`)

```go
type Tool struct {
    Name        string
    Description string
    InputSchema json.RawMessage // JSON Schema object as returned by the MCP server
}

type Client interface {
    ListTools(ctx context.Context) ([]Tool, error)
    CallTool(ctx context.Context, name string, params json.RawMessage) (string, error)
    Close() error
}
```

One transport implementation exists today:

| Transport | Constructor | Use case |
|---|---|---|
| HTTP streamable | `NewHTTPClient(ctx, url, headers, log)` | Remote MCP over HTTP (Streamable MCP protocol) |

stdio and SSE transports are deferred — see `docs/deferred.md`.

SDK: `github.com/modelcontextprotocol/go-sdk v1.3.1` (`sdkmcp` import alias).

## 2.2 HTTP Transport (`internal/mcp/http_client.go`)

```go
type HTTPClient struct {
    session *sdkmcp.ClientSession
    log     *slog.Logger
}

func NewHTTPClient(ctx context.Context, url string, headers map[string]string, log *slog.Logger) (*HTTPClient, error)
```

**Auth**: static headers injected via `headerTransport` (a custom `http.RoundTripper`). All headers from `config.yaml mcp.servers[*].headers` are added to every request. Typically `Authorization: Bearer <PAT>`.

**`ListTools`**: calls `session.ListTools`, marshals each tool's `InputSchema` to `json.RawMessage`. Falls back to `{}` if marshal fails.

**`CallTool`**:
1. Unmarshals `params json.RawMessage` → `map[string]any`
2. Calls `session.CallTool` with `CallToolParams{Name, Arguments}`
3. Concatenates `*sdkmcp.TextContent` blocks; logs a `Warn` for any non-text block (image, embedded resource) that cannot be forwarded to the model
4. Truncates result to `tool.MaxToolResultBytes` (16 KB)
5. Checks `result.IsError` — returns `fmt.Errorf("mcp tool error: %s", text)` when true, so the agent loop marks the `tool_result` block as an error and `AuditHook` records it correctly

**`Close`**: calls `session.Close()`.

## 2.3 Tool Routing Table (`internal/mcp/router.go`)

Built once at startup by `ProvideMCPRouter`. Maps `prefixedToolName → (Client, unprefixedName)`.

```go
type Router struct {
    routes map[string]routeEntry   // prefixed name → {client, unprefixedName}
    tools  []tool.ToolDef          // all MCP tools as ToolDef (Handler always nil)
    log    *slog.Logger
}

// ProvideMCPRouter is the Wire provider. Soft-fails per server — agent runs
// with whatever servers are reachable; unreachable servers are logged and skipped.
func ProvideMCPRouter(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Router, func(), error)

// Route returns the client and unprefixed name for a prefixed tool name.
func (r *Router) Route(prefixedName string) (Client, string, bool)

// AllTools returns all MCP tools as []tool.ToolDef for injection into the Anthropic SDK call.
// Handler is nil on all returned ToolDefs — dispatch goes through Route(), not Handler.
func (r *Router) AllTools() []tool.ToolDef

// NewRouterForTest constructs a Router from a client and tool list, bypassing
// config and network. Used by unit tests only.
func NewRouterForTest(prefix string, client Client, tools []Tool) *Router
```

**Startup sequence** (inside `ProvideMCPRouter`):
1. For each `cfg.MCP.Servers` entry, skip if transport is not `"http"` (logs warn)
2. `NewHTTPClient` — skip server on connect error (logs warn)
3. `client.ListTools` — skip server on error, close the client (logs warn)
4. For each tool: prepend `prefix + "__"` to the name (if prefix non-empty), store in `routes` and `tools`
5. Return cleanup func that closes all connected clients

**Tool dispatch** (in `internal/agent/tool_dispatch.go`):
```go
if preErr != nil {
    result = "tool call denied: " + preErr.Error()
} else if client, unprefixed, ok := s.mcpRouter.Route(params.Name); ok {
    result, toolErr = client.CallTool(ctx, unprefixed, params.Params)
    ...
} else if def, ok := tool.Get(params.Name); ok {
    result, toolErr = def.Handler(ctx, params.Params)
    ...
} else {
    result = "unknown tool: " + params.Name
}
```

Dispatch order: denied by pre-hook → MCP tool → builtin tool → unknown.

## 2.4 Name-Prefix Deduplication

Both GitHub MCP servers expose identically named tools (e.g. `search_commits`, `list_pull_requests`). The `prefix` field in `MCPServerConfig` is prepended with `__` as a separator:

```
github-tools__search_commits
github-wdf__search_commits
```

Configured in `config.yaml`:
```yaml
mcp:
  servers:
    - name: "github-tools"
      prefix: "github-tools"
      transport: "http"
      url: "https://mcp.github.tools.sap/mcp"
      headers:
        Authorization: "Bearer ghp_xxx"
    - name: "github-wdf"
      prefix: "github-wdf"
      transport: "http"
      url: "https://github-mcp.wdf.sap.corp/mcp"
      headers:
        Authorization: "Bearer ghp_yyy"
```

If `prefix` is empty, the tool name is used as-is (no separator added).

## 2.5 SchemaFields Helper (`internal/mcp/router.go`)

MCP tool schemas arrive as `json.RawMessage`. Builtin tool schemas are `map[string]any`. `buildToolParams` needs to extract `properties` and `required` from both types:

```go
func SchemaFields(schema any) (properties any, required []string)
```

Handles two input types:
- `map[string]any` — used directly
- `json.RawMessage` — unmarshalled into `map[string]any` first
- anything else — returns `(nil, nil)`

Extracts `m["properties"]` and casts `m["required"]` from `[]any` to `[]string`. Used in `buildToolParams` for both builtin and MCP tools.

## 2.6 MCP OAuth Token Management — DEFERRED

The spec called for a `TokenManager` with:
- DB table `mcp_tokens` (unique on `user_id + server_id`)
- `GetToken` with DB cache + auto-refresh
- Client-credentials and OIDC refresh modes

**Current state**: not implemented. Auth is a static `Authorization: Bearer <PAT>` header in `config.yaml`. See `docs/deferred.md` for details and workaround.

## 2.7 Wire DI

```go
// wire.go
mcp.ProvideMCPRouter,           // (ctx, *config.Config, *slog.Logger) → (*Router, func(), error)
agent.ProvideAgentService,      // now accepts *mcp.Router
```

`wire_gen.go` wires cleanup2 (MCP client Close calls) into the app shutdown function.

## 2.8 Config

`internal/config/config.go`:
```go
type MCPConfig struct {
    Servers []MCPServerConfig `yaml:"servers"`
}

type MCPServerConfig struct {
    Name      string            `yaml:"name"`
    Prefix    string            `yaml:"prefix"`
    Transport string            `yaml:"transport"`  // only "http" supported
    URL       string            `yaml:"url"`
    Headers   map[string]string `yaml:"headers"`
}
```

`Config.MCP MCPConfig` is the top-level field, populated from `config.yaml mcp:` key.

---

## Architecture Invariants

| # | Rule | Where enforced |
|---|---|---|
| Route-before-Handler | MCP tools have `Handler: nil` — always dispatch via `Router.Route()` | `router.go`, `tool_dispatch.go` |
| Soft-fail at startup | Unreachable MCP servers are skipped with a warning; agent starts normally | `router.go:ProvideMCPRouter` |
| IsError propagated | `CallTool` returns a Go error when `result.IsError == true` | `http_client.go:CallTool` |
| Non-text content logged | Non-text MCP blocks are warned, never silently dropped | `http_client.go:CallTool` |
| Truncation applied | MCP results truncated to 16 KB, same as builtin tools | `http_client.go:CallTool` |

---

## File List

| File | Key exports |
|---|---|
| `internal/mcp/client.go` | `Tool`, `Client` interface |
| `internal/mcp/http_client.go` | `HTTPClient`, `NewHTTPClient` |
| `internal/mcp/router.go` | `Router`, `ProvideMCPRouter`, `Route`, `AllTools`, `NewRouterForTest`, `SchemaFields` |

---

## Tests

| File | Coverage |
|---|---|
| `test/mcp/mcp_test.go` | `SchemaFields` (map, raw JSON, invalid, no-required, partial), `Router` (route hit/miss, AllTools names, nil Handler, no-prefix passthrough, empty config), CallTool roundtrip via fakeClient, schema preservation through AllTools |

---

## Verification

```bash
go build ./...
go test ./test/mcp/...

# With real MCP servers configured in config.yaml:
# - Start server; logs should show "mcp: server connected" with tool count
# - Send a message referencing a GitHub repo; agent should call github-tools__<tool>
# - Confirm prefixed tool names appear in the Anthropic API call (check debug logs)
```
