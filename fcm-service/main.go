package main

import (
	"fmt"
	"net"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"google.golang.org/grpc"
	"social-network-go/fcm-service/config"
	"social-network-go/fcm-service/handler"
	"social-network-go/fcm-service/service"
	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/profiler"
)

func main() {
	logger.Info("Starting FCM Microservice...")

	// 1. Load Config
	cfg := config.LoadConfig()

	// 2. Connect to Neo4j
	var driver neo4j.DriverWithContext
	var err error
	driver, err = neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		logger.Warn("FCM Service: Failed to connect to Neo4j at %s: %v", cfg.Neo4jURI, err)
	} else {
		logger.Info("FCM Service: Connected to Neo4j successfully")
	}

	// 3. Initialize Service & Handler
	svc := service.NewFCMService(cfg, driver)
	h := handler.NewFCMHandler(svc)

	// 4. Start Background Kafka Consumer for FCM Push Events
	svc.StartKafkaConsumer()

	// 5. Start gRPC Server
	grpcPort := fmt.Sprintf(":%s", cfg.GRPCPort)
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		logger.Error("FCM Service: Failed to listen gRPC port %s: %v", grpcPort, err)
	} else {
		grpcServer := grpc.NewServer()
		pb.RegisterFCMGrpcServiceServer(grpcServer, h)
		go func() {
			logger.Info("🚀 FCM Service gRPC listening on port %s", grpcPort)
			if err := grpcServer.Serve(lis); err != nil {
				logger.Error("FCM Service gRPC Server error: %v", err)
			}
		}()
	}

	// 6. Setup Gin HTTP Router & Profiler
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("fcm-service"))
	r.Use(logger.GinMiddleware())

	// Profiler Routes
	debugGroup := r.Group("/debug/profiler")
	debugGroup.Use(profiler.EndpointGuard())
	{
		debugGroup.GET("", profiler.Handler)
		debugGroup.POST("/reset", func(c *gin.Context) {
			profiler.Reset()
			c.JSON(http.StatusOK, gin.H{"status": "success"})
		})
	}

	// Health check & REST Push Endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "fcm-service"})
	})
	r.POST("/v1/fcm/send", h.SendPush)

	httpPort := fmt.Sprintf(":%s", cfg.HTTPPort)
	logger.Info("🚀 FCM Service HTTP listening on port %s", httpPort)
	if err := r.Run(httpPort); err != nil {
		logger.Error("FCM Service HTTP server failed: %v", err)
	}
}
