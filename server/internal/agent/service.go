package agent

import (
	"context"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"gorm.io/gorm"

	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/hook"
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
