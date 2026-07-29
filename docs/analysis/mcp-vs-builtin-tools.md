# Analysis: MCP Tools vs Built-in Tools — Dispatch, Logging, and How to Tell Them Apart

---

## 1. Why they look the same in logs

The log line `"tool call succeeded" "tool":"github-tools__search_pull_requests"` comes from `hook/audit_hook.go:25`, which fires in the **post-hook chain** after every tool call — MCP or built-in. The `AuditHook` only receives `ToolCallParams{Name, Params}` and has no knowledge of which path was taken.

---

## 2. Dispatch code path (`agent/tool_dispatch.go:85–98`)

```go
} else if client, unprefixed, ok := s.mcpRouter.Route(params.Name); ok {
    // ← MCP path: routed to an MCP HTTP client
    result, toolErr = client.CallTool(ctx, unprefixed, params.Params)

} else if def, ok := tool.Get(params.Name); ok {
    // ← built-in path: called via Handler function pointer
    result, toolErr = def.Handler(ctx, params.Params)

} else {
    result = "unknown tool: " + params.Name
}
```

The decision is: **try MCP router first, fall back to built-in registry**.
Both paths then call the same post-hook (`s.chain.FirePost`) with identical arguments, so
`AuditHook` sees the same shape regardless.

### How to tell them apart structurally

| | Built-in tool | MCP tool |
|---|---|---|
| Registered in | `tool.globalRegistry` (`tool.Register()`) | `mcp.Router.routes` (`Router.GetOrCreate()`) |
| `ToolDef.Handler` | non-nil function | `nil` (intentional — would panic if called) |
| Name prefix | plain (e.g. `fetch_url`) | `<server>__<tool>` (e.g. `github-tools__search_pull_requests`) |
| Dispatch | `def.Handler(ctx, params)` | `client.CallTool(ctx, unprefixed, params)` |

The `__` separator in the tool name is the visible signal: MCP tools are always
`serverName__toolName`.

---

## 3. How to add a distinct log for MCP calls

The cleanest approach is to log at the dispatch site in `tool_dispatch.go`, right where the
routing decision is made, before the existing post-hook fires:

```go
} else if client, unprefixed, ok := s.mcpRouter.Route(params.Name); ok {
    s.log.Info("mcp tool call", "tool", params.Name)          // ← add this
    result, toolErr = client.CallTool(ctx, unprefixed, params.Params)
```

This fires only for MCP tools and uses a distinct `msg` field (`"mcp tool call"` vs
`"tool call succeeded"`), so you can grep or filter on it separately.

Alternatively, add a dedicated `PostHook` that checks the name prefix:

```go
// In a new hook file, e.g. hook/mcp_audit_hook.go
func (h *MCPAuditHook) RunPost(_ context.Context, params ToolCallParams, result string, err error) {
    if !strings.Contains(params.Name, "__") {
        return   // not an MCP tool
    }
    parts := strings.SplitN(params.Name, "__", 2)
    h.log.Info("mcp tool call succeeded", "server", parts[0], "tool", parts[1])
}
```

Then register it in `ProvideHookChain` alongside `AuditHook`.

The hook approach is cleaner because it keeps all audit concerns in one place and doesn't touch the dispatch logic.

---

## 4. Summary

- MCP and built-in tools share the **same dispatch function and the same post-hook chain** — that is why the logs look identical.
- The only runtime distinguisher is `mcpRouter.Route()` returning true, which happens before the built-in registry is checked.
- MCP tool names always contain `__`; built-in names never do — this is the simplest filter.
- To get a distinct log title, either log at the dispatch site in `tool_dispatch.go` (one line), or add a prefix-checking `PostHook` (more extensible).
