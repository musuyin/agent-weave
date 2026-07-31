package service

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
)

// terminalStatuses lists thread states that must not be overwritten by CancelAllThreads.
var terminalStatuses = []string{
	string(repository.ThreadStatusDone),
	string(repository.ThreadStatusCancelled),
	string(repository.ThreadStatusError),
}

type ThreadService struct {
	db *gorm.DB
}

func NewThreadService(db *gorm.DB) *ThreadService {
	return &ThreadService{db: db}
}

// CancelAllThreads marks all non-terminal threads for convID as cancelled.
// Uses one short db.Transaction per thread (invariant C).
func (s *ThreadService) CancelAllThreads(ctx context.Context, convID string) error {
	var threads []repository.Thread
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ? AND status NOT IN ?", convID, terminalStatuses).
		Find(&threads).Error; err != nil {
		return fmt.Errorf("cancel threads: list: %w", err)
	}

	for _, t := range threads {
		id := t.ID
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return tx.Model(&repository.Thread{}).
				Where("id = ? AND status NOT IN ?", id, terminalStatuses).
				Update("status", repository.ThreadStatusCancelled).Error
		}); err != nil {
			return fmt.Errorf("cancel thread %s: %w", id, err)
		}
	}
	return nil
}
