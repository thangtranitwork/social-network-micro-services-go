package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"social-network-go/file-service/config"
	"social-network-go/file-service/model"
	"social-network-go/logger"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type FileService struct {
	cfg         *config.Config
	minioClient *minio.Client
}

var ErrForbiddenFileDelete = errors.New("forbidden file delete")

func NewFileService(cfg *config.Config) *FileService {
	client, err := minio.New(cfg.MinioEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinioAccessKey, cfg.MinioSecretKey, ""),
		Secure: cfg.MinioUseSSL,
	})
	if err != nil {
		logger.Err(err).Fatal("Failed to initialize MinIO client")
	}

	// Create bucket if it doesn't exist
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.MinioBucket)
	if err != nil {
		logger.Err(err).Warn("Could not check if bucket exists")
	} else if !exists {
		err = client.MakeBucket(ctx, cfg.MinioBucket, minio.MakeBucketOptions{})
		if err != nil {
			logger.Err(err).Fatal("Failed to create bucket")
		}
		logger.Info("Bucket %s created successfully", cfg.MinioBucket)
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
		logger.Err(err).Error("Failed to upload file %s to minio", filename)
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
		logger.Err(err).Error("Failed to generate presigned upload URL for %s", filename)
		return "", "", fmt.Errorf("failed to generate presigned upload URL: %w", err)
	}

	return fileID, presignedURL.String(), nil
}

func (s *FileService) Load(ctx context.Context, id string) (io.ReadCloser, string, string, int64, error) {
	info, err := s.minioClient.StatObject(ctx, s.cfg.MinioBucket, id, minio.StatObjectOptions{})
	if err != nil {
		logger.Err(err).Error("Failed to stat object %s", id)
		return nil, "", "", 0, fmt.Errorf("failed to stat object: %w", err)
	}

	obj, err := s.minioClient.GetObject(ctx, s.cfg.MinioBucket, id, minio.GetObjectOptions{})
	if err != nil {
		logger.Err(err).Error("Failed to get object %s", id)
		return nil, "", "", 0, fmt.Errorf("failed to get object: %w", err)
	}

	originalName := info.UserMetadata["Original-Name"]
	if originalName == "" {
		originalName = id
	}

	return obj, originalName, info.ContentType, info.Size, nil
}

func (s *FileService) GetPresignedURL(ctx context.Context, id string) (string, error) {
	reqParams := make(url.Values)
	// Set expiration to 168 hours (7 days)
	expiry := time.Duration(168) * time.Hour
	presignedURL, err := s.minioClient.PresignedGetObject(ctx, s.cfg.MinioBucket, id, expiry, reqParams)
	if err != nil {
		logger.Err(err).Error("Failed to generate presigned GET URL for %s", id)
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
			logger.Err(err).Warn("failed to get presigned URL for %s", id)
			continue
		}
		urls[id] = url
	}
	return urls, nil
}

func (s *FileService) DeleteFile(ctx context.Context, id string) error {
	err := s.minioClient.RemoveObject(ctx, s.cfg.MinioBucket, id, minio.RemoveObjectOptions{})
	if err != nil {
		logger.Err(err).Error("Failed to remove object %s", id)
		return fmt.Errorf("failed to remove object: %w", err)
	}
	return nil
}

func (s *FileService) DeleteFileForUser(ctx context.Context, id, userID string, isAdmin bool) error {
	if !isAdmin {
		ownerID, err := s.uploaderID(ctx, id)
		if err != nil {
			return err
		}
		if ownerID == "" || ownerID != userID {
			return ErrForbiddenFileDelete
		}
	}
	return s.DeleteFile(ctx, id)
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
			logger.Err(err.Err).Error("Failed to remove objects")
			return fmt.Errorf("failed to remove objects: %w", err.Err)
		}
	}

	return nil
}

func (s *FileService) DeleteFilesForUser(ctx context.Context, ids []string, userID string, isAdmin bool) error {
	if !isAdmin {
		for _, id := range ids {
			ownerID, err := s.uploaderID(ctx, id)
			if err != nil {
				return err
			}
			if ownerID == "" || ownerID != userID {
				return ErrForbiddenFileDelete
			}
		}
	}
	return s.DeleteFiles(ctx, ids)
}

func (s *FileService) uploaderID(ctx context.Context, id string) (string, error) {
	info, err := s.minioClient.StatObject(ctx, s.cfg.MinioBucket, id, minio.StatObjectOptions{})
	if err != nil {
		logger.Err(err).Error("Failed to stat object %s for ownership check", id)
		return "", fmt.Errorf("failed to stat object: %w", err)
	}
	for key, val := range info.UserMetadata {
		normalized := strings.ToLower(strings.ReplaceAll(key, "-", ""))
		if normalized == "uploaderid" {
			return val, nil
		}
	}
	return "", nil
}

func (s *FileService) Ping(ctx context.Context) error {
	_, err := s.minioClient.BucketExists(ctx, s.cfg.MinioBucket)
	return err
}
