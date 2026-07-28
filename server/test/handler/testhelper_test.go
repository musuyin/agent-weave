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
	"github/musuyin/agent-weave/internal/model/repository"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestDB opens an in-memory SQLite DB isolated per test and auto-migrates the schema.
// _loc=UTC is required so go-sqlite3 parses stored time strings back into time.Time.
func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared&_loc=UTC"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&repository.Conversation{}, &repository.Message{}, &repository.Thread{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func newTestRegistry(t *testing.T) *agent.HubRegistry {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	return agent.NewHubRegistry(log)
}

func seedConversation(t *testing.T, db *gorm.DB, title string) string {
	t.Helper()
	conv := repository.Conversation{
		ID:        uuid.NewString(),
		Title:     title,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	if err := db.Create(&conv).Error; err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	return conv.ID
}

func seedMessage(t *testing.T, db *gorm.DB, convID, role, text string) repository.Message {
	t.Helper()
	msg := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           role,
		Content:        repository.ContentBlocks{{Type: "text", Text: text}},
		CreatedAt:      time.Now().UTC(),
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("seed message: %v", err)
	}
	return msg
}
