package hook

import (
	"context"
	"encoding/json"
)

// ToolCallParams holds the name and raw JSON parameters of a tool call.
// PreHooks may mutate Params to change the arguments the tool sees.
type ToolCallParams struct {
	Name   string
	Params json.RawMessage
}

// PreHook runs synchronously before a tool call.
// Returning a non-nil error aborts the call and propagates the error as a tool_result.
type PreHook interface {
	RunPre(ctx context.Context, params *ToolCallParams) error
}

// PostHook is a pure observer that runs after a tool call.
// It is invoked in a separate goroutine and its return value is ignored.
type PostHook interface {
	RunPost(ctx context.Context, params ToolCallParams, result string, err error)
}

// Chain holds an ordered list of pre- and post-hooks.
type Chain struct {
	pre  []PreHook
	post []PostHook
}

// NewChain creates a Chain with the given hooks.
func NewChain(pre []PreHook, post []PostHook) *Chain {
	return &Chain{pre: pre, post: post}
}

// FirePre runs all PreHooks serially. The first error aborts the chain.
// Must be called BEFORE writing message history (invariant A).
func (c *Chain) FirePre(ctx context.Context, params *ToolCallParams) error {
	for _, h := range c.pre {
		if err := h.RunPre(ctx, params); err != nil {
			return err
		}
	}
	return nil
}

// FirePost spawns one goroutine per PostHook. Non-blocking.
func (c *Chain) FirePost(ctx context.Context, params ToolCallParams, result string, err error) {
	for _, h := range c.post {
		h := h
		go h.RunPost(ctx, params, result, err)
	}
}
