package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/pb"
	"social-network-go/post-service/config"
	"social-network-go/post-service/model"
	"social-network-go/post-service/repository"
	"social-network-go/profiler"

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
}

type PostService struct {
	Cfg        *config.Config
	Redis      *redis.Client
	UserClient pb.UserServiceClient
	Repo       repository.PostRepository

	FileClient        FileClient
	Notification      NotificationPublisher
	KeywordInteractor KeywordInteractor
	Moderation        ModerationPublisher
}

func NewPostService(cfg *config.Config, repo repository.PostRepository) *PostService {
	var userClient pb.UserServiceClient
	conn, err := grpc.NewClient(
		cfg.UserGrpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(logger.UnaryClientInterceptor()),
	)
	if err != nil {
		logger.Err(err).Warn("failed to connect User gRPC %s", cfg.UserGrpcAddr)
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

func (s *PostService) WithModeration(publisher ModerationPublisher) *PostService {
	s.Moderation = publisher
	return s
}

func (s *PostService) ResolveAuthor(ctx context.Context, authorID string) model.AuthorInfo {
	cacheKey := fmt.Sprintf("user:profile:%s", authorID)

	if s.Redis != nil {
		cached, err := s.Redis.Get(ctx, cacheKey).Result()
		lookupErr := err
		if errors.Is(err, redis.Nil) {
			lookupErr = nil
		}
		if err == nil && cached != "" {
			var info model.AuthorInfo
			if json.Unmarshal([]byte(cached), &info) == nil {
				profiler.TrackCacheLookup("post-service:cache authorProfile", true, nil)
				return info
			}
		}
		profiler.TrackCacheLookup("post-service:cache authorProfile", false, lookupErr)
	}

	if s.UserClient != nil {
		callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()

		resp, err := s.UserClient.GetCommonUserInfo(callCtx, &pb.UserRequest{UserId: authorID})
		if err != nil {
			logger.Err(err).Error("Failed to get common user info for author %s", authorID)
		} else if resp != nil {
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

func (s *PostService) ResolveAuthors(ctx context.Context, posts []*model.Post) {
	if len(posts) == 0 {
		return
	}

	uniqueIDsMap := make(map[string]bool)
	for _, post := range posts {
		if post.AuthorID != "" {
			uniqueIDsMap[post.AuthorID] = true
		}
		if post.OriginalPost != nil && post.OriginalAuthorID != "" {
			uniqueIDsMap[post.OriginalAuthorID] = true
		}
	}

	if len(uniqueIDsMap) == 0 {
		return
	}

	authorMap := make(map[string]model.AuthorInfo)
	uncachedIDs := make([]string, 0, len(uniqueIDsMap))

	if s.Redis != nil {
		keys := make([]string, 0, len(uniqueIDsMap))
		idToKey := make(map[string]string)
		for id := range uniqueIDsMap {
			key := fmt.Sprintf("user:profile:%s", id)
			keys = append(keys, key)
			idToKey[key] = id
		}

		vals, err := s.Redis.MGet(ctx, keys...).Result()
		if err == nil {
			for i, val := range vals {
				if val != nil {
					if strVal, ok := val.(string); ok && strVal != "" {
						var info model.AuthorInfo
						if json.Unmarshal([]byte(strVal), &info) == nil {
							authorMap[info.ID] = info
							profiler.TrackCacheLookup("post-service:cache authorProfile", true, nil)
							continue
						}
					}
				}
				key := keys[i]
				profiler.TrackCacheLookup("post-service:cache authorProfile", false, nil)
				uncachedIDs = append(uncachedIDs, idToKey[key])
			}
		} else {
			for id := range uniqueIDsMap {
				profiler.TrackCacheLookup("post-service:cache authorProfile", false, err)
				uncachedIDs = append(uncachedIDs, id)
			}
		}
	} else {
		for id := range uniqueIDsMap {
			profiler.TrackCacheLookup("post-service:cache authorProfile", false, nil)
			uncachedIDs = append(uncachedIDs, id)
		}
	}

	if len(uncachedIDs) > 0 && s.UserClient != nil {
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		resp, err := s.UserClient.GetUsersByIds(callCtx, &pb.UsersByIdsRequest{UserIds: uncachedIDs})
		if err != nil {
			logger.Err(err).Error("Failed to GetUsersByIds for %v", uncachedIDs)
		} else if resp != nil && len(resp.Users) > 0 {
			var pipe redis.Pipeliner
			if s.Redis != nil {
				pipe = s.Redis.Pipeline()
			}

			for _, u := range resp.Users {
				info := model.AuthorInfo{
					ID:                u.UserId,
					Username:          u.Username,
					GivenName:         u.GivenName,
					FamilyName:        u.FamilyName,
					ProfilePictureUrl: u.ProfilePictureId,
				}
				authorMap[info.ID] = info

				if pipe != nil {
					cacheKey := fmt.Sprintf("user:profile:%s", info.ID)
					bytes, _ := json.Marshal(info)
					_ = pipe.Set(ctx, cacheKey, string(bytes), 10*time.Minute)
				}
			}

			if pipe != nil {
				_, _ = pipe.Exec(ctx)
			}
		}
	}

	for _, post := range posts {
		if post.AuthorID != "" {
			if info, ok := authorMap[post.AuthorID]; ok {
				post.Author = info
			} else {
				post.Author = model.AuthorInfo{ID: post.AuthorID}
			}
		}
		if post.OriginalPost != nil && post.OriginalAuthorID != "" {
			if info, ok := authorMap[post.OriginalAuthorID]; ok {
				post.OriginalPost.Author = info
			} else {
				post.OriginalPost.Author = model.AuthorInfo{ID: post.OriginalAuthorID}
			}
		}
	}
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
	if len(posts) == 0 {
		return
	}
	for _, post := range posts {
		for i, fileID := range post.Files {
			if fileID != "" && !strings.HasPrefix(fileID, "http://") && !strings.HasPrefix(fileID, "https://") {
				post.Files[i] = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, fileID)
			}
		}
		post.Images = post.Files
		if post.Author.ProfilePictureUrl != "" && !strings.HasPrefix(post.Author.ProfilePictureUrl, "http://") && !strings.HasPrefix(post.Author.ProfilePictureUrl, "https://") {
			post.Author.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, post.Author.ProfilePictureUrl)
		}
		if post.OriginalPost != nil {
			for i, fileID := range post.OriginalPost.Files {
				if fileID != "" && !strings.HasPrefix(fileID, "http://") && !strings.HasPrefix(fileID, "https://") {
					post.OriginalPost.Files[i] = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, fileID)
				}
			}
			post.OriginalPost.Images = post.OriginalPost.Files
			if post.OriginalPost.Author.ProfilePictureUrl != "" && !strings.HasPrefix(post.OriginalPost.Author.ProfilePictureUrl, "http://") && !strings.HasPrefix(post.OriginalPost.Author.ProfilePictureUrl, "https://") {
				post.OriginalPost.Author.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, post.OriginalPost.Author.ProfilePictureUrl)
			}
		}
	}
}

func (s *PostService) enrichCommentsWithPresignedURLs(ctx context.Context, comments []*model.Comment) {
	if len(comments) == 0 {
		return
	}
	for _, comment := range comments {
		for i, fileID := range comment.Files {
			if fileID != "" && !strings.HasPrefix(fileID, "http://") && !strings.HasPrefix(fileID, "https://") {
				comment.Files[i] = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, fileID)
			}
		}
		if comment.FileUrl != "" && !strings.HasPrefix(comment.FileUrl, "http://") && !strings.HasPrefix(comment.FileUrl, "https://") {
			comment.FileUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, comment.FileUrl)
		}
		if comment.Author.ProfilePictureUrl != "" && !strings.HasPrefix(comment.Author.ProfilePictureUrl, "http://") && !strings.HasPrefix(comment.Author.ProfilePictureUrl, "https://") {
			comment.Author.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, comment.Author.ProfilePictureUrl)
		}
	}
}
