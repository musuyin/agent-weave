package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/agent"
)

type StreamHandler struct {
	registry *agent.HubRegistry
}

func NewStreamHandler(registry *agent.HubRegistry) *StreamHandler {
	return &StreamHandler{registry: registry}
}

// Stream opens a Server-Sent Events connection for the given conversation.
// Events are JSON-serialised SSEEvent objects. The stream closes on
// EventQueueDrained or client disconnect.
func (h *StreamHandler) Stream(c *gin.Context) {
	convID := c.Param("id")

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

	// Ensure the hub is removed if the client disconnects before queue_drained.
	h.registry.Delete(convID)
}

// Health is a simple liveness check.
func Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
