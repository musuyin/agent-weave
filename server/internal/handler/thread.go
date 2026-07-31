package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/service"
)

type ThreadHandler struct {
	svc *service.ThreadService
}

func NewThreadHandler(svc *service.ThreadService) *ThreadHandler {
	return &ThreadHandler{svc: svc}
}

// CancelAll handles DELETE /api/conversations/:id/threads.
// Marks all non-terminal threads for the conversation as cancelled.
func (h *ThreadHandler) CancelAll(c *gin.Context) {
	convID := c.Param("id")
	if convID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "conversation id required"})
		return
	}
	if err := h.svc.CancelAllThreads(c.Request.Context(), convID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}
