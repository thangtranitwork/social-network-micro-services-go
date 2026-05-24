package main

import (
	"net/http"

	"social-network-go/post-service/config"
	"social-network-go/post-service/db"
	"social-network-go/post-service/handler"
	"social-network-go/post-service/service"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
)

func main() {
	logger.Info("Starting Post & Feed Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Neo4j
	db.InitNeo4j(cfg)

	// 3. Initialize Service & Handler
	postSvc := service.NewPostService(cfg)
	
	// Initialize Notification Publisher
	notifPublisher := service.NewKafkaNotificationPublisher(cfg.KafkaAddr)
	defer notifPublisher.Close()

	// Initialize File Client
	fileClient, err := service.NewGrpcFileClient(cfg.FileGrpcAddr)
	if err != nil {
		logger.Warn("Warning: Failed to connect to File gRPC at %s: %v", cfg.FileGrpcAddr, err)
		postSvc.WithIntegrations(nil, notifPublisher, nil)
	} else {
		postSvc.WithIntegrations(fileClient, notifPublisher, nil)
	}

	postHandler := handler.NewPostHandler(postSvc)

	// 3. Setup HTTP/REST Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "post-service"})
	})

	// REST APIs (Internal Routes mapped under Gateway)
	r.GET("/v1/posts/newsfeed", postHandler.GetNewsfeed)
	r.GET("/v1/posts/of-user/:username", postHandler.GetPostsOfUser)
	r.GET("/v1/posts/:id", postHandler.GetPost)
	r.POST("/v1/posts/post", postHandler.CreatePost)
	r.POST("/v1/posts/share", postHandler.SharePost)
	r.POST("/v1/posts/like/:postId", postHandler.LikePost)
	r.PATCH("/v1/posts/update-privacy/:postId", postHandler.UpdatePrivacy)
	r.PATCH("/v1/posts/update-content/:postId", postHandler.UpdateContent)
	r.DELETE("/v1/posts/unlike/:postId", postHandler.UnlikePost)
	r.DELETE("/v1/posts/:postId", postHandler.DeletePost)
	r.GET("/v1/posts/files/:username", postHandler.GetFilesInPostsOfUser)

	// Comment REST APIs
	r.POST("/v1/posts/comment", postHandler.Comment)
	r.POST("/v1/posts/reply-comment", postHandler.ReplyComment)
	r.POST("/v1/posts/like-comment/:commentId", postHandler.LikeComment)
	r.DELETE("/v1/posts/unlike-comment/:commentId", postHandler.UnlikeComment)
	r.GET("/v1/posts/comments/:postId", postHandler.GetComments)
	r.GET("/v1/posts/replied-comments/:commentId", postHandler.GetRepliedComments)
	r.DELETE("/v1/posts/comment/:commentId", postHandler.DeleteComment)

	// Compatibility aliases for frontend standard comment routes
	r.GET("/v1/comments/of-post/:postId", postHandler.GetComments)
	r.GET("/v1/comments/replies/:commentId", postHandler.GetRepliedComments)
	r.POST("/v1/comments", postHandler.Comment)
	r.POST("/v1/comments/reply", postHandler.ReplyComment)
	r.POST("/v1/comments/like/:commentId", postHandler.LikeComment)
	r.DELETE("/v1/comments/unlike/:commentId", postHandler.UnlikeComment)
	r.DELETE("/v1/comments/:commentId", postHandler.DeleteComment)

	logger.Info("Post HTTP Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
