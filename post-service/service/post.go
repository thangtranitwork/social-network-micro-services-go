package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"social-network-go/post-service/model"

	"github.com/google/uuid"
)

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

func (s *PostService) GetSuggestedPosts(ctx context.Context, currentUserID string, pageable Pageable) ([]*model.Post, error) {
	pageType := pageable.Type
	if pageType == "" {
		pageType = PageTypeRelevant
	}

	posts, err := s.Repo.GetSuggestedPosts(ctx, currentUserID, pageType, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	validPosts := make([]*model.Post, 0, len(posts))
	for _, post := range posts {
		if err := s.ValidateViewPost(ctx, post, currentUserID); err == nil {
			validPosts = append(validPosts, post)
		}
	}

	s.ResolveAuthors(ctx, validPosts)

	if pageType == PageTypeRelevant && s.KeywordInteractor != nil {
		ids := make([]string, 0, len(validPosts))
		for _, p := range validPosts {
			ids = append(ids, p.ID)
		}
		_ = s.KeywordInteractor.PostsLoaded(ctx, ids, currentUserID)
	}

	s.enrichPostsWithPresignedURLs(ctx, validPosts)
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
	return nil
}

func (s *PostService) LikePost(ctx context.Context, userID, postID string) error {
	post, err := s.GetPost(ctx, postID, userID)
	if err != nil {
		return err
	}
	if err := s.Repo.ValidateBlockByIDs(ctx, userID, post.AuthorID); err != nil {
		return err
	}

	err = s.Repo.LikePost(ctx, userID, postID)
	if err != nil {
		return err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, userID, "LIKE_SCORE")
	}
	if s.Notification != nil && userID != post.AuthorID {
		_ = s.Notification.Send(ctx, "LIKE_POST", userID, post.AuthorID, postID, "POST", "")
	}
	return nil
}

func (s *PostService) UnlikePost(ctx context.Context, userID, postID string) error {
	post, err := s.GetPost(ctx, postID, userID)
	if err != nil {
		return err
	}
	if err := s.Repo.ValidateBlockByIDs(ctx, userID, post.AuthorID); err != nil {
		return err
	}

	return s.Repo.UnlikePost(ctx, userID, postID)
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

	return s.Repo.GetFilesInPostsOfUser(ctx, username, pageable.Skip, normalizeLimit(pageable.Limit))
}
