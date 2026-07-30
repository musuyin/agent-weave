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

func newSkillRouter(t *testing.T) (*gin.Engine, *gorm.DB) {
	t.Helper()
	db := newTestDB(t)
	svc := service.NewSkillService(db)
	h := handler.NewSkillHandler(svc)

	r := gin.New()
	r.GET("/api/skills", h.List)
	r.POST("/api/skills", h.Create)
	r.GET("/api/skills/:id", h.Get)
	r.PUT("/api/skills/:id", h.Update)
	r.DELETE("/api/skills/:id", h.Delete)
	return r, db
}

func TestSkill_CRUD(t *testing.T) {
	r, _ := newSkillRouter(t)

	// Create.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/skills",
		strings.NewReader(`{"name":"greeting","description":"say hi","body":"# Greeting"}`)))
	require.Equal(t, http.StatusCreated, w.Code)

	var created repository.Skill
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "greeting", created.Name)
	assert.False(t, created.IsSystem)
	require.NotEmpty(t, created.ID)

	// List.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var list []repository.Skill
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	require.Len(t, list, 1)

	// Get.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills/"+created.ID, nil))
	require.Equal(t, http.StatusOK, w.Code)

	// Update.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/skills/"+created.ID,
		strings.NewReader(`{"name":"greeting2","description":"","body":"# Greeting v2"}`)))
	require.Equal(t, http.StatusOK, w.Code)
	var updated repository.Skill
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &updated))
	assert.Equal(t, "greeting2", updated.Name)

	// Delete.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/skills/"+created.ID, nil))
	require.Equal(t, http.StatusNoContent, w.Code)

	// Get after delete → 404.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills/"+created.ID, nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSkill_GetMissing(t *testing.T) {
	r, _ := newSkillRouter(t)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/skills/nope", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestSkill_SystemReadOnly(t *testing.T) {
	r, db := newSkillRouter(t)

	// Seed a system skill directly.
	sysID := "sys-skill-1"
	require.NoError(t, db.Create(&repository.Skill{
		ID: sysID, Name: "sys", Body: "x", IsSystem: true,
	}).Error)

	// Update system skill → 400.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPut, "/api/skills/"+sysID,
		strings.NewReader(`{"name":"sys","description":"","body":"y"}`)))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	// Delete system skill → 400.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/skills/"+sysID, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}
