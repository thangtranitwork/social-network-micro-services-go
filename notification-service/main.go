package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"social-network-go/logger"
	"social-network-go/notification-service/config"
	"social-network-go/notification-service/handler"
	"social-network-go/notification-service/repository"
	"social-network-go/notification-service/service"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting Notification Service...")

	// 1. Load Configuration
	cfg := config.LoadConfig()

	// 2. Connect to Neo4j
	var driver neo4j.DriverWithContext
	var err error
	driver, err = neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		logger.Warn("Warning: Failed to connect to Neo4j at %s: %v", cfg.Neo4jURI, err)
	} else {
		logger.Info("Connected to Neo4j successfully")
	}

	// 3. Initialize Layers (Repository -> Service -> Handler)
	repo := repository.NewNotificationRepository(driver)
	svc := service.NewNotificationService(cfg, repo)
	h := handler.NewNotificationHandler(svc, cfg, driver)

	// 4. Start Background Kafka Consumers
	svc.StartKafkaConsumers()

	// 5. Setup Gin Router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("notification-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", h.HealthCheck)

	// Profiler Routes
	debugGroup := r.Group("/debug/profiler")
	debugGroup.Use(profiler.EndpointGuard())
	{
		debugGroup.GET("", profiler.Handler)
		debugGroup.POST("/reset", func(c *gin.Context) {
			profiler.Reset()
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
	}

	// Notification API Routes
	r.GET("/v1/notifications/ws", h.HandleWebSocket)
	r.GET("/v1/notifications", h.GetNotifications)
	r.GET("/v1/notifications/unread-count", h.GetUnreadCount)
	r.POST("/v1/notifications/read", h.MarkAsRead)
	r.PUT("/v1/notifications/read", h.MarkAsRead)
	r.POST("/v1/notifications/fcm-token", h.SaveFCMToken)

	port := ":" + cfg.HTTPPort
	logger.Info("🚀 Notification Service listening on port %s", port)
	if err := r.Run(port); err != nil {
		logger.Error("Notification Service failed to run: %v", err)
	}
}
