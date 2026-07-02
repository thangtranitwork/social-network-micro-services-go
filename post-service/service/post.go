package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"social-network-go/internal/moderation"
	"social-network-go/logger"
	"social-network-go/post-service/model"
	"social-network-go/profiler"

	"github.com/google/uuid"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

func (s *PostService) CreatePost(ctx context.Context, authorID, content, privacy string, fileIDs []string) (*model.Post, error) {
	content = strings.TrimSpace(content)

	cleanFileIDs := make([]string, 0)
	for _, id := range fileIDs {
		if id != "" {
			cleanFileIDs = append(cleanFileIDs, id)
		}
	}
	fileIDs = cleanFileIDs

	if err := validatePostRequest(content, fileIDs); err != nil {
		return nil, err
	}
	if privacy == "" {
		privacy = PostPrivacyPublic
	}
	if !isValidPostPrivacy(privacy) {
		return nil, errors.New("INVALID_POST_PRIVACY")
	}

	postID := uuid.NewString()
	now := time.Now()

	err := s.Repo.CreatePost(ctx, postID, authorID, content, privacy, fileIDs)
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, content, false)
	}
	if s.Notification != nil {
		_ = s.Notification.SendToFriends(ctx, "POST", authorID, postID, "POST", truncateByWord(content))
	}
	s.requestModeration(ctx, moderation.RequestEvent{
		TargetType: moderation.TargetPost,
		TargetID:   postID,
		AuthorID:   authorID,
		Content:    content,
		MediaIDs:   fileIDs,
		Source:     moderation.SourcePostCreated,
		OccurredAt: now,
	})

	post := &model.Post{
		ID:         postID,
		Content:    content,
		AuthorID:   authorID,
		Privacy:    privacy,
		Files:      make([]string, 0),
		Images:     make([]string, 0),
		SharedPost: false,
		Liked:      false,
		CreatedAt:  now,
		Author:     s.ResolveAuthor(ctx, authorID),
	}
	if len(fileIDs) > 0 {
		post.Files = fileIDs
		post.Images = fileIDs
	}

	s.enrichPostsWithPresignedURLs(ctx, []*model.Post{post})

	return post, nil
}

func (s *PostService) SharePost(ctx context.Context, authorID, originalPostID, content, privacy string) (*model.Post, error) {
	content = strings.TrimSpace(content)
	if content != "" && len(content) > MaxPostContentLength {
		return nil, errors.New("INVALID_POST_CONTENT_LENGTH")
	}
	if privacy == "" {
		privacy = PostPrivacyPublic
	}
	if !isValidPostPrivacy(privacy) {
		return nil, errors.New("INVALID_POST_PRIVACY")
	}

	original, err := s.GetPost(ctx, originalPostID, authorID)
	if err != nil {
		return nil, err
	}
	if original.Privacy != PostPrivacyPublic {
		return nil, errors.New("ONLY_PUBLIC_POST_CAN_BE_SHARED")
	}
	if err := s.Repo.ValidateBlockByIDs(ctx, authorID, original.AuthorID); err != nil {
		return nil, err
	}

	postID := uuid.NewString()
	err = s.Repo.SharePost(ctx, authorID, originalPostID, content, privacy, postID)
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, originalPostID, authorID, "SHARE_SCORE")
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, content, false)
	}
	if s.Notification != nil {
		_ = s.Notification.Send(ctx, "SHARE", authorID, original.AuthorID, postID, "POST", truncateByWord(content))
		_ = s.Notification.SendToFriends(ctx, "POST", authorID, postID, "POST", truncateByWord(original.Content))
	}

	return s.GetPost(ctx, postID, authorID)
}

func (s *PostService) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
	post, err := s.Repo.GetPost(ctx, postID, currentUserID)
	if err != nil {
		return nil, err
	}

	if err := s.ValidateViewPost(ctx, post, currentUserID); err != nil {
		return nil, err
	}

	if currentUserID != "" && s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, currentUserID, "GET_SCORE")
	}

	// Resolve authors
	post.Author = s.ResolveAuthor(ctx, post.AuthorID)
	if post.OriginalPost != nil {
		post.OriginalPost.Author = s.ResolveAuthor(ctx, post.OriginalAuthorID)
	}

	s.enrichPostsWithPresignedURLs(ctx, []*model.Post{post})

	return post, nil
}

