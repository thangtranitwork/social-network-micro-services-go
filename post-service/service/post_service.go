package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"social-network-go/pb"
	"social-network-go/post-service/config"
	"social-network-go/post-service/model"
	"social-network-go/post-service/repository"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	PostPrivacyPublic  = "PUBLIC"
	PostPrivacyFriend  = "FRIEND"
	PostPrivacyPrivate = "PRIVATE"

	PageTypeRelevant   = "RELEVANT"
	PageTypeFriendOnly = "FRIEND_ONLY"
	PageTypeTime       = "TIME"

	MaxPostContentLength    = 5000
	MaxPostAttachFiles      = 10
	MaxCommentContentLength = 1000
)

type Pageable struct {
	Skip  int64
	Limit int64
	Type  string
}

type FileClient interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
	GetPresignedURL(ctx context.Context, fileID string) (string, error)
	GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error)
	GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error)
}

type NotificationPublisher interface {
	Send(ctx context.Context, action string, creatorID string, receiverID string, targetID string, targetType string, shortenedContent string) error
	SendToFriends(ctx context.Context, action string, creatorID string, targetID string, targetType string, shortenedContent string) error
}

type KeywordInteractor interface {
	Interact(ctx context.Context, postID string, actorID string, score string) error
	ExtractPostKeywords(ctx context.Context, postID string, content string, isUpdate bool) error
	PostsLoaded(ctx context.Context, postIDs []string, userID string) error
}

type PostService struct {
	Cfg         *config.Config
	Redis      *redis.Client
	UserClient pb.UserServiceClient
	Repo       repository.PostRepository

	FileClient        FileClient
	Notification     NotificationPublisher
	KeywordInteractor KeywordInteractor
}

func NewPostService(cfg *config.Config, repo repository.PostRepository) *PostService {
	var userClient pb.UserServiceClient

	conn, err := grpc.Dial(cfg.UserGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Warning: failed to connect User gRPC %s: %v", cfg.UserGrpcAddr, err)
	} else {
		userClient = pb.NewUserServiceClient(conn)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	return &PostService{
		Cfg:        cfg,
		Redis:      rdb,
		UserClient: userClient,
		Repo:       repo,
	}
}

func (s *PostService) WithIntegrations(fileClient FileClient, notification NotificationPublisher, keyword KeywordInteractor) *PostService {
	s.FileClient = fileClient
	s.Notification = notification
	s.KeywordInteractor = keyword
	return s
}

func (s *PostService) ResolveAuthor(ctx context.Context, authorID string) model.AuthorInfo {
	cacheKey := fmt.Sprintf("user:profile:%s", authorID)

	if s.Redis != nil {
		cached, err := s.Redis.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var info model.AuthorInfo
			if json.Unmarshal([]byte(cached), &info) == nil {
				return info
			}
		}
	}

	if s.UserClient != nil {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		resp, err := s.UserClient.GetCommonUserInfo(callCtx, &pb.UserRequest{UserId: authorID})
		if err == nil && resp != nil {
			info := model.AuthorInfo{
				ID:                resp.UserId,
				Username:          resp.Username,
				GivenName:         resp.GivenName,
				FamilyName:        resp.FamilyName,
				ProfilePictureUrl: resp.ProfilePictureId,
			}
			if s.Redis != nil {
				bytes, _ := json.Marshal(info)
				_ = s.Redis.Set(ctx, cacheKey, string(bytes), 10*time.Minute).Err()
			}
			return info
		}
	}

	return model.AuthorInfo{ID: authorID}
}

func validatePostRequest(content string, files []string) error {
	if strings.TrimSpace(content) == "" && len(files) == 0 {
		return errors.New("POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY")
	}
	if len(content) > MaxPostContentLength {
		return errors.New("INVALID_POST_CONTENT_LENGTH")
	}
	if len(files) > MaxPostAttachFiles {
		return errors.New("INVALID_NUMBER_OF_POST_ATTACHMENTS")
	}
	return nil
}

func validateCommentContent(content string, fileID *string) error {
	hasNoAttachment := fileID == nil || strings.TrimSpace(*fileID) == ""
	if strings.TrimSpace(content) == "" && hasNoAttachment {
		return errors.New("COMMENT_CONTENT_AND_ATTACH_FILE_BOTH_EMPTY")
	}
	if len(content) > MaxCommentContentLength {
		return errors.New("INVALID_COMMENT_CONTENT_LENGTH")
	}
	return nil
}

func isValidPostPrivacy(privacy string) bool {
	return privacy == PostPrivacyPublic || privacy == PostPrivacyFriend || privacy == PostPrivacyPrivate
}

func normalizeLimit(limit int64) int64 {
	if limit <= 0 || limit > 100 {
		return 20
	}
	return limit
}

func truncateByWord(s string) string {
	s = strings.TrimSpace(s)
	if len(s) <= 120 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= 120 {
		return s
	}
	return string(runes[:120]) + "..."
}

func (s *PostService) enrichPostsWithPresignedURLs(ctx context.Context, posts []*model.Post) {
	if s.FileClient == nil || len(posts) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)
	
	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, post := range posts {
		for _, fileID := range post.Files {
			addFileID(fileID)
		}
		addFileID(post.Author.ProfilePictureUrl)
		if post.OriginalPost != nil {
			for _, fileID := range post.OriginalPost.Files {
				addFileID(fileID)
			}
			addFileID(post.OriginalPost.Author.ProfilePictureUrl)
		}
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for posts: %v", err)
		return
	}

	for _, post := range posts {
		for i, fileID := range post.Files {
			if url, ok := urls[fileID]; ok {
				post.Files[i] = url
			}
		}
		post.Images = post.Files
		if url, ok := urls[post.Author.ProfilePictureUrl]; ok {
			post.Author.ProfilePictureUrl = url
		}
		if post.OriginalPost != nil {
			for i, fileID := range post.OriginalPost.Files {
				if url, ok := urls[fileID]; ok {
					post.OriginalPost.Files[i] = url
				}
			}
			post.OriginalPost.Images = post.OriginalPost.Files
			if url, ok := urls[post.OriginalPost.Author.ProfilePictureUrl]; ok {
				post.OriginalPost.Author.ProfilePictureUrl = url
			}
		}
	}
}

func (s *PostService) enrichCommentsWithPresignedURLs(ctx context.Context, comments []*model.Comment) {
	if s.FileClient == nil || len(comments) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)

	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, comment := range comments {
		for _, fileID := range comment.Files {
			addFileID(fileID)
		}
		addFileID(comment.Author.ProfilePictureUrl)
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for comments: %v", err)
		return
	}

	for _, comment := range comments {
		for i, fileID := range comment.Files {
			if url, ok := urls[fileID]; ok {
				comment.Files[i] = url
			}
		}
		if url, ok := urls[comment.FileUrl]; ok {
			comment.FileUrl = url
		}
		if url, ok := urls[comment.Author.ProfilePictureUrl]; ok {
			comment.Author.ProfilePictureUrl = url
		}
	}
}
