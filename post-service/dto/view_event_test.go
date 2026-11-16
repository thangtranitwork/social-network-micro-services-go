package dto_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"social-network-go/post-service/dto"
)

func validEvent(now time.Time) dto.ViewEventDTO {
	return dto.ViewEventDTO{
		EventID:           "test-event-uuid-1",
		PostID:            "post-123",
		ObservedAt:        now.Add(-5 * time.Second),
		VisibleDurationMs: 2500,
		VisibleRatio:      0.75,
		Surface:           "newsfeed",
		SchemaVersion:     1,
	}
}

func TestViewEventsBatchRequest_Validate(t *testing.T) {
	now := time.Now().UTC()

	t.Run("valid batch", func(t *testing.T) {
		req := dto.ViewEventsBatchRequest{
			Events: []dto.ViewEventDTO{validEvent(now)},
		}
		if err := req.Validate(now); err != nil {
			t.Fatalf("expected valid batch, got error: %v", err)
		}
	})

	t.Run("empty batch fails", func(t *testing.T) {
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
			t.Fatalf("expected empty error, got: %v", err)
		}
	})

	t.Run("batch exceeding max limit fails", func(t *testing.T) {
		events := make([]dto.ViewEventDTO, dto.MaxEventsPerBatch+1)
		for i := range events {
			ev := validEvent(now)
			ev.EventID = fmt.Sprintf("event-%d", i)
			events[i] = ev
		}
		req := dto.ViewEventsBatchRequest{Events: events}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "exceeded maximum batch limit") {
			t.Fatalf("expected max limit error, got: %v", err)
		}
	})

	t.Run("missing eventId fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.EventID = ""
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "eventId is required") {
			t.Fatalf("expected eventId required error, got: %v", err)
		}
	})

	t.Run("missing postId fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.PostID = ""
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "postId is required") {
			t.Fatalf("expected postId required error, got: %v", err)
		}
	})

	t.Run("unsupported schemaVersion fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.SchemaVersion = 2
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "unsupported schemaVersion") {
			t.Fatalf("expected unsupported schemaVersion error, got: %v", err)
		}
	})

	t.Run("duration less than minimum fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.VisibleDurationMs = 1999
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "visibleDurationMs") {
			t.Fatalf("expected visibleDurationMs error, got: %v", err)
		}
	})

	t.Run("ratio less than minimum fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.VisibleRatio = 0.49
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "visibleRatio") {
			t.Fatalf("expected visibleRatio error, got: %v", err)
		}
	})

	t.Run("ratio greater than 1.0 fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.VisibleRatio = 1.05
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "visibleRatio") {
			t.Fatalf("expected visibleRatio error, got: %v", err)
		}
	})

	t.Run("missing surface fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.Surface = ""
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "surface is required") {
			t.Fatalf("expected surface is required error, got: %v", err)
		}
	})

	t.Run("timestamp too old in past fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.ObservedAt = now.Add(-11 * time.Minute)
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "outside accepted window") {
			t.Fatalf("expected outside accepted window error, got: %v", err)
		}
	})

	t.Run("timestamp too far in future fails", func(t *testing.T) {
		ev := validEvent(now)
		ev.ObservedAt = now.Add(35 * time.Second)
		req := dto.ViewEventsBatchRequest{Events: []dto.ViewEventDTO{ev}}
		err := req.Validate(now)
		if err == nil || !strings.Contains(err.Error(), "outside accepted window") {
			t.Fatalf("expected outside accepted window error, got: %v", err)
		}
	})
}
