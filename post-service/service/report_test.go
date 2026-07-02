package service

import (
	"context"
	"testing"

	"social-network-go/internal/moderation"
)

type fakeModerationPublisher struct {
	reports  []moderation.ReportedEvent
	requests []moderation.RequestEvent
}

func (p *fakeModerationPublisher) RequestReview(ctx context.Context, event moderation.RequestEvent) error {
	p.requests = append(p.requests, event)
	return nil
}

func (p *fakeModerationPublisher) Report(ctx context.Context, event moderation.ReportedEvent) error {
	p.reports = append(p.reports, event)
	return nil
}

func (p *fakeModerationPublisher) Close() error {
	return nil
}

func TestSubmitReportRejectsDuplicateReporter(t *testing.T) {
	publisher := &fakeModerationPublisher{}
	svc := NewReportService(publisher)

	if _, _, err := svc.SubmitReport(context.Background(), moderation.TargetPost, "post-1", "user-1", "spam"); err != nil {
		t.Fatalf("first report failed: %v", err)
	}
	count, threshold, err := svc.SubmitReport(context.Background(), moderation.TargetPost, "post-1", "user-1", "spam again")
	if err == nil || err.Error() != "DUPLICATE_REPORT" {
		t.Fatalf("expected duplicate report error, got %v", err)
	}
	if count != 1 || threshold {
		t.Fatalf("expected count=1 threshold=false, got count=%d threshold=%v", count, threshold)
	}
}

func TestSubmitReportPublishesHighPriorityRequestAtThreshold(t *testing.T) {
	publisher := &fakeModerationPublisher{}
	svc := NewReportService(publisher)

	for _, reporter := range []string{"user-1", "user-2", "user-3"} {
		if _, _, err := svc.SubmitReport(context.Background(), moderation.TargetComment, "comment-1", reporter, "toxic"); err != nil {
			t.Fatalf("report failed: %v", err)
		}
	}

	if len(publisher.reports) != 3 {
		t.Fatalf("expected 3 report events, got %d", len(publisher.reports))
	}
	if len(publisher.requests) != 1 {
		t.Fatalf("expected 1 moderation request, got %d", len(publisher.requests))
	}
	if publisher.requests[0].Priority != "high" {
		t.Fatalf("expected high priority request, got %q", publisher.requests[0].Priority)
	}
}
