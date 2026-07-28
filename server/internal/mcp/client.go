package mcp

import (
	"context"
	"encoding/json"
)

// Tool represents a single tool exposed by an MCP server.
type Tool struct {
	Name        string
	Description string
	InputSchema json.RawMessage // JSON Schema object as returned by the MCP server
}

// Client abstracts over different MCP transport implementations.
type Client interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, params json.RawMessage) (string, error)
	Close() error
}
