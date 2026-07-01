package main

import (
	"context"
	"net"
	"net/http"
	"social-network-go/file-service/config"
	fileGrpc "social-network-go/file-service/grpc"
	"social-network-go/file-service/handler"
	"social-network-go/file-service/service"
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
		s := grpc.NewServer(grpc.UnaryInterceptor(logger.UnaryServerInterceptor()))
		pb.RegisterFileServiceServer(s, fileGrpc.NewFileGrpcServer(fileSvc))
		logger.Info("File gRPC Server starting on port %s", cfg.GRPCPort)
		if err := s.Serve(lis); err != nil {
			logger.Error("failed to serve: %v", err)
		}
	}()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(logger.TraceMiddleware())
	r.Use(profiler.Middleware("file-service"))
	r.Use(logger.GinMiddleware())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		status := "UP"
		details := gin.H{}

		// Check MinIO
		if fileSvc == nil {
			status = "DOWN"
			details["minio"] = "DOWN (service not initialized)"
		} else {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
			if err := fileSvc.Ping(ctx); err != nil {
				status = "DOWN"
				details["minio"] = "DOWN (" + err.Error() + ")"
			} else {
				details["minio"] = "UP"
			}
			cancel()
		}

		httpStatus := http.StatusOK
		if status == "DOWN" {
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"service":   "file-service",
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
