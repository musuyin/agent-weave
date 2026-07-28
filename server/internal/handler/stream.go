package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/service"
)

type StreamHandler struct {
	registry *agent.HubRegistry
	msgSvc   *service.MessageService
}

func NewStreamHandler(registry *agent.HubRegistry, msgSvc *service.MessageService) *StreamHandler {
	return &StreamHandler{registry: registry, msgSvc: msgSvc}
}

// Stream opens a Server-Sent Events connection for the given conversation.
// Events are JSON-serialised SSEEvent objects. The stream closes on
// EventQueueDrained or client disconnect.
func (h *StreamHandler) Stream(c *gin.Context) {
	convID := c.Param("id")

	ok, err := h.msgSvc.ConversationExists(c.Request.Context(), convID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return
	}

	hub := h.registry.GetOrCreate(convID)

	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	c.Header("Content-Type", "text/event-stream")

	c.Stream(func(w io.Writer) bool {
		select {
		case event, ok := <-hub.Chan():
			if !ok {
				return false
			}
			data, _ := json.Marshal(event)
			c.SSEvent(string(event.Type), string(data))
			if event.Type == agent.EventQueueDrained {
				h.registry.Delete(convID)
				return false
			}
			return true
		case <-c.Request.Context().Done():
			return false
		}
	})

	h.registry.Delete(convID)
}

// Health is a simple liveness check.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
