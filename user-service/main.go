package main

import (
	"net/http"

	"social-network-go/user-service/config"
	"social-network-go/user-service/db"
	usergrpc "social-network-go/user-service/grpc"
	"social-network-go/user-service/handler"
	"social-network-go/user-service/redis"
	"social-network-go/user-service/service"

	"github.com/gin-gonic/gin"
	"social-network-go/logger"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting User & Graph Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize Neo4j Database
	db.InitNeo4j(cfg)
	if db.Neo4jDriver != nil {
		defer db.Neo4jDriver.Close(nil)
	}

	// 3. Initialize Redis
	redis.InitRedis(cfg)

	// 4. Initialize Core Service & Handler
	userSvc := service.NewUserService()
	
	// Initialize File Client
	fileClient, err := service.NewGrpcFileClient(cfg.FileGrpcAddr)
	if err != nil {
		logger.Warn("Warning: Failed to connect to File gRPC at %s: %v", cfg.FileGrpcAddr, err)
	} else {
		userSvc.WithIntegrations(fileClient)
	}

	userHandler := handler.NewUserHandler(userSvc)

	// 5. Start gRPC Server
	usergrpc.StartGrpcServer(cfg.GRPCPort, userSvc)

	// 6. Setup and Start HTTP/REST Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(profiler.Middleware("user-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "user-service"})
	})

	// Profiler
	r.GET("/debug/profiler", profiler.Handler)
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Profile Management REST APIs
	r.GET("/v1/users/:username", userHandler.GetUserProfile)
	r.PATCH("/v1/users/update-bio", userHandler.UpdateBio)
	r.PATCH("/v1/users/update-birthdate", userHandler.UpdateBirthdate)
	r.PATCH("/v1/users/update-name", userHandler.UpdateName)
	r.PATCH("/v1/users/update-username", userHandler.UpdateUsername)
	r.PATCH("/v1/users/update-profile-picture", userHandler.UpdateProfilePicture)

	// Friend Graph REST APIs
	r.GET("/v1/friends/suggested", userHandler.GetSuggestedFriends)
	r.GET("/v1/friends/mutual-friends/:username", userHandler.GetMutualFriends)
	r.GET("/v1/friends/:username", userHandler.GetFriends)
	r.DELETE("/v1/friends/:username", userHandler.Unfriend)

	// Block Graph REST APIs
	r.GET("/v1/blocks", userHandler.GetBlockedUsers)
	r.POST("/v1/blocks/:username", userHandler.Block)
	r.DELETE("/v1/blocks/:username", userHandler.Unblock)

	// Friend Requests REST APIs
	r.GET("/v1/friend-request/sent-requests", userHandler.GetSentRequests)
	r.GET("/v1/friend-request/received-requests", userHandler.GetReceivedRequests)
	r.POST("/v1/friend-request/send/:username", userHandler.SendFriendRequest)
	r.POST("/v1/friend-request/accept/:username", userHandler.AcceptFriendRequest)
	r.DELETE("/v1/friend-request/delete/:username", userHandler.DeleteFriendRequest)

	logger.Info("User HTTP Server starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run HTTP server: %v", err)
	}
}
