package main

import (
	"context"
	"net"
	"net/http"
	"time"

	"social-network-go/post-service/config"
	"social-network-go/post-service/db"
	"social-network-go/post-service/handler"
	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"social-network-go/logger"
	"social-network-go/pb"
	postGrpc "social-network-go/post-service/grpc"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting Post & Feed Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Neo4j
	db.InitNeo4j(cfg)

	// 3. Initialize Service & Handler
	postRepo := repository.NewPostRepository()
	postSvc := service.NewPostService(cfg, postRepo)

	// Initialize Notification Publisher
	notifPublisher := service.NewKafkaNotificationPublisher(cfg.KafkaAddr)
	defer notifPublisher.Close()
	moderationPublisher := service.NewKafkaModerationPublisher(cfg.KafkaAddr)
	defer moderationPublisher.Close()
	keywordPublisher := service.NewKafkaKeywordPublisher(cfg.KafkaAddr, db.Neo4jDriver)
	defer keywordPublisher.Close()

	// Initialize File Client
	fileClient, err := service.NewGrpcFileClient(cfg.FileGrpcAddr)
	if err != nil {
		logger.Warn("Warning: Failed to connect to File gRPC at %s: %v", cfg.FileGrpcAddr, err)
		postSvc.WithIntegrations(nil, notifPublisher, keywordPublisher).WithModeration(moderationPublisher)
	} else {
		postSvc.WithIntegrations(fileClient, notifPublisher, keywordPublisher).WithModeration(moderationPublisher)
	}

	postHandler := handler.NewPostHandler(postSvc)
	reportHandler := handler.NewReportHandler(service.NewReportService(moderationPublisher))

	// Start Ad gRPC Server
	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			logger.Error("failed to listen gRPC: %v", err)
			return
		}
		s := grpc.NewServer(grpc.UnaryInterceptor(logger.UnaryServerInterceptor()))
		pb.RegisterAdServiceServer(s, postGrpc.NewAdGrpcServer(postSvc))
		logger.Info("Post Ad gRPC Server starting on port %s", cfg.GRPCPort)
		if err := s.Serve(lis); err != nil {
			logger.Error("failed to serve gRPC: %v", err)
		}
	}()

	// 3. Setup HTTP/REST Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("post-service"))
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

		// Check Kafka
		kafkaConn, err := net.DialTimeout("tcp", cfg.KafkaAddr, 2*time.Second)
		if err != nil {
			status = "DOWN"
			details["kafka"] = "DOWN (" + err.Error() + ")"
		} else {
			details["kafka"] = "UP"
			kafkaConn.Close()
		}

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "post-service",
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

	// User report API
	r.POST("/v1/reports", reportHandler.SubmitReport)

	logger.Info("Post HTTP Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
