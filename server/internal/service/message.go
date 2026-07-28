package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
)

var ErrConversationNotFound = errors.New("conversation not found")
var ErrInvalidCursor = errors.New("invalid cursor: after_id not found in conversation")

type MessageService struct {
	db *gorm.DB
}

func NewMessageService(db *gorm.DB) *MessageService {
	return &MessageService{db: db}
}

// ConversationExists returns true if the conversation exists.
func (s *MessageService) ConversationExists(ctx context.Context, convID string) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&repository.Conversation{}).
		Where("id = ?", convID).
		Count(&count).Error
	return count > 0, err
}

// ListParams holds keyset cursor parameters for listing messages.
type ListParams struct {
	AfterCreatedAt *time.Time
	AfterID        string
	Limit          int
}

// List returns messages for a conversation using keyset pagination.
// It validates that AfterID belongs to the conversation before applying the cursor.
func (s *MessageService) List(ctx context.Context, convID string, p ListParams) ([]repository.Message, error) {
	limit := p.Limit
	if limit <= 0 {
		limit = 50
	}

	query := s.db.WithContext(ctx).
		Where("conversation_id = ?", convID).
		Order("created_at ASC, id ASC").
		Limit(limit)

	if p.AfterCreatedAt != nil && p.AfterID != "" {
		// Validate that AfterID belongs to this conversation (security invariant C).
		var count int64
		if err := s.db.WithContext(ctx).Model(&repository.Message{}).
			Where("id = ? AND conversation_id = ?", p.AfterID, convID).
			Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 0 {
			return nil, ErrInvalidCursor
		}
		query = query.Where(
			"(created_at > ? OR (created_at = ? AND id > ?))",
			p.AfterCreatedAt, p.AfterCreatedAt, p.AfterID,
		)
	}

	var msgs []repository.Message
	return msgs, query.Find(&msgs).Error
}

// SaveUserMessage persists a user message and returns it.
func (s *MessageService) SaveUserMessage(ctx context.Context, convID, text string) (repository.Message, error) {
	msg := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: convID,
		Role:           "user",
		Content:        repository.ContentBlocks{{Type: "text", Text: text}},
		CreatedAt:      time.Now().UTC(),
	}
	return msg, s.db.WithContext(ctx).Create(&msg).Error
}