func (s *PostService) GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, pageable Pageable) ([]*model.Post, error) {
	if err := s.Repo.ValidateBlockByUsername(ctx, currentUserID, authorUsername); err != nil {
		return nil, err
	}

	posts, err := s.Repo.GetPostsOfUser(ctx, authorUsername, currentUserID, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	// Post processing: Validate view post, resolve authors, enrich presigned URLs
	validPosts := make([]*model.Post, 0, len(posts))
	for _, post := range posts {
		if err := s.ValidateViewPost(ctx, post, currentUserID); err == nil {
			validPosts = append(validPosts, post)
		}
	}

	s.ResolveAuthors(ctx, validPosts)
	s.enrichPostsWithPresignedURLs(ctx, validPosts)
	return validPosts, nil
}

func (s *PostService) GetAllPosts(ctx context.Context, pageable Pageable) ([]*model.Post, error) {
	posts, err := s.Repo.GetAllPosts(ctx, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	s.ResolveAuthors(ctx, posts)
	s.enrichPostsWithPresignedURLs(ctx, posts)
	return posts, nil
}

func (s *PostService) fetchActiveAds(ctx context.Context) ([]model.Post, error) {
	if s.Redis == nil {
		return nil, nil
	}

	results, err := s.Redis.HGetAll(ctx, "active_ads").Result()
	profiler.TrackCacheLookup("post-service:cache activeAds", err == nil && len(results) > 0, err)
	if err != nil {
		return nil, err
	}

	var adPosts []model.Post
	for _, valStr := range results {
		var ad struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Description  string `json:"description"`
			MediaURL     string `json:"mediaUrl"`
			TargetURL    string `json:"targetUrl"`
			Status       string `json:"status"`
			AdvertiserID string `json:"advertiserId"`
		}

		if err := json.Unmarshal([]byte(valStr), &ad); err != nil {
			logger.Error("Failed to unmarshal cached ad: %v", err)
			continue
		}

		filesList := []string{}
		if ad.MediaURL != "" {
			filesList = []string{ad.MediaURL}
		}

		// Use Description if present, otherwise Title
		contentVal := ad.Title
		if ad.Description != "" {
			contentVal = ad.Description
		}

		authorID := ad.AdvertiserID
		if authorID == "" {
			authorID = "sponsored"
		}

		adPosts = append(adPosts, model.Post{
			ID:          ad.ID,
			Content:     contentVal,
			AuthorID:    authorID,
			Privacy:     "PUBLIC",
			CreatedAt:   time.Now(),
			SharedPost:  false,
			Files:       filesList,
			Images:      filesList,
			IsAd:        true,
			AdID:        ad.ID,
			AdTargetURL: ad.TargetURL,
			AdMediaURL:  ad.MediaURL,
			Author: model.AuthorInfo{
				ID:                authorID,
				Username:          "sponsored",
				GivenName:         "PocPoc",
				FamilyName:        "Sponsored Partner",
				ProfilePictureUrl: "",
			},
		})
	}

	return adPosts, nil
}

func (s *PostService) GetSuggestedPosts(ctx context.Context, currentUserID string, pageable Pageable) ([]*model.Post, error) {
	pageType := pageable.Type
	if pageType == "" {
		pageType = PageTypeRelevant
	}

	posts, err := profiler.TrackResult("post-service:query newsfeed.repository", func() ([]*model.Post, error) {
		return s.Repo.GetSuggestedPosts(ctx, currentUserID, pageType, pageable.Skip, normalizeLimit(pageable.Limit))
	})
	if err != nil {
		return nil, err
	}

	validPosts := make([]*model.Post, 0, len(posts))
	profiler.TrackExecution("post-service:code newsfeed.validatePosts", func() {
		for _, post := range posts {
			if err := s.ValidateViewPost(ctx, post, currentUserID); err == nil {
				validPosts = append(validPosts, post)
			}
		}
	})

	if pageType == PageTypeRelevant && s.KeywordInteractor != nil {
		ids := make([]string, 0, len(validPosts))
		for _, p := range validPosts {
			ids = append(ids, p.ID)
		}
		profiler.TrackExecution("post-service:integration newsfeed.keywords.postsLoaded", func() {
			_ = s.KeywordInteractor.PostsLoaded(ctx, ids, currentUserID)
		})
	}

	// Interleave ads if there are active ads and organic posts
	if len(validPosts) > 0 {
		ads, err := profiler.TrackResult("post-service:query newsfeed.fetchActiveAds", func() ([]model.Post, error) {
			return s.fetchActiveAds(ctx)
		})
		logger.WithContext(ctx).JsonField("ads", ads).Info("Fetched ads")
		if err == nil && len(ads) > 0 {
			// Shuffle the ads slice to show ads randomly
			rand.Shuffle(len(ads), func(i, j int) {
				ads[i], ads[j] = ads[j], ads[i]
			})

			profiler.TrackExecution("post-service:code newsfeed.interleaveAds", func() {
				interleaved := make([]*model.Post, 0, len(validPosts)+len(ads))
				adIdx := 0
				for i, post := range validPosts {
					interleaved = append(interleaved, post)
					// Interleave ad at index 2 (after 2 posts) and then every 5 posts
					if i == 1 || (i > 1 && (i-1)%5 == 0) {
						if adIdx < len(ads) {
							adCopy := ads[adIdx]
							interleaved = append(interleaved, &adCopy)
							adIdx++
						}
					}
				}
				// If we still have ads left and the feed has less than 2 posts, append the first remaining ad
				if adIdx < len(ads) && len(validPosts) < 2 {
					adCopy := ads[adIdx]
					interleaved = append(interleaved, &adCopy)
					adIdx++
				}
				validPosts = interleaved
			})
		}
	}

	// Resolve authors (including both organic posts and interleaved ads!)
	profiler.TrackExecution("post-service:integration newsfeed.resolveAuthors", func() {
		s.ResolveAuthors(ctx, validPosts)
	})

	profiler.TrackExecution("post-service:integration newsfeed.enrichPresignedURLs", func() {
		s.enrichPostsWithPresignedURLs(ctx, validPosts)
	})
	return validPosts, nil
}

func (s *PostService) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
	if !isValidPostPrivacy(privacy) {
		return errors.New("INVALID_POST_PRIVACY")
	}

	return s.Repo.UpdatePrivacy(ctx, currentUserID, postID, privacy)
}

