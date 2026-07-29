package mcp

import (
	"context"
	"encoding/json"
	"log/slog"

	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/tool"
)

type routeEntry struct {
	client         Client
	unprefixedName string
}

// Router maps prefixed tool names (e.g. "github-tools__list_commits") to the
// correct MCP client and unprefixed name.
type Router struct {
	routes map[string]routeEntry
	tools  []tool.ToolDef
	log    *slog.Logger
}

// ProvideMCPRouter builds the router at startup. Wire provider.
// Errors connecting to individual servers are logged as warnings — the agent
// runs with whatever servers are reachable.
func ProvideMCPRouter(ctx context.Context, cfg *config.Config, log *slog.Logger) (*Router, func(), error) {
	r := &Router{
		routes: make(map[string]routeEntry),
		log:    log,
	}
	var clients []Client

	for _, srv := range cfg.MCP.Servers {
		if srv.Transport != "http" {
			log.Warn("mcp: unsupported transport, skipping", "server", srv.Name, "transport", srv.Transport)
			continue
		}
		client, err := NewHTTPClient(ctx, srv.URL, srv.Headers, log)
		if err != nil {
			log.Warn("mcp: failed to connect, skipping", "server", srv.Name, "error", err)
			continue
		}
		mcpTools, err := client.ListTools(ctx)
		if err != nil {
			log.Warn("mcp: failed to list tools, skipping", "server", srv.Name, "error", err)
			_ = client.Close()
			continue
		}
		clients = append(clients, client)
		for _, t := range mcpTools {
			prefixed := t.Name
			if srv.Prefix != "" {
				prefixed = srv.Prefix + "__" + t.Name
			}
			r.routes[prefixed] = routeEntry{client: client, unprefixedName: t.Name}
			r.tools = append(r.tools, tool.ToolDef{
				Name:        prefixed,
				Description: t.Description,
				InputSchema: t.InputSchema,
				// Handler is intentionally nil: MCP tools are dispatched via
				// Router.Route() in dispatchToolsFromBlocks, never via Handler.
				// Do not call Handler on these ToolDefs — it will panic.
				Handler: nil,
			})
		}
		log.Info("mcp: server connected", "server", srv.Name, "tools", len(mcpTools))
	}

	cleanup := func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}
	return r, cleanup, nil
}

// Route returns the client and unprefixed name for a prefixed tool name.
func (r *Router) Route(prefixedName string) (Client, string, bool) {
	e, ok := r.routes[prefixedName]
	if !ok {
		return nil, "", false
	}
	return e.client, e.unprefixedName, true
}

// AllTools returns all MCP tools as ToolDef for injection into the Anthropic API.
func (r *Router) AllTools() []tool.ToolDef {
	return r.tools
}

// NewRouterForTest constructs a Router directly from a client and tool list,
// bypassing config and network. Used by unit tests only.
func NewRouterForTest(prefix string, client Client, tools []Tool) *Router {
	r := &Router{
		routes: make(map[string]routeEntry),
		log:    slog.Default(),
	}
	for _, t := range tools {
		prefixed := t.Name
		if prefix != "" {
			prefixed = prefix + "__" + t.Name
		}
		r.routes[prefixed] = routeEntry{client: client, unprefixedName: t.Name}
		r.tools = append(r.tools, tool.ToolDef{
			Name:        prefixed,
			Description: t.Description,
			InputSchema: t.InputSchema,
			Handler:     nil,
		})
	}
	return r
}

// schemaToToolInputSchemaFields extracts properties and required from an InputSchema
// that may be json.RawMessage or map[string]any.
func SchemaFields(schema any) (properties any, required []string) {
	var m map[string]any
	switch v := schema.(type) {
	case map[string]any:
		m = v
	case json.RawMessage:
		_ = json.Unmarshal(v, &m)
	default:
		return nil, nil
	}
	properties = m["properties"]
	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				required = append(required, s)
			}
		}
	}
	return properties, required
}
