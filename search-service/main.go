package main

import (
	"context"
	"net/http"
	"time"

	"social-network-go/logger"
	"social-network-go/profiler"
	"social-network-go/search-service/config"
	"social-network-go/search-service/consumer"
	"social-network-go/search-service/db"
	"social-network-go/search-service/handler"
	"social-network-go/search-service/service"

	"github.com/gin-gonic/gin"
)

func main() {
	logger.Info("Starting Global Search Service (with Elasticsearch & Neo4j)...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Neo4j (Graph & Fallback Store)
	db.InitNeo4j(cfg)

	// 3. Initialize Elasticsearch (Primary Full-Text Search Engine)
	db.InitElasticsearch(cfg)

	// 4. Initialize Service & Handler
	searchSvc := service.NewSearchService(cfg)
	searchHandler := handler.NewSearchHandler(searchSvc)

	// 5. Start Real-time Kafka Event Consumer for Syncing
	if searchSvc.ESRepo != nil {
		eventConsumer := consumer.NewSearchEventConsumer(cfg, searchSvc.ESRepo)
		eventConsumer.Start(context.Background())
		logger.Info("Elasticsearch Real-time Event Consumer initialized")

		imageConsumer := consumer.NewPostImageConsumer(cfg, searchSvc.ESRepo)
		imageConsumer.Start(context.Background())
		logger.Info("Elasticsearch Real-time Post Image Consumer initialized")
	}

	// 6. Setup REST Server (Gin)
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
			details["neo4j"] = "DOWN (driver not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.Neo4jDriver.VerifyConnectivity(ctx); err != nil {
				details["neo4j"] = "DOWN (" + err.Error() + ")"
			} else {
				details["neo4j"] = "UP"
			}
			cancel()
		}

		// Check Elasticsearch
		if db.ESClient == nil {
			details["elasticsearch"] = "DOWN (client not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := db.PingElasticsearch(ctx); err != nil {
				details["elasticsearch"] = "DOWN (" + err.Error() + ")"
			} else {
				details["elasticsearch"] = "UP"
			}
			cancel()
		}

		// Overall status: if both are DOWN then DOWN, otherwise partially UP
		if details["neo4j"] != "UP" && details["elasticsearch"] != "UP" {
			status = "DOWN"
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
	r.POST("/v1/search/sync", searchHandler.SyncData)
	r.POST("/v1/search/reindex", searchHandler.Reindex)

	// Visual Search APIs
	r.POST("/v1/search/image", searchHandler.SearchByImage)
	r.GET("/v1/search/visual", searchHandler.SearchVisualText)
	r.GET("/v1/search/similar", searchHandler.SearchSimilar)

	logger.Info("Search HTTP Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
