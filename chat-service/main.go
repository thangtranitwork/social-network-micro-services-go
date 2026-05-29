package main

import (
	"context"
	"net/http"
	"time"

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
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("chat-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		status := "UP"
		details := gin.H{}

		// Check MongoDB
		if db.MongoClient == nil {
			status = "DOWN"
			details["mongodb"] = "DOWN (client not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.MongoClient.Ping(ctx, nil); err != nil {
				status = "DOWN"
				details["mongodb"] = "DOWN (" + err.Error() + ")"
			} else {
				details["mongodb"] = "UP"
			}
			cancel()
		}

		// Check Neo4j
		if db.Neo4jDriver == nil {
			status = "DOWN"
			details["neo4j"] = "DOWN (driver not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.Neo4jDriver.VerifyConnectivity(ctx); err != nil {
				status = "DOWN"
				details["neo4j"] = "DOWN (" + err.Error() + ")"
			} else {
				details["neo4j"] = "UP"
			}
			cancel()
		}

		// Check Redis
		if db.RedisClient == nil {
			status = "DOWN"
			details["redis"] = "DOWN (client not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.RedisClient.Ping(ctx).Err(); err != nil {
				status = "DOWN"
				details["redis"] = "DOWN (" + err.Error() + ")"
			} else {
				details["redis"] = "UP"
			}
			cancel()
		}

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "chat-service",
			"timestamp": time.Now().Format(time.RFC3339),
			"details":   details,
		})
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

	// WebRTC ICE server configuration (STUN/TURN)
	r.GET("/v1/call/ice-servers", chatHandler.GetICEServers)

	// Group chat routes
	r.POST("/v1/chat/groups", chatHandler.CreateGroupChat)
	r.POST("/v1/chat/groups/:chatId/members", chatHandler.AddMembersToGroup)
	r.DELETE("/v1/chat/groups/:chatId/members/:userId", chatHandler.RemoveMemberFromGroup)
	r.PUT("/v1/chat/groups/:chatId", chatHandler.UpdateGroupChat)
	r.GET("/v1/chat/groups/:chatId/members", chatHandler.GetGroupMembers)

	logger.Info("Chat HTTP & WS Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
