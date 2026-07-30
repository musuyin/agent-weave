package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/model/dto"
	"github/musuyin/agent-weave/internal/service"
)

type AgentHandler struct {
	svc *service.AgentService
}

func NewAgentHandler(svc *service.AgentService) *AgentHandler {
	return &AgentHandler{svc: svc}
}

func (h *AgentHandler) List(c *gin.Context) {
	agents, err := h.svc.List(c.Request.Context())
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, agents)
}

func (h *AgentHandler) Get(c *gin.Context) {
	agent, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, agent)
}

func (h *AgentHandler) Create(c *gin.Context) {
	var req dto.CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent, err := h.svc.Create(c.Request.Context(), req.Name, req.Description, req.Prompt)
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, agent)
}

func (h *AgentHandler) Update(c *gin.Context) {
	var req dto.UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent, err := h.svc.Update(c.Request.Context(), c.Param("id"), req.Name, req.Description, req.Prompt)
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, agent)
}

func (h *AgentHandler) Delete(c *gin.Context) {
	err := h.svc.Delete(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AgentHandler) ListSkills(c *gin.Context) {
	skills, err := h.svc.ListSkills(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, skills)
}

func (h *AgentHandler) LoadSkill(c *gin.Context) {
	var req dto.LoadSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.LoadSkill(c.Request.Context(), c.Param("id"), req.SkillID); respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusOK)
}

func (h *AgentHandler) UnloadSkill(c *gin.Context) {
	err := h.svc.UnloadSkill(c.Request.Context(), c.Param("id"), c.Param("skillId"))
	if respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AgentHandler) ListConversationAgents(c *gin.Context) {
	agents, err := h.svc.ListConversationAgents(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, agents)
}

func (h *AgentHandler) AddConversationAgent(c *gin.Context) {
	var req dto.AddAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.AddAgentToConversation(c.Request.Context(), c.Param("id"), req.AgentID); respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusOK)
}

func (h *AgentHandler) RemoveConversationAgent(c *gin.Context) {
	err := h.svc.RemoveAgentFromConversation(c.Request.Context(), c.Param("id"), c.Param("agentId"))
	if respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}
