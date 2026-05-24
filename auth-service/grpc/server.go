package grpc

import (
	"context"
	"net"
	"os"
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
	if err != nil {
		return &pb.TokenResponse{
			IsValid:      false,
			ErrorMessage: err.Error(),
		}, nil
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

	grpcServer := grpc.NewServer()
	pb.RegisterAuthServiceServer(grpcServer, &GrpcServer{AuthSvc: authSvc})

	logger.Field("port", port).Info("Auth Service gRPC server listening")
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			logger.Field("error", err).Error("Failed to serve gRPC")
			os.Exit(1)
		}
	}()
}
