package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
)

type ConversationService struct {
	db *gorm.DB
}

func NewConversationService(db *gorm.DB) *ConversationService {
	return &ConversationService{db: db}
}

// FindByTitle returns the conversation with the given title, or gorm.ErrRecordNotFound.
func (s *ConversationService) FindByTitle(ctx context.Context, title string) (repository.Conversation, error) {
	var conv repository.Conversation
	err := s.db.WithContext(ctx).Where("title = ?", title).First(&conv).Error
	return conv, err
}

// List returns up to 50 conversations ordered newest-first.
func (s *ConversationService) List(ctx context.Context) ([]repository.Conversation, error) {
	var convs []repository.Conversation
	err := s.db.WithContext(ctx).
		Order("created_at DESC").
		Limit(50).
		Find(&convs).Error
	return convs, err
}

// Create persists a new conversation and returns it.
func (s *ConversationService) Create(ctx context.Context, title string) (repository.Conversation, error) {
	if title == "" {
		title = "New conversation"
	}
	now := time.Now().UTC()
	conv := repository.Conversation{
		ID:        uuid.NewString(),
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	return conv, s.db.WithContext(ctx).Create(&conv).Error
}
