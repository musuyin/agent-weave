//go:build wireinject

package main

import (
	"context"
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/wire"

	"github/musuyin/agent-weave/internal/agent"
	"github/musuyin/agent-weave/internal/config"
	"github/musuyin/agent-weave/internal/db"
	"github/musuyin/agent-weave/internal/handler"
	"github/musuyin/agent-weave/internal/hook"
	"github/musuyin/agent-weave/internal/service"
)

// InitializeApp wires all dependencies and returns a ready Gin engine plus a cleanup func.
func InitializeApp(ctx context.Context, log *slog.Logger) (*gin.Engine, func(), error) {
	wire.Build(
		config.ProvideConfig,
		db.ProvideDB,
		agent.NewHubRegistry,
		hook.ProvideSecurityHook,
		hook.NewAuditHook,
		hook.ProvideHookChain,
		agent.ProvideAgentService,
		service.NewConversationService,
		service.NewMessageService,
		handler.NewConversationHandler,
		handler.NewMessageHandler,
		handler.NewStreamHandler,
		handler.ProvideRouter,
	)
	return nil, nil, nil
}
