package hook

import "context"

// SecurityHook blocks tool calls whose name appears in the denylist.
type SecurityHook struct {
	denylist map[string]struct{}
}

// NewSecurityHook creates a SecurityHook with the given denied tool names.
// Pass nil or an empty slice for an empty denylist (Phase 1 default).
func NewSecurityHook(denied []string) *SecurityHook {
	m := make(map[string]struct{}, len(denied))
	for _, name := range denied {
		m[name] = struct{}{}
	}
	return &SecurityHook{denylist: m}
}

// ProvideSecurityHook is a Wire provider that creates a SecurityHook with an empty denylist.
func ProvideSecurityHook() *SecurityHook {
	return NewSecurityHook(nil)
}

func (h *SecurityHook) RunPre(_ context.Context, params *ToolCallParams) error {
	if _, blocked := h.denylist[params.Name]; blocked {
		return &ErrToolDenied{Name: params.Name}
	}
	return nil
}

// ErrToolDenied is returned when a tool is blocked by the SecurityHook.
type ErrToolDenied struct {
	Name string
}

func (e *ErrToolDenied) Error() string {
	return "tool denied by security policy: " + e.Name
}
