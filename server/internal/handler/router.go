package handler

import (
	"log/slog"

	"github.com/gin-gonic/gin"
)

// ProvideRouter constructs and wires the Gin engine with all routes.
// Each handler is injected by Wire, keeping the router free of construction logic.
func ProvideRouter(
	convH *ConversationHandler,
	msgH *MessageHandler,
	streamH *StreamHandler,
	reportH *ReportHandler,
	skillH *SkillHandler,
	agentH *AgentHandler,
	threadH *ThreadHandler,
	log *slog.Logger,
) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(ginLogger(log))

	r.GET("/health", Health)

	api := r.Group("/api")
	{
		api.GET("/conversations", convH.List)
		api.POST("/conversations", convH.Create)
		api.GET("/conversations/:id/messages", msgH.List)
		api.POST("/conversations/:id/messages", msgH.Send)
		api.GET("/conversations/:id/stream", streamH.Stream)
		api.GET("/conversations/:id/agents", agentH.ListConversationAgents)
		api.POST("/conversations/:id/agents", agentH.AddConversationAgent)
		api.DELETE("/conversations/:id/agents/:agentId", agentH.RemoveConversationAgent)
		api.DELETE("/conversations/:id/threads", threadH.CancelAll)
		api.POST("/reports/:type/run", reportH.Run)

		api.GET("/skills", skillH.List)
		api.POST("/skills", skillH.Create)
		api.GET("/skills/:id", skillH.Get)
		api.PUT("/skills/:id", skillH.Update)
		api.DELETE("/skills/:id", skillH.Delete)

		api.GET("/agents", agentH.List)
		api.POST("/agents", agentH.Create)
		api.GET("/agents/:id", agentH.Get)
		api.PUT("/agents/:id", agentH.Update)
		api.DELETE("/agents/:id", agentH.Delete)
		api.GET("/agents/:id/skills", agentH.ListSkills)
		api.POST("/agents/:id/skills", agentH.LoadSkill)
		api.DELETE("/agents/:id/skills/:skillId", agentH.UnloadSkill)
	}

	return r
}

func ginLogger(log *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		log.Info("request",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
		)
	}
}
