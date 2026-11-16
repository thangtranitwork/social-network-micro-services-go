package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"social-network-go/post-service/config"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/service"
)

type mockViewEventPublisher struct {
	publishedEvents []dto.ViewEventDTO
	publishedUserID string
	errToReturn     error
}

func (m *mockViewEventPublisher) PublishViewEvents(ctx context.Context, userID string, events []dto.ViewEventDTO) error {
	if m.errToReturn != nil {
		return m.errToReturn
	}
	m.publishedUserID = userID
	m.publishedEvents = events
	return nil
}

func (m *mockViewEventPublisher) Close() error {
	return nil
}

func sampleValidEvent(now time.Time) dto.ViewEventDTO {
	return dto.ViewEventDTO{
		EventID:           "evt-12345",
		PostID:            "post-8888",
		ObservedAt:        now.Add(-2 * time.Second),
		VisibleDurationMs: 2200,
		VisibleRatio:      0.8,
		Surface:           "newsfeed",
		SchemaVersion:     1,
	}
}

func TestProcessViewEvents(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	t.Run("success batch processing", func(t *testing.T) {
		mockPub := &mockViewEventPublisher{}
		svc := &service.PostService{
			Cfg:           &config.Config{},
			ViewPublisher: mockPub,
		}

		req := &dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{sampleValidEvent(now)},
		}

		resp, err := svc.ProcessViewEvents(ctx, "user-1", req)
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if resp.AcceptedCount != 1 {
			t.Fatalf("expected accepted count 1, got %d", resp.AcceptedCount)
		}
		if mockPub.publishedUserID != "user-1" {
			t.Fatalf("expected user-1, got %s", mockPub.publishedUserID)
		}
	})

	t.Run("unauthorized when currentUserID empty", func(t *testing.T) {
		svc := &service.PostService{}
		req := &dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{sampleValidEvent(now)},
		}

		_, err := svc.ProcessViewEvents(ctx, "", req)
		if !errors.Is(err, service.ErrUnauthorized) {
			t.Fatalf("expected ErrUnauthorized, got %v", err)
		}
	})

	t.Run("bad request when payload invalid", func(t *testing.T) {
		svc := &service.PostService{}
		badReq := &dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{
				{
					EventID:           "",
					PostID:            "post-1",
					ObservedAt:        now,
					VisibleDurationMs: 1000,
					VisibleRatio:      0.2,
					Surface:           "newsfeed",
					SchemaVersion:     1,
				},
			},
		}

		_, err := svc.ProcessViewEvents(ctx, "user-1", badReq)
		if !errors.Is(err, service.ErrBadRequest) {
			t.Fatalf("expected ErrBadRequest, got %v", err)
		}
	})

	t.Run("service unavailable when publisher fails", func(t *testing.T) {
		mockPub := &mockViewEventPublisher{
			errToReturn: errors.New("kafka connection broker down"),
		}
		svc := &service.PostService{
			Cfg:           &config.Config{},
			ViewPublisher: mockPub,
		}

		req := &dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{sampleValidEvent(now)},
		}

		_, err := svc.ProcessViewEvents(ctx, "user-1", req)
		if !errors.Is(err, service.ErrServiceUnavailable) {
			t.Fatalf("expected ErrServiceUnavailable, got %v", err)
		}
	})
}
