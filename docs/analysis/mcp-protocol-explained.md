# Analysis: How MCP Works — Protocol, Transport, and What "MCP Server" Actually Means

---

## 1. Your intuition is correct — and incomplete

> "MCP server is just sending a formatted message (like an HTTP request) to the service provider
> and the provider responds with messages. Just like entering a URL in the browser."

This is **correct at the transport layer**. An MCP call over HTTP really is a POST request with a
JSON body, and the server returns a JSON response. In that sense it is exactly like a browser
fetching a URL.

But the analogy stops there. What makes MCP interesting is the **layer above the transport**:
a standardized protocol that lets the AI model discover what tools exist, what their inputs look
like, and call them — all without any custom integration code per tool.

---

## 2. What MCP actually is

MCP (Model Context Protocol) is a **standard interface** between an AI agent and external
capability providers. Think of it as a universal plug shape:

- Any service that wants to expose tools to an AI agent implements the MCP server side.
- Any AI agent that wants to use external tools implements the MCP client side.
- Neither side needs to know anything about the other's internals.

Before MCP, every tool integration was bespoke: you'd write custom code to call the GitHub API,
parse its response, and describe the tool to the model. With MCP, GitHub (or any provider) runs
an MCP server. Your agent connects once, asks "what tools do you have?", and gets back a
machine-readable list it can use immediately.

---

## 3. The MCP protocol — two phases

### Phase 1: Handshake + discovery (startup)

```
Agent                           MCP Server
  │── POST /  {initialize} ────►│   "I am agent-weave v0.1.0"
  │◄─ {serverInfo, capabilities}─│   "I am github-tools, I support tools"
  │── POST /  {tools/list} ─────►│
  │◄─ [{name, description,       │
  │     inputSchema}, ...]  ─────│   "Here are my tools and their JSON Schemas"
```

This happens **once at startup** in `mcp/router.go:40–50`:
```go
client, err := NewHTTPClient(ctx, srv.URL, srv.Headers, log)  // handshake
mcpTools, err := client.ListTools(ctx)                         // discovery
```

The result is stored in the router and injected into every Anthropic API call as part of the
tool list. The model now knows these tools exist.

### Phase 2: Tool call (per agent turn)

```
Agent                           MCP Server
  │── POST /  {tools/call,  ────►│
  │    name: "search_pull_requests",
  │    arguments: {query: "..."}} │
  │◄─ {content: [{type:"text",   │
  │     text: "...results..."}]} ─│
```

This happens in `mcp/http_client.go:85–88`:
```go
result, err := c.session.CallTool(ctx, &sdkmcp.CallToolParams{
    Name:      name,       // unprefixed: "search_pull_requests"
    Arguments: args,       // {"query": "..."}
})
```

The MCP SDK (`modelcontextprotocol/go-sdk`) handles serializing these into the correct JSON-RPC
envelope and sending the HTTP POST. You never write the raw HTTP yourself.

---

## 4. The transport detail: Streamable HTTP

The transport used here is **MCP Streamable HTTP** (`sdkmcp.StreamableClientTransport`).

Unlike a plain request-response, this transport keeps the HTTP connection open and can stream
multiple JSON-RPC messages back. This allows the server to send partial results or progress
events before the final response — similar to how SSE works for the agent→frontend path.

For most tool calls (like a GitHub search) the server just sends one response and closes. But
the protocol supports streaming when the server needs it.

---

## 5. How this codebase wires it all together

```
startup
  ProvideMCPRouter()
    for each server in config:
      NewHTTPClient()         → TCP connect + MCP initialize handshake
      client.ListTools()      → tools/list RPC
      store in router.routes["github-tools__search_pull_requests"] = {client, "search_pull_requests"}
      store in router.tools   → ToolDef with Handler: nil

per agent turn
  buildToolParams()
    tool.All() + mcpRouter.AllTools()   → merged list sent to Anthropic API
                                          model sees all tools, built-in and MCP, identically

  model decides to call "github-tools__search_pull_requests"
  dispatchToolsFromBlocks()
    mcpRouter.Route("github-tools__search_pull_requests")
      → returns (client, "search_pull_requests", true)
    client.CallTool(ctx, "search_pull_requests", params)
      → MCP tools/call RPC over HTTP
      → returns text result
    result stored as tool_result content block
    persistMessage() → saved to DB
```

---

## 6. MCP vs "just calling an API directly"

| | Direct API call (e.g. calling GitHub REST yourself) | MCP |
|---|---|---|
| Tool description | You write it manually | Server provides it (JSON Schema) |
| Auth | You manage tokens in your code | Server manages it; you only pass headers |
| Adding a new tool | Code change in your agent | Zero change — server publishes it, agent discovers it |
| Protocol knowledge | Custom per provider | Uniform: `tools/list` + `tools/call` |
| What the agent sees | What you manually describe | What the server declares |

The key insight: **the agent discovers capabilities at runtime**. You do not hard-code what
`github-tools` can do — you ask it at startup and it tells you.

---

## 7. Summary

MCP is a standardized RPC protocol (JSON-RPC over HTTP) with two operations that matter:

1. **`tools/list`** — "what can you do?" → returns names, descriptions, JSON Schemas
2. **`tools/call`** — "do this thing" → returns text results

The AI model never speaks MCP directly. The agent (this codebase) acts as a **bridge**:
it speaks Anthropic's API on one side and MCP on the other, translating between them.
The model just sees a tool named `github-tools__search_pull_requests` in its tool list — it has
no idea whether that tool is a local Go function or a remote MCP server call.
