package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/google/uuid"

	"github/musuyin/agent-weave/internal/model/repository"
)

const (
	CompactThreshold = 40
	PinnedHead       = 4
	LiveTail         = 10
)

// maybeCompact checks whether msgs exceeds CompactThreshold and, if so, compacts
// the middle slice into a single summary message persisted to the DB.
// msgs must already be filtered to compacted=false, ordered created_at ASC.
// Returns the (possibly replaced) slice to use as history.
// Errors are non-fatal: the caller logs and falls back to the original slice.
func (s *Service) maybeCompact(ctx context.Context, conversationID string, msgs []repository.Message) ([]repository.Message, error) {
	if len(msgs) < CompactThreshold {
		return msgs, nil
	}

	head := msgs[:PinnedHead]
	tail := msgs[len(msgs)-LiveTail:]
	middle := msgs[PinnedHead : len(msgs)-LiveTail]

	summary, err := s.compactMessages(ctx, conversationID, middle, tail)
	if err != nil {
		return msgs, fmt.Errorf("compact: %w", err)
	}

	result := make([]repository.Message, 0, PinnedHead+1+LiveTail)
	result = append(result, head...)
	result = append(result, summary)
	result = append(result, tail...)
	return result, nil
}

// compactMessages calls the LLM to summarise middle, persists the summary,
// and marks all middle messages as compacted=1.
func (s *Service) compactMessages(ctx context.Context, conversationID string, middle, tail []repository.Message) (repository.Message, error) {
	prompt := buildCompactionPrompt(middle, tail)

	resp, err := s.aiClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(s.cfg.LLMModel.Anthropic.Model),
		MaxTokens: 4096,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		return repository.Message{}, fmt.Errorf("compaction LLM call: %w", err)
	}

	var sb strings.Builder
	for _, block := range resp.Content {
		if block.Type == "text" {
			sb.WriteString(block.Text)
		}
	}
	summaryText := strings.TrimSpace(sb.String())
	if summaryText == "" {
		summaryText = "(summary unavailable)"
	}

	// Place summary just after the last middle message to preserve ordering.
	summaryTime := middle[len(middle)-1].CreatedAt.Add(time.Millisecond)

	summary := repository.Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           "user",
		Compacted:      false,
		Content:        repository.ContentBlocks{{Type: "text", Text: "[Context summary]\n" + summaryText}},
		CreatedAt:      summaryTime,
	}
	if err := s.db.WithContext(ctx).Create(&summary).Error; err != nil {
		return repository.Message{}, fmt.Errorf("persist summary: %w", err)
	}

	// Mark all middle messages compacted in one batch UPDATE.
	ids := make([]string, len(middle))
	for i, m := range middle {
		ids[i] = m.ID
	}
	if err := s.db.WithContext(ctx).
		Model(&repository.Message{}).
		Where("id IN ?", ids).
		Update("compacted", true).Error; err != nil {
		return repository.Message{}, fmt.Errorf("mark compacted: %w", err)
	}

	return summary, nil
}

// buildCompactionPrompt constructs the prompt sent to the LLM for compaction.
func buildCompactionPrompt(middle, tail []repository.Message) string {
	var sb strings.Builder
	sb.WriteString(`You are compressing conversation history to save context space.

Below are OLD MESSAGES (candidates for compression), followed by RECENT MESSAGES
(shown so you understand what has already been resolved).

Write a concise summary of the OLD MESSAGES only. Drop anything already acted on,
resolved, or superseded given the RECENT MESSAGES. Preserve:
- Open questions or unresolved tasks
- Key decisions and their reasoning
- Subagent outputs not yet summarized
- Facts the assistant will need to continue correctly

Output only the summary text. No preamble, no headings.

=== OLD MESSAGES ===
`)
	sb.WriteString(renderMessages(middle))
	sb.WriteString("\n\n=== RECENT MESSAGES (context only — do not summarize) ===\n")
	sb.WriteString(renderMessages(tail))
	return sb.String()
}

// renderMessages converts a slice of repository.Message to a plain-text representation
// suitable for inclusion in the compaction prompt.
func renderMessages(msgs []repository.Message) string {
	var sb strings.Builder
	for _, m := range msgs {
		prefix := "[" + m.Role + "]"
		if m.Role == "assistant" && m.AgentID != nil {
			prefix = "[assistant] (agent: " + *m.AgentID + ")"
		}
		for _, cb := range m.Content {
			switch cb.Type {
			case "text":
				sb.WriteString(prefix)
				sb.WriteString(": ")
				sb.WriteString(cb.Text)
				sb.WriteString("\n")
			case "tool_use":
				sb.WriteString(prefix)
				sb.WriteString(" tool_call: ")
				sb.WriteString(cb.Name)
				sb.WriteString("\n")
			case "tool_result":
				sb.WriteString("[user] tool_result: ")
				sb.WriteString(cb.Content)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String()
}
