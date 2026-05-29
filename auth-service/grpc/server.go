package grpc

import (
	"context"
	"fmt"
	"net"
	"os"
	red "social-network-go/auth-service/redis"
	"social-network-go/auth-service/service"
	"social-network-go/logger"
	"social-network-go/pb"

	"google.golang.org/grpc"
)

type GrpcServer struct {
	pb.AuthServiceServer
	AuthSvc *service.AuthService
}

func (s *GrpcServer) ValidateToken(ctx context.Context, req *pb.TokenRequest) (*pb.TokenResponse, error) {
	claims, err := s.AuthSvc.ValidateToken(req.Token)
	logger.WithContext(ctx).Field("token", req.Token).Info("Validating token")
	if err != nil {
		return &pb.TokenResponse{
			IsValid:      false,
			ErrorMessage: err.Error(),
		}, nil
	}

	// Check if user is suspended in Redis
	if red.RedisClient != nil {
		logger.WithContext(ctx).Field("user_id", claims.UserId).Info("Checking if user is suspended")
		suspendedKey := fmt.Sprintf("auth:suspended:user:%s", claims.UserId)
		exists, err := red.RedisClient.Exists(ctx, suspendedKey).Result()
		logger.WithContext(ctx).Field("user_id", claims.UserId).Field("exists", exists).Info("User is")
		if err == nil && exists > 0 {
			logger.WithContext(ctx).Field("user_id", claims.UserId).Warn("User is suspended")
			return &pb.TokenResponse{
				IsValid:      false,
				ErrorMessage: "USER_SUSPENDED",
			}, nil
		}
	}

	return &pb.TokenResponse{
		IsValid:      true,
		UserId:       claims.UserId,
		Email:        claims.Email,
		Role:         claims.Role,
		ErrorMessage: "",
	}, nil
}

func StartGrpcServer(port string, authSvc *service.AuthService) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Field("port", port).Field("error", err).Error("Failed to listen on TCP port")
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(logger.UnaryServerInterceptor()))
	pb.RegisterAuthServiceServer(grpcServer, &GrpcServer{AuthSvc: authSvc})

	logger.Field("port", port).Info("Auth Service gRPC server listening")
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Field("error", err).Error("Failed to serve gRPC")
			os.Exit(1)
		}
	}()
}
