package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/post-service/config"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/service"
)

type mockViewPublisher struct {
	err error
}

func (m *mockViewPublisher) PublishViewEvents(ctx context.Context, userID string, events []dto.ViewEventDTO) error {
	return m.err
}

func (m *mockViewPublisher) Close() error {
	return nil
}

func setupViewEventRouter(postSvc *service.PostService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewPostHandler(postSvc)
	r.POST("/v1/posts/view-events", h.RecordViewEvents)
	return r
}

func TestRecordViewEvents(t *testing.T) {
	now := time.Now().UTC()

	t.Run("success returns 202 accepted", func(t *testing.T) {
		postSvc := &service.PostService{
			Cfg:           &config.Config{},
			ViewPublisher: &mockViewPublisher{},
		}
		r := setupViewEventRouter(postSvc)

		reqBody := dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{
				{
					EventID:           "evt-test-1",
					PostID:            "post-100",
					ObservedAt:        now.Add(-2 * time.Second),
					VisibleDurationMs: 2100,
					VisibleRatio:      0.6,
					Surface:           "newsfeed",
					SchemaVersion:     1,
				},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/posts/view-events", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "user-uuid-123")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusAccepted {
			t.Fatalf("expected status 202, got %d: %s", w.Code, w.Body.String())
		}

		var resp ApiResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Code != 202 {
			t.Fatalf("expected code 202, got %d", resp.Code)
		}
	})

	t.Run("missing user returns 401 unauthorized", func(t *testing.T) {
		postSvc := &service.PostService{Cfg: &config.Config{}}
		r := setupViewEventRouter(postSvc)

		reqBody := dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{
				{
					EventID:           "evt-test-1",
					PostID:            "post-100",
					ObservedAt:        now,
					VisibleDurationMs: 2500,
					VisibleRatio:      0.8,
					Surface:           "newsfeed",
					SchemaVersion:     1,
				},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/posts/view-events", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		// no X-User-ID
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("invalid payload returns 400 bad request", func(t *testing.T) {
		postSvc := &service.PostService{Cfg: &config.Config{}}
		r := setupViewEventRouter(postSvc)

		reqBody := dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{
				{
					EventID:           "evt-test-1",
					PostID:            "post-100",
					ObservedAt:        now,
					VisibleDurationMs: 500, // too short (< 2000)
					VisibleRatio:      0.8,
					Surface:           "newsfeed",
					SchemaVersion:     1,
				},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/posts/view-events", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "user-uuid-123")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d: %s", w.Code, w.Body.String())
		}
	})

	t.Run("broker failure returns 503 service unavailable", func(t *testing.T) {
		postSvc := &service.PostService{
			Cfg: &config.Config{},
			ViewPublisher: &mockViewPublisher{
				err: errors.New("kafka connection failure"),
			},
		}
		r := setupViewEventRouter(postSvc)

		reqBody := dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{
				{
					EventID:           "evt-test-1",
					PostID:            "post-100",
					ObservedAt:        now.Add(-1 * time.Second),
					VisibleDurationMs: 2500,
					VisibleRatio:      0.8,
					Surface:           "newsfeed",
					SchemaVersion:     1,
				},
			},
		}
		bodyBytes, _ := json.Marshal(reqBody)

		req := httptest.NewRequest(http.MethodPost, "/v1/posts/view-events", bytes.NewBuffer(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-ID", "user-uuid-123")
		w := httptest.NewRecorder()

		r.ServeHTTP(w, req)

		if w.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected status 503, got %d: %s", w.Code, w.Body.String())
		}
	})
}
