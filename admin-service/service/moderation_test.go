package service

import (
	"context"
	"testing"

	"social-network-go/internal/moderation"
)

func TestModerationQueueUpsertAndFilter(t *testing.T) {
	svc := NewAdminService(nil)

	svc.UpsertModerationResult(context.Background(), moderation.CompletedEvent{
		TargetType: moderation.TargetPost,
		TargetID:   "post-1",
		AuthorID:   "author-1",
		Verdict:    moderation.VerdictNeedsReview,
		Categories: []string{moderation.CategorySpam},
		Confidence: 0.72,
		Reason:     "spam signal",
		Action:     moderation.ActionQueue,
	})

	items := svc.ListModerationQueue(context.Background(), ModerationQueueFilter{Category: moderation.CategorySpam})
	if len(items) != 1 {
		t.Fatalf("expected one queue item, got %d", len(items))
	}
	if items[0].AuthorID != "author-1" || items[0].Status != ModerationStatusPending {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestApproveModerationItemUpdatesStatus(t *testing.T) {
	svc := NewAdminService(nil)
	svc.UpsertModerationResult(context.Background(), moderation.CompletedEvent{
		TargetType: moderation.TargetPost,
		TargetID:   "post-1",
		Verdict:    moderation.VerdictNeedsReview,
		Action:     moderation.ActionQueue,
	})

	if err := svc.ApproveModerationItem(context.Background(), "POST:post-1", "admin-1", "looks fine"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	items := svc.ListModerationQueue(context.Background(), ModerationQueueFilter{Status: ModerationStatusApproved})
	if len(items) != 1 {
		t.Fatalf("expected approved item, got %d", len(items))
	}
}
