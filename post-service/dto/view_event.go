package dto

import (
	"errors"
	"fmt"
	"time"
)

const (
	CurrentSchemaVersion = 1
	MaxEventsPerBatch    = 50
	MaxPayloadBytes      = 64 * 1024 // 64 KiB
	MinVisibleDurationMs = 2000      // 2 seconds continuous visibility
	MinVisibleRatio      = 0.5       // 50% element visible

	AllowedPastWindow   = 10 * time.Minute
	AllowedFutureWindow = 30 * time.Second
)

type ViewEventDTO struct {
	EventID           string    `json:"eventId"`
	PostID            string    `json:"postId"`
	ObservedAt        time.Time `json:"observedAt"`
	VisibleDurationMs int       `json:"visibleDurationMs"`
	VisibleRatio      float64   `json:"visibleRatio"`
	Surface           string    `json:"surface"`
	SchemaVersion     int       `json:"schemaVersion"`
}

type ViewEventsBatchRequest struct {
	Events []ViewEventDTO `json:"events"`
}

type ViewEventsBatchResponse struct {
	AcceptedCount int `json:"acceptedCount"`
}

// Validate checks individual event constraints
func (e *ViewEventDTO) Validate(now time.Time) error {
	if e.EventID == "" {
		return errors.New("eventId is required")
	}
	if e.PostID == "" {
		return errors.New("postId is required")
	}
	if e.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("unsupported schemaVersion %d, expected %d", e.SchemaVersion, CurrentSchemaVersion)
	}
	if e.VisibleDurationMs < MinVisibleDurationMs {
		return fmt.Errorf("visibleDurationMs (%d) must be at least %d ms", e.VisibleDurationMs, MinVisibleDurationMs)
	}
	if e.VisibleRatio < MinVisibleRatio || e.VisibleRatio > 1.0 {
		return fmt.Errorf("visibleRatio (%.2f) must be between %.1f and 1.0", e.VisibleRatio, MinVisibleRatio)
	}
	if e.Surface == "" {
		return errors.New("surface is required")
	}
	if e.ObservedAt.IsZero() {
		return errors.New("observedAt is required")
	}

	earliestAllowed := now.Add(-AllowedPastWindow)
	latestAllowed := now.Add(AllowedFutureWindow)

	if e.ObservedAt.Before(earliestAllowed) || e.ObservedAt.After(latestAllowed) {
		return fmt.Errorf("observedAt (%s) outside accepted window [%s, %s]",
			e.ObservedAt.Format(time.RFC3339),
			earliestAllowed.Format(time.RFC3339),
			latestAllowed.Format(time.RFC3339))
	}

	return nil
}

// Validate checks the batch constraints and all individual events
func (r *ViewEventsBatchRequest) Validate(now time.Time) error {
	if len(r.Events) == 0 {
		return errors.New("events batch cannot be empty")
	}
	if len(r.Events) > MaxEventsPerBatch {
		return fmt.Errorf("exceeded maximum batch limit of %d events (received %d)", MaxEventsPerBatch, len(r.Events))
	}

	for i := range r.Events {
		if err := r.Events[i].Validate(now); err != nil {
			return fmt.Errorf("event[%d] invalid: %w", i, err)
		}
	}

	return nil
}
