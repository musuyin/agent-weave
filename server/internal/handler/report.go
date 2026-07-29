package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/service"
)

type ReportHandler struct {
	svc      *service.ReportService
	msgSvc   *service.MessageService
	agentSvc *agent.Service
	registry *agent.HubRegistry
}

func NewReportHandler(
	svc *service.ReportService,
	msgSvc *service.MessageService,
	agentSvc *agent.Service,
	registry *agent.HubRegistry,
) *ReportHandler {
	return &ReportHandler{svc: svc, msgSvc: msgSvc, agentSvc: agentSvc, registry: registry}
}

func (h *ReportHandler) Run(c *gin.Context) {
	title, prompt, err := h.svc.ReportDef(c.Param("type"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	convID, err := h.svc.GetOrCreateReportConversation(c.Request.Context(), title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if _, err := h.msgSvc.SaveUserMessage(c.Request.Context(), convID, prompt); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	hub := h.registry.GetOrCreate(convID)
	go h.agentSvc.Run(context.Background(), convID, hub)

	c.JSON(http.StatusAccepted, gin.H{"conversation_id": convID})
}
