package service_test

import (
	"context"
	"testing"
	"time"

	"social-network-go/post-service/dto"
	"social-network-go/post-service/model"
	"social-network-go/post-service/repository"
	"social-network-go/post-service/service"
)

type mockHistoryRepoForService struct {
	recordsToReturn   []dto.ViewHistoryRecord
	pageStateToReturn []byte
	errToReturn       error
}

func (m *mockHistoryRepoForService) RecordViewEvent(ctx context.Context, item dto.ViewHistoryRecord, userID string, retention time.Duration) error {
	return nil
}

func (m *mockHistoryRepoForService) GetViewsByUser(ctx context.Context, userID string, month string, limit int, pageState []byte) ([]dto.ViewHistoryRecord, []byte, error) {
	if m.errToReturn != nil {
		return nil, nil, m.errToReturn
	}
	return m.recordsToReturn, m.pageStateToReturn, nil
}

type mockPostRepoForHistory struct {
	repository.PostRepository
	postsMap map[string]*model.Post
}

func (m *mockPostRepoForHistory) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
	if p, ok := m.postsMap[postID]; ok {
		return p, nil
	}
	return nil, nil
}

func TestGetViewHistory_HydratesPostsAndFiltersDeleted(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	hRepo := &mockHistoryRepoForService{
		recordsToReturn: []dto.ViewHistoryRecord{
			{EventID: "e1", PostID: "p-active", ObservedAt: now.Add(-10 * time.Minute), VisibleDurationMs: 3000, VisibleRatio: 0.9},
			{EventID: "e2", PostID: "p-deleted", ObservedAt: now.Add(-20 * time.Minute), VisibleDurationMs: 2000, VisibleRatio: 0.8},
			{EventID: "e3", PostID: "p-missing", ObservedAt: now.Add(-30 * time.Minute), VisibleDurationMs: 2500, VisibleRatio: 0.75},
		},
	}

	pRepo := &mockPostRepoForHistory{
		postsMap: map[string]*model.Post{
			"p-active": {ID: "p-active", Content: "Active post", Privacy: "PUBLIC"},
		},
	}

	svc := service.NewViewHistoryService(hRepo, pRepo)

	res, err := svc.GetViewHistory(ctx, "user-1", "", 20)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only p-active should be returned, p-deleted and p-missing must be filtered out!
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item in history, got %d", len(res.Items))
	}
	if res.Items[0].PostID != "p-active" {
		t.Fatalf("expected p-active, got %s", res.Items[0].PostID)
	}
	if res.Items[0].Post == nil || res.Items[0].Post.Content != "Active post" {
		t.Fatalf("expected hydrated post content, got %+v", res.Items[0].Post)
	}
}

func TestGetViewHistory_CursorPagination(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	hRepo := &mockHistoryRepoForService{
		recordsToReturn: []dto.ViewHistoryRecord{
			{EventID: "e1", PostID: "p1", ObservedAt: now},
		},
		pageStateToReturn: []byte("cass-page-token-123"),
	}

	pRepo := &mockPostRepoForHistory{
		postsMap: map[string]*model.Post{
			"p1": {ID: "p1", Content: "Post 1"},
		},
	}

	svc := service.NewViewHistoryService(hRepo, pRepo)

	res, err := svc.GetViewHistory(ctx, "user-1", "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.HasMore {
		t.Fatal("expected hasMore to be true")
	}
	if res.NextCursor == "" {
		t.Fatal("expected nextCursor to be non-empty")
	}

	month, pageState, err := dto.DecodeHistoryCursor(res.NextCursor)
	if err != nil {
		t.Fatalf("failed to decode next cursor: %v", err)
	}
	if month != now.Format("2006-01") {
		t.Fatalf("expected month %s, got %s", now.Format("2006-01"), month)
	}
	if string(pageState) != "cass-page-token-123" {
		t.Fatalf("expected pageState 'cass-page-token-123', got '%s'", string(pageState))
	}
}
