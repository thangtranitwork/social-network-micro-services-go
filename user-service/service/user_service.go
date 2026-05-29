package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"social-network-go/exception"
	config "social-network-go/user-service/config"
	"social-network-go/user-service/model"
	red "social-network-go/user-service/redis"
	"social-network-go/user-service/repository"
)

var (
	ErrUserNotFound           = exception.NewAppException(exception.UserNotFound)
	ErrInvalidUsername        = exception.NewAppException(exception.InvalidUsername)
	ErrInvalidAge             = exception.NewAppException(exception.AgeMustBeAtLeast16)
	ErrInvalidName            = exception.NewAppException(exception.InvalidGivenNameLength)
	ErrProfilePictureRequired = exception.NewAppException(exception.ProfilePictureRequired)
	ErrCannotMakeSelfRequest  = exception.NewAppException(exception.CanNotMakeSelfRequest)
)

type FileClient interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
	GetPresignedURL(ctx context.Context, fileID string) (string, error)
	GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error)
	GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error)
}

type UserService struct {
	FileClient FileClient
	UserRepo   repository.UserRepository
	cfg        *config.Config
}

func NewUserService(cfg *config.Config) *UserService {
	return &UserService{
		UserRepo: repository.NewUserRepository(),
		cfg:      cfg,
	}
}

func (s *UserService) WithIntegrations(fileClient FileClient) *UserService {
	s.FileClient = fileClient
	return s
}

func (s *UserService) clearCache(ctx context.Context, userID string) {
	if red.RedisClient == nil {
		return
	}
	authKey := fmt.Sprintf("user_info:%s", userID)
	postKey := fmt.Sprintf("user:profile:%s", userID)
	_ = red.RedisClient.Del(ctx, authKey, postKey).Err()
}

func (s *UserService) enrichUsersWithPresignedURLs(ctx context.Context, users []*model.User) {
	if len(users) == 0 {
		return
	}
	for _, u := range users {
		if u.ProfilePictureId != "" && !strings.HasPrefix(u.ProfilePictureId, "http://") && !strings.HasPrefix(u.ProfilePictureId, "https://") {
			u.ProfilePictureId = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, u.ProfilePictureId)
		}
	}
}

func parseTime(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}
