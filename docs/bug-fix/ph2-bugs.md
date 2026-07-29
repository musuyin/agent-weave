# Phase 2 Bug Fix Reminders

Discovered during code review on 2026-07-29.

---

## BUG-1: `CallTool` silently drops non-text MCP content

**File:** `internal/mcp/http_client.go:91–97`

**Symptom:** If an MCP server returns `ImageContent` or `EmbeddedResource` blocks, the tool result is an empty string with no error. The model receives a blank tool result and cannot distinguish it from a genuine empty response.

**Root cause:**
```go
for _, content := range result.Content {
    if tc, ok := content.(*sdkmcp.TextContent); ok {
        sb.WriteString(tc.Text)
    }
    // ImageContent, EmbeddedResource silently dropped
}
```

**Fix:** Log a warning for each skipped non-text block so the operator knows content was lost. Optionally serialize non-text blocks as a JSON fallback string so the model has some signal:
```go
for _, content := range result.Content {
    switch tc := content.(type) {
    case *sdkmcp.TextContent:
        sb.WriteString(tc.Text)
    default:
        log.Warn("mcp: skipping non-text content block", "type", fmt.Sprintf("%T", content))
    }
}
```

---

## BUG-2: `CallTool` ignores `result.IsError`, masking MCP tool errors

**File:** `internal/mcp/http_client.go:83–98`

**Symptom:** When an MCP tool signals failure via `CallToolResult.IsError == true`, the implementation returns `(text, nil)`. The agent loop sets `IsError: false` on the `tool_result` block and `AuditHook` logs no error. The model sees an error message as a normal success result.

**Root cause:**
```go
result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{...})
if err != nil {
    return "", err
}
// result.IsError is never checked
```

**Fix:**
```go
if result.IsError {
    return text, fmt.Errorf("mcp tool error: %s", text)
}
```

---

## DESIGN-1: Spec/implementation mismatch on `Route` return signature

**Files:** `docs/ph2-mcp.md §2.2`, `internal/mcp/router.go:77`

**Symptom:** The spec defines `Route(toolName string) (Client, bool)` but the implementation returns `(Client, string, bool)` — the extra string is the unprefixed tool name. This is correct behavior (without it `CallTool` would need to re-strip the prefix), but the spec is wrong.

**Fix:** Update `docs/ph2-mcp.md §2.2` to reflect the actual signature:
```go
func (r *Router) Route(prefixedName string) (Client, string, bool)
```

---

## DESIGN-2: `tool.ToolDef.Handler = nil` for MCP tools — silent panic risk

**File:** `internal/mcp/router.go:58–63`

**Symptom:** MCP tools are stored in the global tool def list with `Handler: nil`. Any code path that calls `def.Handler(ctx, params)` for an MCP tool will panic with a nil function call. Currently safe because `buildToolParams` only reads schema fields, but fragile.

**Root cause:**
```go
r.tools = append(r.tools, tool.ToolDef{
    Name:        prefixed,
    Description: t.Description,
    InputSchema: t.InputSchema,
    Handler:     nil, // dispatched via Route(), not Handler
})
```

**Fix:** Add a comment on the `nil` field explaining the invariant. Longer term, consider separating the "schema for the API" list from the "dispatchable tools" list so there is no `nil` handler to trip over.

---

## DESIGN-3: No tests for `internal/mcp`

**Package:** `internal/mcp`

**Symptom:** Zero test coverage for Phase 2 code. The `SchemaFields` helper handles a two-branch type switch and a `[]any` cast that are both easy to regress. `Router` logic (prefix routing, `AllTools` aggregation) is also untested.

`HTTPClient` requires a live server, but the following are unit-testable without network:

| Test | What it checks |
|---|---|
| `TestSchemaFields_MapInput` | `map[string]any` schema extracts properties + required correctly |
| `TestSchemaFields_RawJSONInput` | `json.RawMessage` schema is unmarshalled and extracted correctly |
| `TestSchemaFields_InvalidInput` | Unknown type returns `(nil, nil)` without panic |
| `TestRouter_Route` | Prefixed name resolves to correct client + unprefixed name |
| `TestRouter_AllTools` | Returns all tools with prefixed names and correct schemas |
| `TestRouter_Empty` | Empty config → no routes, no tools, no panic |

---

## MISSING-1: Token management not implemented (spec §2.4, §2.5)

**Files:** `internal/mcp/token.go` (absent), `db/migrations/000004_*` (absent)

**Symptom:** The spec defines a `TokenManager` with DB-cached per-user tokens, client-credentials auto-refresh, and an OIDC re-auth flow. None of this exists. Auth is currently a static `Authorization` header in `config.yaml` — sufficient for a single shared token but incompatible with per-user OAuth.

**What's missing:**
- `internal/mcp/token.go` — `TokenManager`, `GetToken`, `StoreToken`, `RefreshClientCredentials`
- `db/migrations/000004_create_mcp_tokens.up.sql` / `down.sql`
- Per-server `OAuthConfig` in `MCPServerConfig`
- OIDC re-auth error surfaced as `message_appended` SSE event

**Fix:** Implement in a follow-up. Until then, document the static-header limitation in `docs/deferred.md`.

---

## MISSING-2: stdio and SSE transports not implemented

**Files:** `internal/mcp/` (no `stdio.go` or `sse.go`)

**Symptom:** The spec lists three transport types (`stdio`, `sse`, `http`). Only `http` is implemented. The router skips non-HTTP servers with a warning, so there is no runtime failure, but local MCP servers (stdio) cannot be connected.

**Fix:** Implement when needed. The `Client` interface is defined correctly so adding new transports is straightforward.
