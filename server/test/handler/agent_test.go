package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/handler"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/service"
)

func newAgentRouter(t *testing.T) (*gin.Engine, *gorm.DB, *service.SkillService) {
	t.Helper()
	db := newTestDB(t)
	agentSvc := service.NewAgentService(db)
	skillSvc := service.NewSkillService(db)
	h := handler.NewAgentHandler(agentSvc)

	r := gin.New()
	r.GET("/api/agents", h.List)
	r.POST("/api/agents", h.Create)
	r.GET("/api/agents/:id", h.Get)
	r.PUT("/api/agents/:id", h.Update)
	r.DELETE("/api/agents/:id", h.Delete)
	r.GET("/api/agents/:id/skills", h.ListSkills)
	r.POST("/api/agents/:id/skills", h.LoadSkill)
	r.DELETE("/api/agents/:id/skills/:skillId", h.UnloadSkill)
	r.GET("/api/conversations/:id/agents", h.ListConversationAgents)
	r.POST("/api/conversations/:id/agents", h.AddConversationAgent)
	r.DELETE("/api/conversations/:id/agents/:agentId", h.RemoveConversationAgent)
	return r, db, skillSvc
}

func TestAgent_CRUD(t *testing.T) {
	r, _, _ := newAgentRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/agents",
		strings.NewReader(`{"name":"researcher","description":"web research","prompt":"You research."}`)))
	require.Equal(t, http.StatusCreated, w.Code)
	var created repository.Agent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "researcher", created.Name)
	require.NotEmpty(t, created.ID)

	// List.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var list []repository.Agent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)

	// Update.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/agents/"+created.ID,
		strings.NewReader(`{"name":"researcher2","description":"","prompt":"Updated."}`)))
	require.Equal(t, http.StatusOK, w.Code)

	// Delete.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/agents/"+created.ID, nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/agents/"+created.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestAgent_SystemReadOnly(t *testing.T) {
	r, db, _ := newAgentRouter(t)
	sysID := "sys-agent-1"
	require.NoError(t, db.Create(&repository.Agent{
		ID: sysID, Name: "sys", Prompt: "x", IsSystem: true,
	}).Error)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/agents/"+sysID, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAgent_LoadUnloadSkill(t *testing.T) {
	r, _, skillSvc := newAgentRouter(t)
	ctx := t.Context()

	// Create an agent and a skill.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/agents",
		strings.NewReader(`{"name":"a","prompt":"p"}`)))
	require.Equal(t, http.StatusCreated, w.Code)
	var agent repository.Agent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &agent))

	skill, err := skillSvc.Create(ctx, "s1", "", "body")
	require.NoError(t, err)

	// Load.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/agents/"+agent.ID+"/skills",
		strings.NewReader(`{"skill_id":"`+skill.ID+`"}`)))
	require.Equal(t, http.StatusOK, w.Code)

	// List loaded → 1.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/skills", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var loaded []repository.Skill
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loaded))
	require.Len(t, loaded, 1)
	assert.Equal(t, skill.ID, loaded[0].ID)

	// Unload.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/agents/"+agent.ID+"/skills/"+skill.ID, nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/skills", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &loaded))
	assert.Empty(t, loaded)
}

func TestAgent_ConversationMembership(t *testing.T) {
	r, db, _ := newAgentRouter(t)
	convID := seedConversation(t, db, "chat")

	// Create an agent.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/agents",
		strings.NewReader(`{"name":"a","prompt":"p"}`)))
	require.Equal(t, http.StatusCreated, w.Code)
	var agent repository.Agent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &agent))

	// Add to conversation.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/conversations/"+convID+"/agents",
		strings.NewReader(`{"agent_id":"`+agent.ID+`"}`)))
	require.Equal(t, http.StatusOK, w.Code)

	// List → 1.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations/"+convID+"/agents", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var members []repository.Agent
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &members))
	require.Len(t, members, 1)
	assert.Equal(t, agent.ID, members[0].ID)

	// Remove.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/conversations/"+convID+"/agents/"+agent.ID, nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations/"+convID+"/agents", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &members))
	assert.Empty(t, members)
}

func TestAgent_Seeding(t *testing.T) {
	db := newTestDB(t)
	ctx := t.Context()
	require.NoError(t, service.NewSkillService(db).Init(ctx))
	agentSvc := service.NewAgentService(db)
	require.NoError(t, agentSvc.Init(ctx))
	// Idempotent: second Init must not error or duplicate.
	require.NoError(t, agentSvc.Init(ctx))

	agents, err := agentSvc.List(ctx)
	require.NoError(t, err)
	var found repository.Agent
	for _, a := range agents {
		if a.Name == "code-reviewer" {
			found = a
		}
	}
	require.NotEmpty(t, found.ID, "code-reviewer should be seeded")
	assert.True(t, found.IsSystem)

	skills, err := agentSvc.ListSkills(ctx, found.ID)
	require.NoError(t, err)
	require.Len(t, skills, 1)
	assert.Equal(t, "code-review-guidelines", skills[0].Name)
}
