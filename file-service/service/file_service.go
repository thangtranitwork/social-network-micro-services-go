package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/url"
	"path/filepath"
	"time"

	"social-network-go/file-service/config"
	"social-network-go/file-service/model"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type FileService struct {
	cfg         *config.Config
	minioClient *minio.Client
}

func NewFileService(cfg *config.Config) *FileService {
	client, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		log.Fatalf("Failed to initialize MinIO client: %v", err)
	}

	// Create bucket if it doesn't exist
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.MinioBucket)
	if err != nil {
		log.Printf("Warning: Could not check if bucket exists: %v", err)
	} else if !exists {
		err = client.MakeBucket(ctx, cfg.MinioBucket, minio.MakeBucketOptions{})
		if err != nil {
			log.Fatalf("Failed to create bucket: %v", err)
		}
		log.Printf("Bucket %s created successfully", cfg.MinioBucket)
	}

	return &FileService{
		cfg:         cfg,
		minioClient: client,
	}
}

func (s *FileService) Upload(ctx context.Context, file io.Reader, filename string, contentType string, uploaderID string) (*model.File, error) {
	ext := filepath.Ext(filename)
	fileID := uuid.New().String() + ext

	userMetadata := map[string]string{
		"original-name": filename,
		"uploader-id":   uploaderID,
	}

	_, err := s.minioClient.PutObject(ctx, s.cfg.MinioBucket, fileID, file, -1, minio.PutObjectOptions{
		ContentType:  contentType,
		UserMetadata: userMetadata,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to upload to minio: %w", err)
	}

	return &model.File{
		ID:          fileID,
		Name:        filename,
		ContentType: contentType,
		UploaderID:  uploaderID,
		CreatedAt:   time.Now(),
	}, nil
}

func (s *FileService) GetPresignedUploadURL(ctx context.Context, filename string, contentType string) (string, string, error) {
	ext := filepath.Ext(filename)
	fileID := uuid.New().String() + ext

	// Set expiration to 1 hour for upload
	expiry := time.Duration(1) * time.Hour
	presignedURL, err := s.minioClient.PresignedPutObject(ctx, s.cfg.MinioBucket, fileID, expiry)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return fileID, presignedURL.String(), nil
}

func (s *FileService) Load(ctx context.Context, id string) (*model.FileResponse, error) {
	info, err := s.minioClient.StatObject(ctx, s.cfg.MinioBucket, id, minio.StatObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	presignedURL, err := s.GetPresignedURL(ctx, id)
	if err != nil {
		return nil, err
	}

	return &model.FileResponse{
		ID:          id,
		Name:        info.UserMetadata["Original-Name"],
		ContentType: info.ContentType,
		URL:         presignedURL,
	}, nil
}

func (s *FileService) GetPresignedURL(ctx context.Context, id string) (string, error) {
	reqParams := make(url.Values)
	// Set expiration to 168 hours (7 days)
	expiry := time.Duration(168) * time.Hour
	presignedURL, err := s.minioClient.PresignedGetObject(ctx, s.cfg.MinioBucket, id, expiry, reqParams)
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}
	return presignedURL.String(), nil
}

func (s *FileService) GetPresignedURLs(ctx context.Context, ids []string) (map[string]string, error) {
	urls := make(map[string]string)
	for _, id := range ids {
		if id == "" {
			continue
		}
		url, err := s.GetPresignedURL(ctx, id)
		if err != nil {
			log.Printf("Warning: failed to get presigned URL for %s: %v", id, err)
			continue
		}
		urls[id] = url
	}
	return urls, nil
}

func (s *FileService) DeleteFile(ctx context.Context, id string) error {
	err := s.minioClient.RemoveObject(ctx, s.cfg.MinioBucket, id, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("failed to remove object: %w", err)
	}
	return nil
}

func (s *FileService) DeleteFiles(ctx context.Context, ids []string) error {
	objectsCh := make(chan minio.ObjectInfo)

	go func() {
		defer close(objectsCh)
		for _, id := range ids {
			objectsCh <- minio.ObjectInfo{
				Key: id,
			}
		}
	}()

	for err := range s.minioClient.RemoveObjects(ctx, s.cfg.MinioBucket, objectsCh, minio.RemoveObjectsOptions{}) {
		if err.Err != nil {
			return fmt.Errorf("failed to remove objects: %w", err.Err)
		}
	}

	return nil
}
