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
		api.POST("/reports/:type/run", reportH.Run)
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
