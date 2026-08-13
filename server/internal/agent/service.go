package agent

import (
	"context"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/mcp"
	"github/musuyin/agent-weave/internal/model/repository"
	"github/musuyin/agent-weave/internal/sandbox"
	svcpkg "github/musuyin/agent-weave/internal/service"
	"github/musuyin/agent-weave/internal/tool/builtin"
)

// Service is the agent execution service. One instance per application.
type Service struct {
	db          *gorm.DB
	aiClient    *anthropic.Client
	registry    *HubRegistry
	chain       *hook.Chain
	mcpRouter   *mcp.Router
	cfg         *config.Config
	log         *slog.Logger
	agentSvc    *svcpkg.AgentService
	dispatchReg *DispatchRegistry
}

// ProvideAgentService constructs the agent Service and registers builtin tools.
// Wire provider.
func ProvideAgentService(
	db *gorm.DB,
	cfg *config.Config,
	registry *HubRegistry,
	chain *hook.Chain,
	mcpRouter *mcp.Router,
	log *slog.Logger,
	agentSvc *svcpkg.AgentService,
	dispatchReg *DispatchRegistry,
	sandboxMgr *sandbox.Manager,
) *Service {
	opts := []option.RequestOption{option.WithAPIKey(cfg.LLMModel.Anthropic.APIKey)}
	if cfg.LLMModel.Anthropic.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(cfg.LLMModel.Anthropic.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	svc := &Service{
		db:          db,
		aiClient:    &client,
		registry:    registry,
		chain:       chain,
		mcpRouter:   mcpRouter,
		cfg:         cfg,
		log:         log,
		agentSvc:    agentSvc,
		dispatchReg: dispatchReg,
	}

	// Register dispatch_to_agent after svc is constructed so the closure captures it.
	// Note: RunSubAgentFunc is called from a goroutine already launched by the handler,
	// so this closure must NOT use `go` again.
	builtin.RegisterDispatchTool(
		db,
		agentSvc,
		func(ctx context.Context, conversationID string, thread repository.Thread, instruction string) {
			hub := svc.registry.GetOrCreate(conversationID)
			svc.RunSubAgent(ctx, conversationID, thread, instruction, hub, svc.dispatchReg)
		},
		svc.dispatchReg.Add,
		conversationIDFromCtx,
	)

	// Register sandbox tools (read_file, list_directory, write_file, run_command).
	// Overwrites the placeholder stubs that were previously registered via init().
	sandboxMgr.RegisterTools(conversationIDFromCtx)

	return svc
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
