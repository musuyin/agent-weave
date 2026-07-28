package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/prompts"
	"github/musuyin/agent-weave/internal/tool"
	_ "github/musuyin/agent-weave/internal/tool/builtin"
)

// Service is the agent execution service. One instance per application.
type Service struct {
	db       *gorm.DB // kept for Phase 1+ db.Session(&gorm.Session{NewDB:true}) task graph queries
	aiClient *anthropic.Client
	registry *HubRegistry
	chain    *hook.Chain
	cfg      *config.Config
	log      *slog.Logger
}

// ProvideAgentService constructs the agent Service. Wire provider.
func ProvideAgentService(db *gorm.DB, cfg *config.Config, registry *HubRegistry, chain *hook.Chain, log *slog.Logger) *Service {
	opts := []option.RequestOption{option.WithAPIKey(cfg.LLMModel.Anthropic.APIKey)}
	if cfg.LLMModel.Anthropic.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.LLMModel.Anthropic.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	return &Service{
		db:       db,
		aiClient: &client,
		registry: registry,
		chain:    chain,
		cfg:      cfg,
		log:      log,
	}
}

// Run executes one full agent turn for the given conversation.
// Called in a goroutine from the POST /conversations/:id/messages handler.
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

	for {
		history, err := s.loadHistory(ctx, conversationID)
		if err != nil {
			return fmt.Errorf("load history: %w", err)
		}

		stream := s.aiClient.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
			Model:     anthropic.Model(s.cfg.LLMModel.Anthropic.Model),
			MaxTokens: 8096,
			System: []anthropic.TextBlockParam{
				{Text: s.buildSystemPrompt()},
			},
			Messages: history,
			Tools:    buildToolParams(),
		})

		blockIDs := map[int64]string{}
		accMsg := anthropic.Message{}

		for stream.Next() {
			event := stream.Current()
			_ = accMsg.Accumulate(event)

			switch ev := event.AsAny().(type) {
			case anthropic.ContentBlockStartEvent:
				blockID := uuid.NewString()
				blockIDs[ev.Index] = blockID
				blockType := "text"
				if ev.ContentBlock.Type == "tool_use" {
					blockType = "tool_use"
				}
				hub.Push(SSEEvent{Type: EventBlockStart, Data: BlockStartData{
					BlockID:   blockID,
					BlockType: blockType,
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

		switch accMsg.StopReason {
		case "end_turn":
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

		case "tool_use":
			if err := s.dispatchTools(ctx, conversationID, hub, accMsg); err != nil {
				return err
			}
			// Loop back to call the API again with updated history.

		default:
			// Unexpected stop reason — persist what we have and end.
			s.log.Warn("unexpected stop reason", "reason", accMsg.StopReason)
			var blocks repository.ContentBlocks
			for _, cb := range accMsg.Content {
				if cb.Type == "text" {
					blocks = append(blocks, repository.ContentBlock{Type: "text", Text: cb.Text})
				}
			}
			if len(blocks) > 0 {
				_ = s.persistMessage(ctx, conversationID, "assistant", blocks)
			}
			hub.Push(SSEEvent{Type: EventRoundDone})
			hub.Push(SSEEvent{Type: EventQueueDrained})
			return nil
		}
	}
}

// dispatchTools handles all tool_use blocks in the accumulated message.
// Order per invariant A: FirePre → write history → execute → write result → FirePost.
func (s *Service) dispatchTools(ctx context.Context, conversationID string, hub *Hub, msg anthropic.Message) error {
	// Collect all tool_use blocks.
	var assistantBlocks repository.ContentBlocks
	type pendingTool struct {
		id    string
		name  string
		input json.RawMessage
	}
	var pending []pendingTool

	for _, cb := range msg.Content {
		switch cb.Type {
		case "text":
			assistantBlocks = append(assistantBlocks, repository.ContentBlock{
				Type: "text",
				Text: cb.Text,
			})
		case "tool_use":
			tu := cb.AsToolUse()
			assistantBlocks = append(assistantBlocks, repository.ContentBlock{
				Type:  "tool_use",
				ID:    tu.ID,
				Name:  tu.Name,
				Input: tu.Input,
			})
			pending = append(pending, pendingTool{id: tu.ID, name: tu.Name, input: tu.Input})
		}
	}

	// For each tool: FirePre, persist assistant message, execute, persist result, FirePost.
	// The assistant message (with all tool_use blocks) is written once before the first execution.
	assistantPersisted := false

	var resultBlocks repository.ContentBlocks

	for _, pt := range pending {
		params := hook.ToolCallParams{Name: pt.name, Params: pt.input}

		// Invariant A: FirePre BEFORE writing history.
		preErr := s.chain.FirePre(ctx, &params)

		// Write assistant message (once, before first tool execution).
		if !assistantPersisted {
			if err := s.persistMessage(ctx, conversationID, "assistant", assistantBlocks); err != nil {
				return fmt.Errorf("persist assistant: %w", err)
			}
			assistantPersisted = true
		}

		var result string
		var toolErr error

		if preErr != nil {
			result = "tool call denied: " + preErr.Error()
		} else {
			def, ok := tool.Get(params.Name)
			if !ok {
				result = "unknown tool: " + params.Name
			} else {
				result, toolErr = def.Handler(ctx, params.Params)
				if toolErr != nil {
					result = "tool error: " + toolErr.Error()
				}
			}
		}

		resultBlocks = append(resultBlocks, repository.ContentBlock{
			Type:      "tool_result",
			ToolUseID: pt.id,
			Content:   result,
			IsError:   preErr != nil || toolErr != nil,
		})

		s.chain.FirePost(ctx, params, result, toolErr)
	}

	// Persist all tool results as a single user message (Anthropic API requirement).
	if len(resultBlocks) > 0 {
		if err := s.persistMessage(ctx, conversationID, "user", resultBlocks); err != nil {
			return fmt.Errorf("persist tool results: %w", err)
		}
	}

	return nil
}

// buildSystemPrompt assembles the layered system prompt.
// Layer 1: orchestrator instructions (from prompts/orchestrator.md)
// Layer 3: tool names + descriptions
// Layers 2, 4-6: deferred to later phases
func (s *Service) buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(prompts.Orchestrator)

	defs := tool.All()
	if len(defs) > 0 {
		sb.WriteString("\n\n## Tool Reference\n")
		for _, d := range defs {
			sb.WriteString("\n- **")
			sb.WriteString(d.Name)
			sb.WriteString("**: ")
			sb.WriteString(d.Description)
		}
	}

	return sb.String()
}

// buildToolParams converts the registered tool definitions to the Anthropic SDK format.
func buildToolParams() []anthropic.ToolUnionParam {
	defs := tool.All()
	params := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		schemaBytes, err := json.Marshal(d.InputSchema)
		if err != nil {
			continue
		}
		var props map[string]any
		_ = json.Unmarshal(schemaBytes, &props)

		tp := anthropic.ToolParam{
			Name:        d.Name,
			Description: anthropic.String(d.Description),
			InputSchema: anthropic.ToolInputSchemaParam{
				Properties: props,
			},
		}
		params = append(params, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return params
}

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
