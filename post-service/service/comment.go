package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"social-network-go/internal/moderation"
	"social-network-go/post-service/model"

	"github.com/google/uuid"
)

func (s *PostService) Comment(ctx context.Context, authorID, postID, content string, fileID *string) (*model.Comment, error) {
	content = strings.TrimSpace(content)
	if err := validateCommentContent(content, fileID); err != nil {
		return nil, err
	}

	post, err := s.GetPost(ctx, postID, authorID)
	if err != nil {
		return nil, err
	}

	commentID := uuid.NewString()
	err = s.Repo.Comment(ctx, commentID, authorID, postID, content, fileID)
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, postID, authorID, "COMMENT_SCORE")
	}
	if s.Notification != nil && authorID != post.AuthorID {
		_ = s.Notification.Send(ctx, "COMMENT", authorID, post.AuthorID, commentID, "COMMENT", truncateByWord(content))
	}
	mediaIDs := []string{}
	if fileID != nil && *fileID != "" {
		mediaIDs = append(mediaIDs, *fileID)
	}
	s.requestModeration(ctx, moderation.RequestEvent{
		TargetType: moderation.TargetComment,
		TargetID:   commentID,
		AuthorID:   authorID,
		Content:    content,
		MediaIDs:   mediaIDs,
		Source:     moderation.SourceCommentCreated,
		OccurredAt: time.Now(),
	})

	return s.GetCommentByID(ctx, commentID, authorID)
}

func (s *PostService) ReplyComment(ctx context.Context, authorID, originalCommentID, content string, fileID *string) (*model.Comment, error) {
	content = strings.TrimSpace(content)
	if err := validateCommentContent(content, fileID); err != nil {
		return nil, err
	}

	original, err := s.GetCommentByID(ctx, originalCommentID, authorID)
	if err != nil {
		return nil, err
	}
	if original.OriginalCommentID != "" {
		return nil, errors.New("CAN_NOT_REPLY_REPLIED_COMMENT")
	}

	post, err := s.GetPost(ctx, original.PostID, authorID)
	if err != nil {
		return nil, err
	}
	if err := s.Repo.ValidateBlockByIDs(ctx, post.AuthorID, authorID); err != nil {
		return nil, err
	}

	commentID := uuid.NewString()
	err = s.Repo.ReplyComment(ctx, commentID, authorID, originalCommentID, original.PostID, content, fileID)
	if err != nil {
		return nil, err
	}

	if s.KeywordInteractor != nil {
		_ = s.KeywordInteractor.Interact(ctx, original.PostID, authorID, "COMMENT_SCORE")
	}
	if s.Notification != nil {
		if authorID != post.AuthorID {
			_ = s.Notification.Send(ctx, "COMMENT", authorID, post.AuthorID, commentID, "COMMENT", truncateByWord(content))
		}
		if authorID != original.AuthorID {
			_ = s.Notification.Send(ctx, "REPLY_COMMENT", authorID, original.AuthorID, commentID, "COMMENT", truncateByWord(original.Content))
		}
	}
	mediaIDs := []string{}
	if fileID != nil && *fileID != "" {
		mediaIDs = append(mediaIDs, *fileID)
	}
	s.requestModeration(ctx, moderation.RequestEvent{
		TargetType: moderation.TargetComment,
		TargetID:   commentID,
		AuthorID:   authorID,
		Content:    content,
		MediaIDs:   mediaIDs,
		Source:     moderation.SourceCommentCreated,
		OccurredAt: time.Now(),
	})

	return s.GetCommentByID(ctx, commentID, authorID)
}

func (s *PostService) LikeComment(ctx context.Context, likerID, commentID string) error {
	err := s.Repo.LikeComment(ctx, likerID, commentID)
	if err != nil {
		return err
	}

	if s.Notification != nil {
		_ = s.Notification.SendToFriends(ctx, "LIKE_COMMENT", likerID, commentID, "COMMENT", "")
	}
	return nil
}

func (s *PostService) UnlikeComment(ctx context.Context, likerID, commentID string) error {
	return s.Repo.UnlikeComment(ctx, likerID, commentID)
}

func (s *PostService) UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) (*model.Comment, error) {
	err := s.Repo.UpdateCommentContent(ctx, currentUserID, commentID, content)
	if err != nil {
		return nil, err
	}
	return s.GetCommentByID(ctx, commentID, currentUserID)
}

func (s *PostService) DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool) error {
	comment, err := s.GetCommentByID(ctx, commentID, currentUserID)
	if err != nil {
		return err
	}
	post, err := s.GetPost(ctx, comment.PostID, currentUserID)
	if err != nil {
		return err
	}

	deletedFiles, err := s.Repo.DeleteComment(ctx, currentUserID, commentID, isAdmin, post.AuthorID)
	if err != nil {
		return err
	}

	if len(deletedFiles) > 0 && s.FileClient != nil {
		_ = s.FileClient.DeleteFiles(ctx, deletedFiles)
	}

	isCommentAuthor := comment.AuthorID == currentUserID
	if (isAdmin || post.AuthorID == currentUserID) && !isCommentAuthor && s.Notification != nil {
		_ = s.Notification.Send(ctx, "DELETE_COMMENT", currentUserID, comment.AuthorID, commentID, "COMMENT", "")
	}
	return nil
}

func (s *PostService) GetComments(ctx context.Context, postID, currentUserID string, pageable Pageable) ([]*model.Comment, error) {
	post, err := s.GetPost(ctx, postID, currentUserID)
	if err != nil {
		return nil, err
	}
	if err := s.ValidateViewPost(ctx, post, currentUserID); err != nil {
		return nil, err
	}

	comments, err := s.Repo.GetComments(ctx, postID, currentUserID, pageable.Type, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	for _, comment := range comments {
		comment.Author = s.ResolveAuthor(ctx, comment.AuthorID)
	}

	s.enrichCommentsWithPresignedURLs(ctx, comments)
	return comments, nil
}

func (s *PostService) GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, pageable Pageable) ([]*model.Comment, error) {
	comments, err := s.Repo.GetRepliedComments(ctx, originalCommentID, currentUserID, pageable.Skip, normalizeLimit(pageable.Limit))
	if err != nil {
		return nil, err
	}

	for _, comment := range comments {
		comment.Author = s.ResolveAuthor(ctx, comment.AuthorID)
	}

	s.enrichCommentsWithPresignedURLs(ctx, comments)
	return comments, nil
}

func (s *PostService) GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error) {
	comment, err := s.Repo.GetCommentByID(ctx, commentID, currentUserID)
	if err != nil {
		return nil, err
	}

	comment.Author = s.ResolveAuthor(ctx, comment.AuthorID)

	s.enrichCommentsWithPresignedURLs(ctx, []*model.Comment{comment})
	return comment, nil
}
