package handler

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/service"
)

type MessageHandler struct {
	svc      *service.MessageService
	agentSvc *agent.Service
	registry *agent.HubRegistry
}

func NewMessageHandler(svc *service.MessageService, agentSvc *agent.Service, registry *agent.HubRegistry) *MessageHandler {
	return &MessageHandler{svc: svc, agentSvc: agentSvc, registry: registry}
}

type sendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

func (h *MessageHandler) List(c *gin.Context) {
	convID := c.Param("id")
	if !h.requireConversationOwner(c, convID) {
		return
	}

	p := service.ListParams{Limit: 50}
	afterCreatedAt := c.Query("after_created_at")
	afterID := c.Query("after_id")
	if afterCreatedAt != "" && afterID != "" {
		t, err := time.Parse(time.RFC3339Nano, afterCreatedAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after_created_at"})
			return
		}
		p.AfterCreatedAt = &t
		p.AfterID = afterID
	}

	msgs, err := h.svc.List(c.Request.Context(), convID, p)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCursor) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, msgs)
}

func (h *MessageHandler) Send(c *gin.Context) {
	convID := c.Param("id")
	if !h.requireConversationOwner(c, convID) {
		return
	}

	var req sendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	msg, err := h.svc.SaveUserMessage(c.Request.Context(), convID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hub := h.registry.GetOrCreate(convID)
	go h.agentSvc.Run(context.Background(), convID, hub)

	c.JSON(http.StatusAccepted, msg)
}

func (h *MessageHandler) requireConversationOwner(c *gin.Context, convID string) bool {
	ok, err := h.svc.ConversationBelongsToUser(c.Request.Context(), convID, stubUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return false
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
		return false
	}
	return true
}
