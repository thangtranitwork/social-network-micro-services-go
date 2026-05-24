package main

import (
	"net"
	"net/http"
	"social-network-go/file-service/config"
	"social-network-go/file-service/handler"
	"social-network-go/file-service/service"
	fileGrpc "social-network-go/file-service/grpc"
	"social-network-go/pb"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"social-network-go/logger"
	"social-network-go/profiler"
)

func main() {
	cfg := config.LoadConfig()

	fileSvc := service.NewFileService(cfg)
	fileHandler := handler.NewFileHandler(fileSvc)

	// Start gRPC server
	go func() {
		lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
		if err != nil {
			logger.Error("failed to listen: %v", err)
			return
		}
		s := grpc.NewServer()
		pb.RegisterFileServiceServer(s, fileGrpc.NewFileGrpcServer(fileSvc))
		logger.Info("File gRPC Server starting on port %s", cfg.GRPCPort)
		if err := s.Serve(lis); err != nil {
			logger.Error("failed to serve: %v", err)
		}
	}()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(profiler.Middleware("file-service"))
	r.Use(logger.GinMiddleware())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "UP",
			"timestamp": time.Now().Format(time.RFC3339),
			"service":   "file-service",
		})
	})

	// Profiler
	r.GET("/debug/profiler", profiler.Handler)
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	v1 := r.Group("/v1/files")
	{
		v1.POST("/upload", fileHandler.Upload)
		v1.POST("/upload-multiple", fileHandler.UploadMultiple)
		v1.GET("/upload/presigned", fileHandler.GetPresignedUploadURL)
		v1.GET("/:id", fileHandler.Load)
		v1.GET("/:id/presigned", fileHandler.GetPresignedURL)
		v1.DELETE("/:id", fileHandler.Delete)
		v1.POST("/delete-multiple", fileHandler.DeleteMultiple)
	}

	logger.Info("File Service starting on port %s", cfg.HTTPPort)
	if err := r.Run(":" + cfg.HTTPPort); err != nil {
		logger.Error("Failed to run server: %v", err)
	}
}
