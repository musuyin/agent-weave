package hook

import (
	"context"
	"encoding/json"
	"sync"
)

// ToolCallParams holds the name, raw JSON parameters, and block ID of a tool call.
// PreHooks may mutate Params to change the arguments the tool sees.
type ToolCallParams struct {
	Name    string
	Params  json.RawMessage
	BlockID string // tool_use block ID assigned by the LLM
}

// PushEventFunc pushes an SSE event to a conversation's hub.
// Defined here to avoid an import cycle with the agent package.
type PushEventFunc func(convID string, eventType string, data any)

// ConvIDFunc extracts a conversation ID from a context.
type ConvIDFunc func(ctx context.Context) (string, bool)

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
		go h.RunPost(ctx, params, result, err)
	}
}

// RoundApprovalState tracks whether the current tool-dispatch round has been
// approved or rejected by the user. A pointer is injected into context via
// WithRoundApproval before the dispatch loop starts, enabling the ApprovalHook
// to skip redundant prompts for subsequent high-risk tools in the same round.
type RoundApprovalState struct {
	mu       sync.Mutex
	approved bool
	rejected bool
}

type roundApprovalKey struct{}

// WithRoundApproval wraps ctx with a fresh RoundApprovalState for the current round.
// Called from agent.dispatchToolsFromBlocks before the tool loop.
func WithRoundApproval(ctx context.Context) context.Context {
	return context.WithValue(ctx, roundApprovalKey{}, &RoundApprovalState{})
}

func roundApprovalFromCtx(ctx context.Context) (*RoundApprovalState, bool) {
	s, ok := ctx.Value(roundApprovalKey{}).(*RoundApprovalState)
	return s, ok
}

func (r *RoundApprovalState) setApproved() { r.mu.Lock(); r.approved = true; r.mu.Unlock() }
func (r *RoundApprovalState) setRejected() { r.mu.Lock(); r.rejected = true; r.mu.Unlock() }
func (r *RoundApprovalState) isApproved() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.approved
}
func (r *RoundApprovalState) isRejected() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rejected
}
