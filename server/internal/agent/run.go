package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/seeding"
	"github/musuyin/agent-weave/internal/tool"
	_ "github/musuyin/agent-weave/internal/tool/builtin"
)

func (s *Service) run(ctx context.Context, conversationID string, hub *Hub) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Inject conversationID so dispatch_to_agent tool handler can retrieve it.
	ctx = withConversationID(ctx, conversationID)

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
				{Text: s.buildSystemPrompt(ctx, conversationID)},
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
			if err := s.persistMessage(ctx, conversationID, "assistant", blocks, nil); err != nil {
				return fmt.Errorf("persist: %w", err)
			}

			// Fan-in: if subagents are still running, wait for them then loop back.
			if s.dispatchReg.Pending(conversationID) > 0 {
				s.dispatchReg.Wait(conversationID)
				results := s.dispatchReg.Drain(conversationID)
				fanInBlocks := subAgentResultsToBlocks(results)
				if err := s.persistMessage(ctx, conversationID, "user", fanInBlocks, nil); err != nil {
					return fmt.Errorf("persist fan-in: %w", err)
				}
				continue // loop back so orchestrator can summarize results
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
				_ = s.persistMessage(ctx, conversationID, "assistant", blocks, nil)
			}
			hub.Push(SSEEvent{Type: EventRoundDone})
			hub.Push(SSEEvent{Type: EventQueueDrained})
			return nil
		}
	}
}

// buildSystemPrompt assembles the layered system prompt.
// Layer 1: orchestrator instructions (from internal/seeding/orchestrator.md)
// Layer 2: available subagents (injected when agents are in this conversation)
// Layer 3: tool names + descriptions (builtin + MCP, stays current with registered tools)
func (s *Service) buildSystemPrompt(ctx context.Context, conversationID string) string {
	var sb strings.Builder
	sb.WriteString(seeding.Orchestrator)

	if s.agentSvc != nil {
		agents, err := s.agentSvc.ListConversationAgents(ctx, conversationID)
		if err == nil && len(agents) > 0 {
			sb.WriteString("\n\n## Available Subagents\n\nThe following subagents are in this conversation. Use dispatch_to_agent to delegate tasks to them.\n")
			for _, a := range agents {
				sb.WriteString("\n- **")
				sb.WriteString(a.Name)
				sb.WriteString("** (ID: `")
				sb.WriteString(a.ID)
				sb.WriteString("`): ")
				sb.WriteString(a.Description)
			}
		}
	}

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

// subAgentResultsToBlocks converts subagent results into a synthetic user message
// that feeds back into the orchestrator loop for summarization.
func subAgentResultsToBlocks(results []SubAgentResult) repository.ContentBlocks {
	var sb strings.Builder
	sb.WriteString("Subagent results:\n")
	for _, r := range results {
		sb.WriteString("\n### ")
		sb.WriteString(r.AgentName)
		sb.WriteString(" (thread ")
		sb.WriteString(r.ThreadID)
		sb.WriteString(")\n")
		if r.Err != nil {
			sb.WriteString("Error: ")
			sb.WriteString(r.Err.Error())
		} else {
			sb.WriteString(r.Output)
		}
	}
	return repository.ContentBlocks{
		{Type: "text", Text: sb.String()},
	}
}
