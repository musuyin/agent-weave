package agent

import "context"

type ctxKey int

const convIDKey ctxKey = iota

// withConversationID stores the conversation ID in ctx for use by tool handlers.
func withConversationID(ctx context.Context, convID string) context.Context {
	return context.WithValue(ctx, convIDKey, convID)
}

// conversationIDFromCtx retrieves the conversation ID injected by the agent loop.
// Returns ("", false) if absent.
func conversationIDFromCtx(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(convIDKey).(string)
	return v, ok && v != ""
}
