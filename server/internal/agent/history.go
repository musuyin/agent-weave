package agent

import (
	"context"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
)

// loadHistory loads all messages for the conversation ordered (created_at ASC, id ASC)
// and converts them to the Anthropic SDK message format, including tool_use and tool_result blocks.
func (s *Service) loadHistory(ctx context.Context, conversationID string) ([]anthropic.MessageParam, error) {
	var msgs []repository.Message
	if err := s.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("created_at ASC, id ASC").
		Find(&msgs).Error; err != nil {
		return nil, err
	}

	params := make([]anthropic.MessageParam, 0, len(msgs))
	for _, m := range msgs {
		var blocks []anthropic.ContentBlockParamUnion
		for _, cb := range m.Content {
			switch cb.Type {
			case "text":
				blocks = append(blocks, anthropic.NewTextBlock(cb.Text))
			case "tool_use":
				blocks = append(blocks, anthropic.NewToolUseBlock(cb.ID, cb.Input, cb.Name))
			case "tool_result":
				blocks = append(blocks, anthropic.NewToolResultBlock(cb.ToolUseID, cb.Content, cb.IsError))
			}
		}
		if len(blocks) == 0 {
			continue
		}
		switch m.Role {
		case "user":
			params = append(params, anthropic.NewUserMessage(blocks...))
		case "assistant":
			params = append(params, anthropic.NewAssistantMessage(blocks...))
		}
	}
	return params, nil
}

// persistMessage saves a message to DB with a generated UUID and current timestamp.
// agentID is nil for orchestrator messages, or a pointer to the agent's UUID for subagent messages.
func (s *Service) persistMessage(ctx context.Context, conversationID, role string, content repository.ContentBlocks, agentID *string) error {
	msg := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           role,
		AgentID:        agentID,
		Content:        content,
		CreatedAt:      time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Create(&msg).Error
}

// ProvideDB returns the underlying *gorm.DB for use in Phase 1+ task graph queries.
// Callers should use db.Session(&gorm.Session{NewDB: true}) to bypass the identity map.
func (s *Service) ProvideDB() *gorm.DB {
	return s.db
}
