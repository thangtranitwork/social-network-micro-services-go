package main

import (
	"time"

	"social-network-go/api-gateway/config"
	"social-network-go/api-gateway/router"
	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/profiler"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	logger.Info("Starting API Gateway...")

	// 1. Load configuration
	cfg := config.LoadConfig()

	// 2. Establish gRPC connection to Auth Service
	var authClient pb.AuthServiceClient
	conn, err := grpc.NewClient(
		cfg.AuthGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		logger.Warn("Warning: Failed to create Auth gRPC client at %s: %v. Auth validation will fail.", cfg.AuthGrpcAddr, err)
	} else {
		defer conn.Close()
		authClient = pb.NewAuthServiceClient(conn)
		// Force connection establishment immediately (not lazy)
		conn.Connect()
		logger.Info("Auth gRPC client created for %s (state: %s)", cfg.AuthGrpcAddr, conn.GetState().String())
		// Wait up to 3s for the connection to become READY
		for i := 0; i < 6; i++ {
			state := conn.GetState()
			if state == connectivity.Ready {
				logger.Info("Connected to Auth gRPC Service at %s", cfg.AuthGrpcAddr)
				break
			}
			logger.Info("Auth gRPC state: %s, waiting...", state.String())
			time.Sleep(500 * time.Millisecond)
		}
	}

	// 3. Initialize Gin engine
	r := gin.Default()

	r.Use(profiler.Middleware("api-gateway"))

	// 4. Setup Routes & Middlewares
	router.SetupRoutes(r, cfg, authClient)

	// 5. Start HTTP server
	logger.Info("API Gateway listening on port %s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		logger.Error("Failed to run API Gateway: %v", err)
	}
}
