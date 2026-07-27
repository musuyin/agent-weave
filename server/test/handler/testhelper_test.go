package handler_test

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/model"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestDB opens an in-memory SQLite DB isolated per test and auto-migrates the schema.
// The DSN uses a unique name per test so parallel tests don't share state.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Unique named in-memory DB per test; cache=shared keeps the connection alive.
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared&_loc=UTC"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Conversation{}, &model.Message{}, &model.Thread{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func newTestRegistry(t *testing.T) *agent.HubRegistry {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return agent.NewHubRegistry(log)
}

// seedConversation inserts a conversation belonging to StubUserID and returns its ID.
func seedConversation(t *testing.T, db *gorm.DB, title string) string {
	t.Helper()
	conv := model.Conversation{
		ID:        uuid.NewString(),
		UserID:    testUserID,
		Title:     title,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return conv.ID
}

// seedMessage inserts a message and returns it.
func seedMessage(t *testing.T, db *gorm.DB, convID, role, text string) model.Message {
	t.Helper()
	msg := model.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           role,
		Content:        model.ContentBlocks{{Type: "text", Text: text}},
		CreatedAt:      time.Now().UTC(),
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return msg
}

// testUserID matches handler.StubUserID so test fixtures are owned by the stub user.
const testUserID = "dev"
