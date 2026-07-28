package agent

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/tool"
)

// DispatchToolsForTest exposes dispatchToolsFromBlocks for external test packages.
func DispatchToolsForTest(
	ctx context.Context,
	db *gorm.DB,
	conversationID string,
	chain *hook.Chain,
	assistantBlocks repository.ContentBlocks,
) error {
	svc := &Service{db: db, chain: chain}
	return svc.dispatchToolsFromBlocks(ctx, conversationID, assistantBlocks)
}

// dispatchTools converts an Anthropic API response into repository blocks and dispatches tools.
func (s *Service) dispatchTools(ctx context.Context, conversationID string, msg anthropic.Message) error {
	var assistantBlocks repository.ContentBlocks
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
		}
	}
	return s.dispatchToolsFromBlocks(ctx, conversationID, assistantBlocks)
}

// dispatchToolsFromBlocks is the core tool-dispatch logic operating on repository blocks.
// Invariant A: assistant message persisted ONCE before any FirePre fires.
func (s *Service) dispatchToolsFromBlocks(ctx context.Context, conversationID string, assistantBlocks repository.ContentBlocks) error {
	type pendingTool struct {
		id    string
		name  string
		input json.RawMessage
	}
	var pending []pendingTool
	for _, cb := range assistantBlocks {
		if cb.Type == "tool_use" {
			pending = append(pending, pendingTool{id: cb.ID, name: cb.Name, input: cb.Input})
		}
	}

	// Persist assistant message ONCE before any hook fires (invariant A).
	if err := s.persistMessage(ctx, conversationID, "assistant", assistantBlocks); err != nil {
		return fmt.Errorf("persist assistant: %w", err)
	}

	var resultBlocks repository.ContentBlocks

	for _, pt := range pending {
		params := hook.ToolCallParams{Name: pt.name, Params: pt.input}

		// Invariant A: FirePre AFTER history write, BEFORE execution.
		preErr := s.chain.FirePre(ctx, &params)

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

		auditErr := toolErr
		if preErr != nil {
			auditErr = preErr
		}
		s.chain.FirePost(ctx, params, result, auditErr)
	}

	// Persist all tool results as a single user message (Anthropic API requirement).
	if len(resultBlocks) > 0 {
		if err := s.persistMessage(ctx, conversationID, "user", resultBlocks); err != nil {
			return fmt.Errorf("persist tool results: %w", err)
		}
	}

	return nil
}

// buildToolParams converts registered tool definitions to the Anthropic SDK format.
// InputSchema must be a map[string]any JSON Schema with "properties" and optional "required" keys.
func buildToolParams() []anthropic.ToolUnionParam {
	defs := tool.All()
	params := make([]anthropic.ToolUnionParam, 0, len(defs))
	for _, d := range defs {
		schema, ok := d.InputSchema.(map[string]any)
		if !ok {
			continue
		}

		inputSchema := anthropic.ToolInputSchemaParam{}
		if props, ok := schema["properties"]; ok {
			inputSchema.Properties = props
		}
		if req, ok := schema["required"].([]string); ok {
			inputSchema.Required = req
		}

		tp := anthropic.ToolParam{
			Name:        d.Name,
			Description: anthropic.String(d.Description),
			InputSchema: inputSchema,
		}
		params = append(params, anthropic.ToolUnionParam{OfTool: &tp})
	}
	return params
}
