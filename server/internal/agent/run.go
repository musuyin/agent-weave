package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/prompts"
	"github/musuyin/agent-weave/internal/tool"
	_ "github/musuyin/agent-weave/internal/tool/builtin"
)

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
			Tools:    s.buildToolParams(),
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
			if err := s.dispatchTools(ctx, conversationID, accMsg); err != nil {
				return err
			}
			// Loop back to call the API again with updated history.

		default:
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

// buildSystemPrompt assembles the layered system prompt.
// Layer 1: orchestrator instructions (from internal/prompts/orchestrator.md)
// Layer 3: tool names + descriptions (builtin + MCP, stays current with registered tools)
// Layers 2, 4-6: deferred to later phases
func (s *Service) buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(prompts.Orchestrator)

	allDefs := append(tool.All(), s.mcpRouter.AllTools()...)
	if len(allDefs) > 0 {
		sb.WriteString("\n\n## Tool Reference\n")
		for _, d := range allDefs {
			sb.WriteString("\n- **")
			sb.WriteString(d.Name)
			sb.WriteString("**: ")
			sb.WriteString(d.Description)
		}
	}

	return sb.String()
}
