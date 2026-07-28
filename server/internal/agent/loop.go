package agent

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/model/repository"
)

const systemPromptLayer1 = `You are an AI assistant and orchestrator. You help the user with their tasks through thoughtful conversation.

Be concise, accurate, and helpful. When you need more information, ask for it. When you have enough context, act.`

// Service is the agent execution service. One instance per application.
type Service struct {
	db       *gorm.DB // kept for Phase 1+ db.Session(&gorm.Session{NewDB:true}) task graph queries
	aiClient *anthropic.Client
	registry *HubRegistry
	cfg      *config.Config
	log      *slog.Logger
}

// ProvideAgentService constructs the agent Service. Wire provider.
func ProvideAgentService(db *gorm.DB, cfg *config.Config, registry *HubRegistry, log *slog.Logger) *Service {
	opts := []option.RequestOption{option.WithAPIKey(cfg.LLMModel.Anthropic.APIKey)}
	if cfg.LLMModel.Anthropic.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.LLMModel.Anthropic.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	return &Service{
		db:       db,
		aiClient: &client,
		registry: registry,
		cfg:      cfg,
		log:      log,
	}
}

// Run executes one full agent turn for the given conversation.
// It is called in a goroutine from the POST /conversations/:id/messages handler.
//
// Phase 0 loop — text only, no tools:
//  1. Check context cancellation
//  2. Load message history from DB
//  3. Call Anthropic API (streaming)
//  4. Push block_start / block_delta / block_stop SSE events per token
//  5. Persist completed assistant message to DB
//  6. Push round_done then queue_drained
func (s *Service) Run(ctx context.Context, conversationID string, hub *Hub) {
	if err := s.run(ctx, conversationID, hub); err != nil {
		s.log.Error("agent run error", "conv_id", conversationID, "error", err)
		hub.Push(SSEEvent{Type: EventRoundDone})
		hub.Push(SSEEvent{Type: EventQueueDrained})
	}
}

func (s *Service) run(ctx context.Context, conversationID string, hub *Hub) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	hub.Push(SSEEvent{Type: EventAgentStart, Data: map[string]string{"conversation_id": conversationID}})

	history, err := s.loadHistory(ctx, conversationID)
	if err != nil {
		return fmt.Errorf("load history: %w", err)
	}

	stream := s.aiClient.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.cfg.LLMModel.Anthropic.Model),
		MaxTokens: 8096,
		System: []anthropic.TextBlockParam{
			{Text: systemPromptLayer1},
		},
		Messages: history,
	})

	// blockIDs maps the Anthropic content block index → our generated block_id.
	blockIDs := map[int64]string{}
	accMsg := anthropic.Message{}

	for stream.Next() {
		event := stream.Current()
		_ = accMsg.Accumulate(event)

		switch ev := event.AsAny().(type) {
		case anthropic.ContentBlockStartEvent:
			blockID := uuid.NewString()
			blockIDs[ev.Index] = blockID
			hub.Push(SSEEvent{Type: EventBlockStart, Data: BlockStartData{
				BlockID:   blockID,
				BlockType: "text",
				Index:     ev.Index,
			}})

		case anthropic.ContentBlockDeltaEvent:
			if delta, ok := ev.Delta.AsAny().(anthropic.TextDelta); ok && delta.Text != "" {
				hub.Push(SSEEvent{Type: EventBlockDelta, Data: BlockDeltaData{
					BlockID: blockIDs[ev.Index],
					Text:    delta.Text,
					Index:   ev.Index,
				}})
			}

		case anthropic.ContentBlockStopEvent:
			hub.Push(SSEEvent{Type: EventBlockStop, Data: BlockStopData{
				BlockID: blockIDs[ev.Index],
				Index:   ev.Index,
			}})
		}
	}

	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream: %w", err)
	}

	// Persist the completed assistant message.
	// This is the "write message history" step — it comes AFTER all hooks fire in Phase 1+
	// (invariant A: PRE_TOOL_USE hook → write history → execute tool).
	var blocks repository.ContentBlocks
	for _, cb := range accMsg.Content {
		if cb.Type == "text" {
			blocks = append(blocks, repository.ContentBlock{Type: "text", Text: cb.Text})
		}
	}
	if err := s.persistMessage(ctx, conversationID, "assistant", blocks); err != nil {
		return fmt.Errorf("persist: %w", err)
	}

	hub.Push(SSEEvent{Type: EventRoundDone})
	hub.Push(SSEEvent{Type: EventQueueDrained})
	return nil
}

// loadHistory loads all messages for the conversation ordered (created_at ASC, id ASC)
// and converts them to the Anthropic SDK message format.
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
			if cb.Type == "text" {
				blocks = append(blocks, anthropic.NewTextBlock(cb.Text))
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
func (s *Service) persistMessage(ctx context.Context, conversationID, role string, content repository.ContentBlocks) error {
	msg := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		CreatedAt:      time.Now().UTC(),
	}
	return s.db.WithContext(ctx).Create(&msg).Error
}
