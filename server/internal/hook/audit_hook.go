package hook

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github/musuyin/agent-weave/internal/model/repository"
	"gorm.io/gorm"
)

// AuditHook logs tool invocations: name, parameter keys (never values), and outcome.
// It also writes a structured record to the audit_logs table (invariant H).
type AuditHook struct {
	log    *slog.Logger
	db     *gorm.DB
	convID ConvIDFunc // set via Configure
}

func NewAuditHook(log *slog.Logger, db *gorm.DB) *AuditHook {
	return &AuditHook{log: log, db: db}
}

// Configure sets the convID extractor. Called from agent.ProvideAgentService.
func (h *AuditHook) Configure(convID ConvIDFunc) {
	h.convID = convID
}

// RunPost logs the tool call and persists a structured audit record.
// Runs in a goroutine — uses context.Background() for DB writes so cancellation
// of the request context does not silently drop audit entries.
func (h *AuditHook) RunPost(ctx context.Context, params ToolCallParams, _ string, err error) {
	keys := paramKeys(params.Params)
	if err != nil {
		h.log.Info("tool call failed", "tool", params.Name, "param_keys", keys, "error", err)
	} else {
		h.log.Info("tool call succeeded", "tool", params.Name, "param_keys", keys)
	}

	if h.db == nil || h.convID == nil {
		return
	}

	convID, _ := h.convID(ctx)
	keysJSON, _ := json.Marshal(keys)

	entry := &repository.AuditLog{
		ConversationID: convID,
		ToolName:       params.Name,
		ParamKeys:      string(keysJSON),
		Success:        err == nil,
		CreatedAt:      time.Now(),
	}
	if err != nil {
		entry.ErrorMessage = err.Error()
	}

	_ = h.db.WithContext(context.Background()).Create(entry).Error
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
// Pre: SecurityHook → ApprovalHook
// Post: AuditHook
func ProvideHookChain(sec *SecurityHook, audit *AuditHook, approval *ApprovalHook) *Chain {
	return NewChain(
		[]PreHook{sec, approval},
		[]PostHook{audit},
	)
}
