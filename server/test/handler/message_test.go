package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github/musuyin/agent-weave/internal/handler"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/service"
)

func TestMessage_ListEmpty(t *testing.T) {
	db := newTestDB(t)
	registry := newTestRegistry(t)
	convSvc := service.NewConversationService(db)
	msgSvc := service.NewMessageService(db)
	convH := handler.NewConversationHandler(convSvc)
	msgH := handler.NewMessageHandler(msgSvc, nil, registry)

	r := gin.New()
	r.POST("/api/conversations", convH.Create)
	r.GET("/api/conversations/:id/messages", msgH.List)

	// Create conversation first.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/conversations",
		strings.NewReader(`{"title":"t"}`)))
	require.Equal(t, http.StatusCreated, w.Code)
	var conv repository.Conversation
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &conv))

	// List messages — should be empty.
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/api/conversations/"+conv.ID+"/messages", nil))
	assert.Equal(t, http.StatusOK, w2.Code)

	var msgs []repository.Message
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &msgs))
	assert.Empty(t, msgs)
}

func TestMessage_ListOrdering(t *testing.T) {
	db := newTestDB(t)
	convID := seedConversation(t, db, "order-test")
	now := time.Now().UTC()

	// Seed 3 messages with explicit well-separated timestamps.
	texts := []string{"first", "second", "third"}
	for i, text := range texts {
		msg := repository.Message{
			ID:             "msg-order-" + text,
			ConversationID: convID,
			Role:           "user",
			Content:        repository.ContentBlocks{{Type: "text", Text: text}},
			CreatedAt:      now.Add(time.Duration(i) * time.Second),
		}
		require.NoError(t, db.Create(&msg).Error)
	}

	registry := newTestRegistry(t)
	msgSvc := service.NewMessageService(db)
	msgH := handler.NewMessageHandler(msgSvc, nil, registry)
	r := gin.New()
	r.GET("/api/conversations/:id/messages", msgH.List)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations/"+convID+"/messages", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var msgs []repository.Message
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &msgs))
	require.Len(t, msgs, 3)
	assert.Equal(t, "first", msgs[0].Content[0].Text)
	assert.Equal(t, "second", msgs[1].Content[0].Text)
	assert.Equal(t, "third", msgs[2].Content[0].Text)
}

func TestMessage_ListKeysetCursor(t *testing.T) {
	db := newTestDB(t)
	convID := seedConversation(t, db, "cursor-test")
	now := time.Now().UTC()

	// Seed 5 messages with explicit, well-separated timestamps.
	seeded := make([]repository.Message, 5)
	for i := range seeded {
		m := seedMessage(t, db, convID, "user", strings.Repeat("x", i+1))
		// Override created_at so ordering is deterministic.
		db.Model(&m).Update("created_at", now.Add(time.Duration(i)*time.Second))
		m.CreatedAt = now.Add(time.Duration(i) * time.Second)
		seeded[i] = m
	}

	registry := newTestRegistry(t)
	msgSvc := service.NewMessageService(db)
	msgH := handler.NewMessageHandler(msgSvc, nil, registry)
	r := gin.New()
	r.GET("/api/conversations/:id/messages", msgH.List)

	// Use seeded[1] as cursor — should receive seeded[2], [3], [4].
	cursor := seeded[1]
	url := "/api/conversations/" + convID + "/messages?after_created_at=" +
		cursor.CreatedAt.UTC().Format(time.RFC3339Nano) + "&after_id=" + cursor.ID

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	require.Equal(t, http.StatusOK, w.Code)

	var got []repository.Message
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Len(t, got, 3)
}

func TestMessage_ListCursorInvalidAfterID(t *testing.T) {
	db := newTestDB(t)
	convID := seedConversation(t, db, "sec-test")
	otherConvID := seedConversation(t, db, "other")
	otherMsg := seedMessage(t, db, otherConvID, "user", "from other conv")

	registry := newTestRegistry(t)
	msgSvc := service.NewMessageService(db)
	msgH := handler.NewMessageHandler(msgSvc, nil, registry)
	r := gin.New()
	r.GET("/api/conversations/:id/messages", msgH.List)

	// Use a message ID from a different conversation — must be rejected.
	url := "/api/conversations/" + convID + "/messages?after_created_at=" +
		otherMsg.CreatedAt.UTC().Format(time.RFC3339Nano) + "&after_id=" + otherMsg.ID

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestMessage_ListConversationNotFound(t *testing.T) {
	db := newTestDB(t)
	registry := newTestRegistry(t)
	msgSvc := service.NewMessageService(db)
	msgH := handler.NewMessageHandler(msgSvc, nil, registry)
	r := gin.New()
	r.GET("/api/conversations/:id/messages", msgH.List)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/conversations/no-such-id/messages", nil))
	assert.Equal(t, http.StatusNotFound, w.Code)
}
