package service

import (
	"context"
	"errors"
	"time"

	"social-network-go/post-service/dto"
	"social-network-go/post-service/repository"
)

type ViewHistoryService struct {
	historyRepo repository.HistoryRepository
	postRepo    repository.PostRepository
}

func NewViewHistoryService(historyRepo repository.HistoryRepository, postRepo repository.PostRepository) *ViewHistoryService {
	return &ViewHistoryService{
		historyRepo: historyRepo,
		postRepo:    postRepo,
	}
}

func (s *ViewHistoryService) GetViewHistory(ctx context.Context, currentUserID string, cursorStr string, limit int) (*dto.ViewHistoryResponse, error) {
	if currentUserID == "" {
		return nil, errors.New("unauthorized: currentUserID required")
	}
	if s.historyRepo == nil {
		return &dto.ViewHistoryResponse{
			Items:   []*dto.ViewHistoryItem{},
			HasMore: false,
		}, nil
	}

	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	now := time.Now().UTC()
	currentMonth := now.Format("2006-01")

	month, pageState, err := dto.DecodeHistoryCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	if month == "" {
		month = currentMonth
	}

	records, nextPageState, err := s.historyRepo.GetViewsByUser(ctx, currentUserID, month, limit, pageState)
	if err != nil {
		return nil, err
	}

	items := make([]*dto.ViewHistoryItem, 0, len(records))

	for _, rec := range records {
		var postModel = (*dto.ViewHistoryItem)(nil)
		_ = postModel

		if s.postRepo != nil {
			post, err := s.postRepo.GetPost(ctx, rec.PostID, currentUserID)
			// Guard: If post was deleted, missing, or user is blocked/unauthorized, skip from history
			if err != nil || post == nil {
				continue
			}

			items = append(items, &dto.ViewHistoryItem{
				EventID:           rec.EventID,
				PostID:            rec.PostID,
				ViewedAt:          rec.ObservedAt,
				VisibleDurationMs: rec.VisibleDurationMs,
				VisibleRatio:      rec.VisibleRatio,
				Surface:           rec.Surface,
				Post:              post,
			})
		}
	}

	var nextCursor string
	hasMore := false

	if len(nextPageState) > 0 {
		nextCursor = dto.EncodeHistoryCursor(month, nextPageState)
		hasMore = true
	} else {
		// Determine previous month within 90 days retention window
		monthTime, parseErr := time.Parse("2006-01", month)
		if parseErr == nil {
			prevMonthTime := monthTime.AddDate(0, -1, 0)
			cutoff90Days := now.AddDate(0, 0, -90)
			// Only advance cursor to previous month if still within 90 days retention
			if prevMonthTime.After(cutoff90Days) || prevMonthTime.Format("2006-01") == cutoff90Days.Format("2006-01") {
				nextCursor = dto.EncodeHistoryCursor(prevMonthTime.Format("2006-01"), nil)
				hasMore = true
			}
		}
	}

	return &dto.ViewHistoryResponse{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}
