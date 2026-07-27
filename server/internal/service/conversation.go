package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model"
)

type ConversationService struct {
	db *gorm.DB
}

func NewConversationService(db *gorm.DB) *ConversationService {
	return &ConversationService{db: db}
}

// List returns up to 50 conversations for the given user, ordered newest-first.
func (s *ConversationService) List(ctx context.Context, userID string) ([]model.Conversation, error) {
	var convs []model.Conversation
	err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Limit(50).
		Find(&convs).Error
	return convs, err
}

// Create persists a new conversation and returns it.
func (s *ConversationService) Create(ctx context.Context, userID, title string) (model.Conversation, error) {
	if title == "" {
		title = "New conversation"
	}
	conv := model.Conversation{
		ID:        uuid.NewString(),
		UserID:    userID,
		Title:     title,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	return conv, s.db.WithContext(ctx).Create(&conv).Error
}
