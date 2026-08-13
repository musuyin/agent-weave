package hook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github/musuyin/agent-weave/internal/model/repository"
	"gorm.io/gorm"
)

// highRiskTools are the tool names that require user approval before execution.
var highRiskTools = map[string]struct{}{
	"write_file":  {},
	"edit_file":   {},
	"run_command": {},
}

const approvalTimeout = 120 * time.Second

// ApprovalHook is a PRE_TOOL_USE hook that intercepts high-risk tool calls and
// blocks until the user approves or rejects them via the decision endpoint.
// Batch behaviour: once approved or rejected for a round, subsequent calls
// in the same round skip the prompt (checked via RoundApprovalState in ctx).
type ApprovalHook struct {
	db        *gorm.DB
	push      PushEventFunc
	convID    ConvIDFunc
	eventType string
	channels  sync.Map // blockID → chan bool
}

// NewApprovalHook creates an ApprovalHook backed by db.
// Configure must be called before the first tool call.
func NewApprovalHook(db *gorm.DB) *ApprovalHook {
	return &ApprovalHook{db: db}
}

// Configure wires in the SSE push function, convID extractor, and event type string.
// Called from agent.ProvideAgentService after HubRegistry is constructed.
func (h *ApprovalHook) Configure(push PushEventFunc, convID ConvIDFunc, eventType string) {
	h.push = push
	h.convID = convID
	h.eventType = eventType
}

func (h *ApprovalHook) RunPre(ctx context.Context, params *ToolCallParams) error {
	if _, isHighRisk := highRiskTools[params.Name]; !isHighRisk {
		return nil
	}

	// Batch: honour a previous decision from the same round without a new prompt.
	if rs, ok := roundApprovalFromCtx(ctx); ok {
		if rs.isRejected() {
			return fmt.Errorf("tool denied: batch was rejected")
		}
		if rs.isApproved() {
			return nil
		}
	}

	convID, ok := h.convID(ctx)
	if !ok {
		return fmt.Errorf("approval: no conversation context")
	}

	blockID := params.BlockID
	if blockID == "" {
		blockID = randomID()
	}

	// Fail-closed: if the DB write fails, deny the call rather than silently allow it.
	approval := &repository.Approval{
		BlockID:        blockID,
		ConversationID: convID,
		ToolName:       params.Name,
		Status:         "pending",
		CreatedAt:      time.Now(),
	}
	if err := h.db.WithContext(ctx).Create(approval).Error; err != nil {
		return fmt.Errorf("approval: failed to persist: %w", err)
	}

	ch := make(chan bool, 1)
	h.channels.Store(blockID, ch)
	defer h.channels.Delete(blockID)

	if h.push != nil {
		h.push(convID, h.eventType, map[string]any{
			"block_id":  blockID,
			"tool_name": params.Name,
		})
	}

	select {
	case approved := <-ch:
		if !approved {
			if rs, ok := roundApprovalFromCtx(ctx); ok {
				rs.setRejected()
			}
			return fmt.Errorf("tool denied: %s was rejected", params.Name)
		}
		if rs, ok := roundApprovalFromCtx(ctx); ok {
			rs.setApproved()
		}
		return nil
	case <-time.After(approvalTimeout):
		if rs, ok := roundApprovalFromCtx(ctx); ok {
			rs.setRejected()
		}
		return fmt.Errorf("tool denied: approval for %s timed out", params.Name)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Signal resolves the pending approval identified by blockID.
// Returns true if a waiting RunPre goroutine was notified.
// Called from the decision handler after writing the DB record (invariant F).
func (h *ApprovalHook) Signal(blockID string, approved bool) bool {
	v, ok := h.channels.Load(blockID)
	if !ok {
		return false
	}
	select {
	case v.(chan bool) <- approved:
		return true
	default:
		return false
	}
}

func randomID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