func (s *PostService) UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string) error {
	deletedFiles, finalContent, err := s.Repo.UpdateContent(ctx, currentUserID, postID, content, newFileIDs, deleteOldFileIDs, MaxPostAttachFiles)
	if err != nil {
		return err
	}

	if len(deletedFiles) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, deletedFiles)
	}
	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.ExtractPostKeywords(ctx, postID, finalContent, true)
	}
	if content != nil {
		s.requestModeration(ctx, moderation.RequestEvent{
			TargetType: moderation.TargetPost,
			TargetID:   postID,
			AuthorID:   currentUserID,
			Content:    finalContent,
			MediaIDs:   newFileIDs,
			Source:     moderation.SourcePostUpdated,
			OccurredAt: time.Now(),
		})
	}
	return nil
}

func (s *PostService) LikePost(ctx context.Context, userID, postID string) error {
	authorID, err := profiler.TrackResult("post-service:query likePost.repository", func() (string, error) {
		return s.Repo.LikePost(ctx, userID, postID)
	})
	if err != nil {
		return err
	}

	if s.KeywordInteractor != nil || (s.Notification != nil && userID != authorID) {
		go func() {
			sideEffectCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()

			if s.KeywordInteractor != nil {
				profiler.TrackExecution("post-service:integration likePost.keywordInteract", func() {
					_ = s.KeywordInteractor.Interact(sideEffectCtx, postID, userID, "LIKE_SCORE")
				})
			}
			if s.Notification != nil && userID != authorID {
				profiler.TrackExecution("post-service:integration likePost.notification", func() {
					_ = s.Notification.Send(sideEffectCtx, "LIKE_POST", userID, authorID, postID, "POST", "")
				})
			}
		}()
	}
	return nil
}

func (s *PostService) requestModeration(ctx context.Context, event moderation.RequestEvent) {
	if s.Moderation == nil || strings.TrimSpace(event.Content) == "" {
		return
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	if err := s.Moderation.RequestReview(ctx, event); err != nil {
		logger.WithContext(ctx).Err(err).Warn("Failed to publish moderation request for %s/%s", event.TargetType, event.TargetID)
	}
}

func (s *PostService) UnlikePost(ctx context.Context, userID, postID string) error {
	_, err := profiler.TrackResult("post-service:query unlikePost.repository", func() (string, error) {
		return s.Repo.UnlikePost(ctx, userID, postID)
	})
	return err
}

func (s *PostService) DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) error {
	authorID, files, err := s.Repo.DeletePost(ctx, postID, currentUserID, isAdmin)
	if err != nil {
		return err
	}

	if len(files) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, files)
	}
	if isAdmin && s.Notification != nil && authorID != currentUserID {
		_ = s.Notification.Send(ctx, "DELETE_POST", currentUserID, authorID, postID, "POST", "")
	}
	return nil
}

func (s *PostService) ValidateViewPost(ctx context.Context, post *model.Post, viewerID string) error {
	if post == nil {
		return errors.New("POST_NOT_FOUND")
	}
	if post.AuthorID == viewerID {
		return nil
	}
	switch post.Privacy {
	case PostPrivacyPublic:
		if viewerID != "" {
			return s.Repo.ValidateBlockByIDs(ctx, viewerID, post.AuthorID)
		}
		return nil
	case PostPrivacyFriend:
		if viewerID == "" {
			return errors.New("UNAUTHORIZED")
		}
		isFriend, err := s.Repo.IsFriendByIDs(ctx, viewerID, post.AuthorID)
		if err != nil {
			return err
		}
		if !isFriend {
			return errors.New("UNAUTHORIZED")
		}
		return nil
	default:
		return errors.New("UNAUTHORIZED")
	}
}

func (s *PostService) GetFilesInPostsOfUser(ctx context.Context, username, currentUserID string, pageable Pageable) ([]string, error) {
	if err := s.Repo.ValidateBlockByUsername(ctx, currentUserID, username); err != nil {
		return nil, err
	}

	files, err := s.Repo.GetFilesInPostsOfUser(ctx, username, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	for i, fileID := range files {
		if fileID != "" && !strings.HasPrefix(fileID, "http://") && !strings.HasPrefix(fileID, "https://") {
			files[i] = fmt.Sprintf("%s/%s", s.Cfg.FileServiceURL, fileID)
		}
	}
	return files, nil
}
