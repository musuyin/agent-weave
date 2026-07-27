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

	"github/musuyin/agent-weave/internal/handler"
	"github/musuyin/agent-weave/internal/model"
	"github/musuyin/agent-weave/internal/service"
)

func newConvRouter(t *testing.T) *gin.Engine {
	t.Helper()
	db := newTestDB(t)
	svc := service.NewConversationService(db)
	h := handler.NewConversationHandler(svc)

	r := gin.New()
	r.GET("/api/conversations", h.List)
	r.POST("/api/conversations", h.Create)
	return r
}

func TestConversation_CreateAndList(t *testing.T) {
	r := newConvRouter(t)

	// Create with title.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/conversations",
		strings.NewReader(`{"title":"my conv"}`)))
	assert.Equal(t, http.StatusCreated, w.Code)

	var created model.Conversation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "my conv", created.Title)
	assert.NotEmpty(t, created.ID)

	// List should return it.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/conversations", nil))
	assert.Equal(t, http.StatusOK, w2.Code)

	var list []model.Conversation
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &list))
	require.Len(t, list, 1)
	assert.Equal(t, created.ID, list[0].ID)
}

func TestConversation_CreateDefaultTitle(t *testing.T) {
	r := newConvRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/conversations",
		strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusCreated, w.Code)

	var created model.Conversation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
	assert.Equal(t, "New conversation", created.Title)
}

func TestConversation_CreateEmptyBody(t *testing.T) {
	r := newConvRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/conversations", nil))
	assert.Equal(t, http.StatusCreated, w.Code)
}

func TestConversation_ListEmpty(t *testing.T) {
	r := newConvRouter(t)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	var list []model.Conversation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
	assert.Empty(t, list)
}
