package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github/musuyin/agent-weave/internal/model/dto"
	"github/musuyin/agent-weave/internal/service"
)

// respondServiceError maps service sentinel errors to HTTP status codes.
// Returns true if it handled the error.
func respondServiceError(c *gin.Context, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, service.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSystemReadOnly):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
	return true
}

type SkillHandler struct {
	svc *service.SkillService
}

func NewSkillHandler(svc *service.SkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

func (h *SkillHandler) List(c *gin.Context) {
	skills, err := h.svc.List(c.Request.Context())
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, skills)
}

func (h *SkillHandler) Get(c *gin.Context) {
	skill, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, skill)
}

func (h *SkillHandler) Create(c *gin.Context) {
	var req dto.CreateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill, err := h.svc.Create(c.Request.Context(), req.Name, req.Description, req.Body)
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, skill)
}

func (h *SkillHandler) Update(c *gin.Context) {
	var req dto.UpdateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	skill, err := h.svc.Update(c.Request.Context(), c.Param("id"), req.Name, req.Description, req.Body)
	if respondServiceError(c, err) {
		return
	}
	c.JSON(http.StatusOK, skill)
}

func (h *SkillHandler) Delete(c *gin.Context) {
	err := h.svc.Delete(c.Request.Context(), c.Param("id"))
	if respondServiceError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}
