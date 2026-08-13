package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/model/repository"
)

// ApprovalHandler handles the tool-call approval decision endpoint.
type ApprovalHandler struct {
	db           *gorm.DB
	approvalHook *hook.ApprovalHook
}

func NewApprovalHandler(db *gorm.DB, approvalHook *hook.ApprovalHook) *ApprovalHandler {
	return &ApprovalHandler{db: db, approvalHook: approvalHook}
}

// Decide handles POST /api/conversations/:id/approvals/:block_id.
// Body: {"decision": "approved" | "rejected"}
// Invariant F: DB write happens before signalling the hook channel.
func (h *ApprovalHandler) Decide(c *gin.Context) {
	convID := c.Param("id")
	blockID := c.Param("block_id")

	var body struct {
		Decision string `json:"decision" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.Decision != "approved" && body.Decision != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "decision must be 'approved' or 'rejected'"})
		return
	}

	now := time.Now()

	// Write DB first (invariant F).
	result := h.db.WithContext(c).
		Model(&repository.Approval{}).
		Where("block_id = ? AND conversation_id = ? AND status = ?", blockID, convID, "pending").
		Updates(map[string]any{"status": body.Decision, "decided_at": now})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record decision"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no pending approval found"})
		return
	}

	// Signal channel second (invariant F).
	h.approvalHook.Signal(blockID, body.Decision == "approved")

	c.Status(http.StatusNoContent)
}
