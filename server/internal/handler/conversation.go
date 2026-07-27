package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/service"
)

type ConversationHandler struct {
	svc *service.ConversationService
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

type createConversationRequest struct {
	Title string `json:"title"`
}

func (h *ConversationHandler) List(c *gin.Context) {
	convs, err := h.svc.List(c.Request.Context(), stubUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, convs)
}

func (h *ConversationHandler) Create(c *gin.Context) {
	var req createConversationRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	conv, err := h.svc.Create(c.Request.Context(), stubUserID, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, conv)
}
