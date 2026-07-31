package builtin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/service"
	"github/musuyin/agent-weave/internal/tool"
)

type DispatchParams struct {
	AgentID     string `json:"agent_id"`
	Instruction string `json:"instruction"`
}

// RunSubAgentFunc is the signature of agent.Service.RunSubAgent, accepted as a callback
// to avoid an import cycle between tool/builtin and agent.
type RunSubAgentFunc func(
	ctx context.Context,
	conversationID string,
	thread repository.Thread,
	instruction string,
)

// ConvIDFromCtxFunc retrieves the conversation ID from a context.
type ConvIDFromCtxFunc func(ctx context.Context) (string, bool)

// AddDispatchFunc increments the dispatch registry for a conversation before launching a goroutine.
type AddDispatchFunc func(convID string)

// RegisterDispatchTool registers the dispatch_to_agent builtin tool.
// Called once at startup from agent.ProvideAgentService after the Service is constructed.
func RegisterDispatchTool(
	db *gorm.DB,
	agentSvc *service.AgentService,
	runSubAgent RunSubAgentFunc,
	addDispatch AddDispatchFunc,
	convIDFromCtx ConvIDFromCtxFunc,
) {
	tool.Register(tool.ToolDef{
		Name:        "dispatch_to_agent",
		Description: "Dispatch a task to a subagent present in this conversation. Returns the thread ID.",
		InputSchema: DispatchParams{},
		Handler: func(ctx context.Context, rawParams json.RawMessage) (string, error) {
			var params DispatchParams
			if err := json.Unmarshal(rawParams, &params); err != nil {
				return "", fmt.Errorf("dispatch_to_agent: bad params: %w", err)
			}
			if params.AgentID == "" {
				return "", fmt.Errorf("dispatch_to_agent: agent_id is required")
			}
			if params.Instruction == "" {
				return "", fmt.Errorf("dispatch_to_agent: instruction is required")
			}

			convID, ok := convIDFromCtx(ctx)
			if !ok {
				return "", fmt.Errorf("dispatch_to_agent: no conversation ID in context")
			}

			// Validate agent exists.
			agentDef, err := agentSvc.Get(ctx, params.AgentID)
			if err != nil {
				return "", fmt.Errorf("dispatch_to_agent: agent not found: %w", err)
			}

			// Validate agent is in this conversation (membership gate).
			members, err := agentSvc.ListConversationAgents(ctx, convID)
			if err != nil {
				return "", fmt.Errorf("dispatch_to_agent: check membership: %w", err)
			}
			inConv := false
			for _, m := range members {
				if m.ID == params.AgentID {
					inConv = true
					break
				}
			}
			if !inConv {
				return "", fmt.Errorf("dispatch_to_agent: agent %q is not in this conversation", agentDef.Name)
			}

			// Create thread with status=running BEFORE launching goroutine (invariant B).
			now := time.Now().UTC()
			thread := repository.Thread{
				ID:             uuid.NewString(),
				ConversationID: convID,
				AgentID:        params.AgentID,
				Status:         repository.ThreadStatusRunning,
				BlockedBy:      repository.StringSlice{},
				CreatedAt:      now,
				UpdatedAt:      now,
			}
			if err := db.WithContext(ctx).Create(&thread).Error; err != nil {
				return "", fmt.Errorf("dispatch_to_agent: create thread: %w", err)
			}

			// Increment registry BEFORE goroutine launch so Pending() is accurate immediately.
			addDispatch(convID)
			go runSubAgent(ctx, convID, thread, params.Instruction)

			result, _ := json.Marshal(map[string]string{
				"thread_id": thread.ID,
				"agent":     agentDef.Name,
			})
			return string(result), nil
		},
	})
}
