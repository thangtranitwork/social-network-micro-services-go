package grpc

import (
	"context"
	"social-network-go/file-service/service"
	"social-network-go/pb"
)

type FileGrpcServer struct {
	pb.UnimplementedFileServiceServer
	fileSvc *service.FileService
}

func NewFileGrpcServer(fileSvc *service.FileService) *FileGrpcServer {
	return &FileGrpcServer{fileSvc: fileSvc}
}

func (s *FileGrpcServer) DeleteFiles(ctx context.Context, req *pb.DeleteFilesRequest) (*pb.DeleteFilesResponse, error) {
	err := s.fileSvc.DeleteFiles(ctx, req.FileIds)
	if err != nil {
		return &pb.DeleteFilesResponse{Success: false, Message: err.Error()}, nil
	}
	return &pb.DeleteFilesResponse{Success: true}, nil
}

func (s *FileGrpcServer) GetPresignedURL(ctx context.Context, req *pb.GetPresignedURLRequest) (*pb.GetPresignedURLResponse, error) {
	url, err := s.fileSvc.GetPresignedURL(ctx, req.FileId)
	if err != nil {
		return nil, err
	}
	return &pb.GetPresignedURLResponse{Url: url}, nil
}

func (s *FileGrpcServer) GetPresignedURLs(ctx context.Context, req *pb.GetPresignedURLsRequest) (*pb.GetPresignedURLsResponse, error) {
	urls, err := s.fileSvc.GetPresignedURLs(ctx, req.FileIds)
	if err != nil {
		return nil, err
	}
	return &pb.GetPresignedURLsResponse{Urls: urls}, nil
}

func (s *FileGrpcServer) GetPresignedUploadURL(ctx context.Context, req *pb.GetPresignedUploadURLRequest) (*pb.GetPresignedUploadURLResponse, error) {
	fileID, url, err := s.fileSvc.GetPresignedUploadURL(ctx, req.Filename, req.ContentType)
	if err != nil {
		return nil, err
	}
	return &pb.GetPresignedUploadURLResponse{FileId: fileID, Url: url}, nil
}
