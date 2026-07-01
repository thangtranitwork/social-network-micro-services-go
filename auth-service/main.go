package main

import (
	"context"
	"net/http"
	"os"
	"time"

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
	userConn, err := grpc.NewClient(
		cfg.UserGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logger.UnaryClientInterceptor()),
	)
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
	grpcSrv := authgrpc.StartGrpcServer(cfg.GRPCPort, authSvc)
	if grpcSrv != nil {
		defer grpcSrv.GracefulStop()
	}

	// 6. Setup and Start HTTP/REST Server (Gin)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())

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
		status := "UP"
		details := gin.H{}

		// Check PostgreSQL (GORM)
		if db.DB == nil {
			status = "DOWN"
			details["postgres"] = "DOWN (DB client not initialized)"
		} else {
			sqlDB, err := db.DB.DB()
			if err != nil {
				status = "DOWN"
				details["postgres"] = "DOWN (failed to get SQL DB client: " + err.Error() + ")"
			} else {
				ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
				if err := sqlDB.PingContext(ctx); err != nil {
					status = "DOWN"
					details["postgres"] = "DOWN (" + err.Error() + ")"
				} else {
					details["postgres"] = "UP"
				}
				cancel()
			}
		}

		// Check Redis
		if redis.RedisClient == nil {
			status = "DOWN"
			details["redis"] = "DOWN (client not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := redis.RedisClient.Ping(ctx).Err(); err != nil {
				status = "DOWN"
				details["redis"] = "DOWN (" + err.Error() + ")"
			} else {
				details["redis"] = "UP"
			}
			cancel()
		}

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "auth-service",
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
		authRoutes.POST("/2fa/generate", authHandler.Generate2FA)
		authRoutes.POST("/2fa/verify", authHandler.Verify2FA)
		authRoutes.POST("/2fa/disable", authHandler.Disable2FA)
		authRoutes.GET("/2fa/status", authHandler.Get2FAStatus)
		authRoutes.GET("/google/login", authHandler.GoogleLogin)
		authRoutes.GET("/google/callback", authHandler.GoogleCallback)
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
