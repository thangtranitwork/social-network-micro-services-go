package main

import (
	"social-network-go/admin-service/config"
	"social-network-go/admin-service/db"
	"social-network-go/admin-service/handler"
	"social-network-go/admin-service/service"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
)

func main() {
	logger.Info("Starting Admin microservice...")

	cfg := config.LoadConfig()

	// Initialize Database Connections
	db.InitDB(cfg)

	// Initialize Service
	svc := service.NewAdminService()

	// Initialize Handler
	adminHandler := handler.NewAdminHandler(svc)

	// Initialize Router
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.GinMiddleware())

	// Register Routes
	adminHandler.RegisterRoutes(r)

	// Start Server
	logger.Info("Admin Service listening on HTTP port: %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to start admin-service: %v", err)
	}
}
