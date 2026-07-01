package main

import (
	"social-network-go/admin-service/config"
	"social-network-go/admin-service/db"
	"social-network-go/admin-service/handler"
	"social-network-go/admin-service/repository"
	"social-network-go/admin-service/service"

	"net/http"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting Admin microservice...")

	cfg := config.LoadConfig()

	// Initialize Database Connections
	db.InitDB(cfg)

	// Initialize Repository & Service
	repo := repository.NewAdminRepository()
	svc := service.NewAdminService(repo)

	// Initialize Handler
	adminHandler := handler.NewAdminHandler(svc)

	// Initialize Router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("admin-service"))
	r.Use(logger.GinMiddleware())

	// Profiler
	debugGroup := r.Group("/debug/profiler")
	debugGroup.Use(profiler.EndpointGuard())
	{
		debugGroup.GET("", profiler.Handler)
		debugGroup.POST("/reset", func(c *gin.Context) {
			profiler.Reset()
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
	}

	// Register Routes
	adminHandler.RegisterRoutes(r)

	// Start Server
	logger.Info("Admin Service listening on HTTP port: %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to start admin-service: %v", err)
	}
}
