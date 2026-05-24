package main

import (
	"net/http"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"social-network-go/auth-service/config"
	"social-network-go/auth-service/db"
	authgrpc "social-network-go/auth-service/grpc"
	"social-network-go/auth-service/handler"
	"social-network-go/auth-service/redis"
	"social-network-go/auth-service/service"
	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
)

func main() {
	logger.Info("Starting Auth Service...")

	// 1. Load Configurations
	cfg := config.LoadConfig()

	// 2. Initialize PostgreSQL (GORM)
	gormDB := db.InitDB(cfg)

	// 3. Initialize Redis
	redis.InitRedis(cfg)

	// 4. Dial User Service via gRPC
	userConn, err := grpc.Dial(cfg.UserGRPCAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Error("Failed to connect to User Service: %v", err)
		os.Exit(1)
	}
	defer userConn.Close()
	userClient := pb.NewUserServiceClient(userConn)

	// 5. Initialize Auth Service & Handler
	authSvc := service.NewAuthService(cfg, userClient, gormDB)
	defer authSvc.Close()
	authHandler := handler.NewAuthHandler(authSvc)

	// 5. Start gRPC Server
	authgrpc.StartGrpcServer(cfg.GRPCPort, authSvc)

	// 6. Setup and Start HTTP/REST Server (Gin)
	r := gin.Default()

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE, PATCH")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})
	r.Use(profiler.Middleware("auth-service"))
	r.Use(logger.GinMiddleware())

	// Health Check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "auth-service"})
	})

	// Profiler
	r.GET("/debug/profiler", profiler.Handler)
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Authentication Routes
	authRoutes := r.Group("/v1/auth")
	{
		authRoutes.POST("/login", authHandler.Login)
		authRoutes.POST("/login-admin", authHandler.LoginAdmin)
		authRoutes.POST("/refresh", authHandler.Refresh)
		authRoutes.DELETE("/logout", authHandler.Logout)
		authRoutes.POST("/forgot-password", authHandler.ForgotPassword)
		authRoutes.POST("/reset-password", authHandler.ResetPassword)
		authRoutes.POST("/change-password", authHandler.ChangePassword)
	}

	// Legacy/Frontend compat endpoints
	r.POST("/v1/update-password", authHandler.ResetPassword)
	r.POST("/v1/forgot-password", authHandler.ForgotPassword)

	// Registration Routes
	registerRoutes := r.Group("/v1/register")
	{
		registerRoutes.POST("", authHandler.Register)
		registerRoutes.POST("/resend-email", authHandler.ResendEmail)
		registerRoutes.PATCH("/verify", authHandler.Verify)
	}

	logger.Field("port", cfg.HTTPPort).Info("Auth HTTP Server starting")
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Field("error", err).Error("Failed to run HTTP server")
		os.Exit(1)
	}
}
