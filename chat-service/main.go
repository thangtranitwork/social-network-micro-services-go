package main

import (
	"net/http"

	"social-network-go/chat-service/config"
	"social-network-go/chat-service/db"
	"social-network-go/chat-service/handler"
	"social-network-go/chat-service/service"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting Chat & Call Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// Initialize MongoDB
	db.InitDB(cfg)

	// Initialize Neo4j
	db.InitNeo4j(cfg)

	// Initialize Redis
	db.InitRedis(cfg)

	// 2. Initialize Service & Handler
	chatSvc := service.NewChatService(cfg)
	
	// Initialize File Client
	fileClient, err := service.NewGrpcFileClient(cfg.FileGrpcAddr)
	if err != nil {
		logger.Warn("Warning: Failed to connect to File gRPC at %s: %v", cfg.FileGrpcAddr, err)
	} else {
		chatSvc.WithIntegrations(fileClient)
	}

	chatHandler := handler.NewChatHandler(chatSvc)

	// 3. Setup HTTP/WebSocket Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(profiler.Middleware("chat-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "chat-service"})
	})

	// Profiler
	r.GET("/debug/profiler", profiler.Handler)
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// WebSocket upgrading endpoint
	r.GET("/v1/chat/ws", chatHandler.HandleWebSocket)

	// Chat list (all conversations for current user)
	r.GET("/v1/chat", chatHandler.GetChatList)

	// Search chats
	r.GET("/v1/chat/search", chatHandler.SearchChats)

	// Chat History endpoint
	r.GET("/v1/chat/history/:partnerId", chatHandler.GetChatHistory)

	// Get messages of a specific Chat room
	r.GET("/v1/chat/messages/:chatId", chatHandler.GetChatMessages)

	// REST Message Sending endpoints
	r.POST("/v1/chat/send", chatHandler.SendMessage)
	r.POST("/v1/chat/send-file", chatHandler.SendFile)
	r.POST("/v1/chat/send-gif", chatHandler.SendGif)
	r.POST("/v1/chat/send-voice", chatHandler.SendVoice)

	// Message editing/deleting endpoints
	r.PUT("/v1/chat/edit", chatHandler.EditMessage)
	r.DELETE("/v1/chat/:messageId", chatHandler.DeleteMessage)

	// Stringee token generation
	r.POST("/v1/stringee/create-token", chatHandler.CreateStringeeToken)

	logger.Info("Chat HTTP & WS Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
