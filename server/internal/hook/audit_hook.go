package hook

import (
	"context"
	"encoding/json"
	"log/slog"
)

// AuditHook logs tool invocations: name, parameter keys (never values), and outcome.
type AuditHook struct {
	log *slog.Logger
}

func NewAuditHook(log *slog.Logger) *AuditHook {
	return &AuditHook{log: log}
}

// RunPost logs the tool call. It runs in a goroutine so it must not mutate params.
func (h *AuditHook) RunPost(_ context.Context, params ToolCallParams, _ string, err error) {
	keys := paramKeys(params.Params)
	if err != nil {
		h.log.Info("tool call failed", "tool", params.Name, "param_keys", keys, "error", err)
		return
	}
	h.log.Info("tool call succeeded", "tool", params.Name, "param_keys", keys)
}

// paramKeys returns the top-level key names from a JSON object.
// Returns nil if params is not a valid JSON object.
func paramKeys(params json.RawMessage) []string {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(params, &obj); err != nil {
		return nil
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	return keys
}

// ProvideHookChain is a Wire provider that assembles the full hook chain.
// Pre: SecurityHook → (Phase 5: ApprovalHook)
// Post: AuditHook
func ProvideHookChain(sec *SecurityHook, audit *AuditHook) *Chain {
	return NewChain(
		[]PreHook{sec},
		[]PostHook{audit},
	)
}
