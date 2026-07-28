package tool

import (
	"context"
	"encoding/json"
	"sync"
)

// Handler executes a tool call and returns its result as a string.
// Errors returned here become error tool_results sent back to the model.
type Handler func(ctx context.Context, params json.RawMessage) (string, error)

// ToolDef describes a single tool available to the agent.
type ToolDef struct {
	Name        string
	Description string
	InputSchema any // Go struct; callers marshal to JSON Schema for the Anthropic SDK
	Handler     Handler
}

var (
	mu             sync.RWMutex
	globalRegistry = map[string]ToolDef{}
)

// Register adds def to the global registry. Called from builtin init() functions.
func Register(def ToolDef) {
	mu.Lock()
	defer mu.Unlock()
	globalRegistry[def.Name] = def
}

// Get returns the ToolDef for name, or false if not found.
func Get(name string) (ToolDef, bool) {
	mu.RLock()
	defer mu.RUnlock()
	d, ok := globalRegistry[name]
	return d, ok
}

// All returns all registered tool definitions in an unspecified order.
func All() []ToolDef {
	mu.RLock()
	defer mu.RUnlock()
	defs := make([]ToolDef, 0, len(globalRegistry))
	for _, d := range globalRegistry {
		defs = append(defs, d)
	}
	return defs
}
