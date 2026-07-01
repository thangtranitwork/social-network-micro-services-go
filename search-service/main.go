package main

import (
	"context"
	"net/http"
	"time"

	"social-network-go/logger"
	"social-network-go/profiler"
	"social-network-go/search-service/config"
	"social-network-go/search-service/db"
	"social-network-go/search-service/handler"
	"social-network-go/search-service/service"

	"github.com/gin-gonic/gin"
)

func main() {
	logger.Info("Starting Global Search Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Neo4j
	db.InitNeo4j(cfg)

	// 3. Initialize Service & Handler
	searchSvc := service.NewSearchService(cfg)
	searchHandler := handler.NewSearchHandler(searchSvc)

	// 4. Setup REST Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("search-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		status := "UP"
		details := gin.H{}

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

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "search-service",
			"timestamp": time.Now().Format(time.RFC3339),
			"details":   details,
		})
	})

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

	// REST APIs (mapped under Gateway)
	r.GET("/v1/search", searchHandler.Search)

	logger.Info("Search HTTP Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
