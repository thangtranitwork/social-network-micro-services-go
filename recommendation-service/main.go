package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
	"social-network-go/profiler"
	"social-network-go/recommendation-service/config"
	"social-network-go/recommendation-service/db"
	"social-network-go/recommendation-service/handler"
	"social-network-go/recommendation-service/service"
)

func main() {
	cfg := config.LoadConfig()

	logger.Info("Starting recommendation-service on port %s...", cfg.Port)

	// Initialize Neo4j Graph Database
	db.InitNeo4j(cfg)

	// Initialize Service & Handler
	recService := service.NewRecommendationService(cfg)
	recHandler := handler.NewRecommendationHandler(recService)

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("recommendation-service"))
	r.Use(logger.GinMiddleware())

	// Health Check Endpoints
	healthCheck := func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "UP",
			"service": "recommendation-service",
		})
	}
	r.GET("/", healthCheck)
	r.GET("/health", healthCheck)

	// Profiler Debug Endpoints
	debugGroup := r.Group("/debug/profiler")
	debugGroup.Use(profiler.EndpointGuard())
	{
		debugGroup.GET("", profiler.Handler)
		debugGroup.POST("/reset", func(c *gin.Context) {
			profiler.Reset()
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
	}

	// Recommendation API routes
	r.GET("/recommendations/friends", recHandler.GetSuggestedFriends)
	r.GET("/recommendations/posts", recHandler.GetSuggestedPosts)
	r.GET("/v1/recommendations/friends", recHandler.GetSuggestedFriends)
	r.GET("/v1/recommendations/posts", recHandler.GetSuggestedPosts)
	r.GET("/v1/friends/suggested", recHandler.GetSuggestedFriends)

	serverAddr := fmt.Sprintf(":%s", cfg.Port)
	if err := r.Run(serverAddr); err != nil {
		logger.Error("recommendation-service failed: %v", err)
		os.Exit(1)
	}
}
