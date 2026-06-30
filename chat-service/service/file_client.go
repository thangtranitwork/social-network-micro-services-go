package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"social-network-go/logger"
	"social-network-go/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type GrpcFileClient struct {
	client pb.FileServiceClient
}

func NewGrpcFileClient(addr string) (*GrpcFileClient, error) {
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logger.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}
	return &GrpcFileClient{client: pb.NewFileServiceClient(conn)}, nil
}

func (c *GrpcFileClient) DeleteFiles(ctx context.Context, fileIDs []string) error {
	_, err := c.client.DeleteFiles(ctx, &pb.DeleteFilesRequest{FileIds: fileIDs})
	return err
}

func (c *GrpcFileClient) GetPresignedURL(ctx context.Context, fileID string) (string, error) {
	resp, err := c.client.GetPresignedURL(ctx, &pb.GetPresignedURLRequest{FileId: fileID})
	if err != nil {
		return "", err
	}
	return resp.Url, nil
}

func (c *GrpcFileClient) GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error) {
	resp, err := c.client.GetPresignedURLs(ctx, &pb.GetPresignedURLsRequest{FileIds: fileIDs})
	if err != nil {
		return nil, err
	}
	return resp.Urls, nil
}

func (c *GrpcFileClient) GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error) {
	resp, err := c.client.GetPresignedUploadURL(ctx, &pb.GetPresignedUploadURLRequest{Filename: filename, ContentType: contentType})
	if err != nil {
		return "", "", err
	}
	return resp.FileId, resp.Url, nil
}

func (c *GrpcFileClient) Upload(ctx context.Context, file io.Reader, filename, contentType string) (string, error) {
	fileID, uploadURL, err := c.GetPresignedUploadURL(ctx, filename, contentType)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", uploadURL, file)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to upload to minio: status %d", resp.StatusCode)
	}

	return fileID, nil
}
