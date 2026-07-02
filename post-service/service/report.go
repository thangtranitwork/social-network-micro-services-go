package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"social-network-go/internal/moderation"
)

const reportThreshold = 3

type ReportService struct {
	mu        sync.Mutex
	reports   map[string]map[string]moderation.ReportedEvent
	publisher ModerationPublisher
}

func NewReportService(publisher ModerationPublisher) *ReportService {
	return &ReportService{
		reports:   make(map[string]map[string]moderation.ReportedEvent),
		publisher: publisher,
	}
}

func (s *ReportService) SubmitReport(ctx context.Context, targetType, targetID, reporterID, reason string) (int, bool, error) {
	targetType = strings.ToUpper(strings.TrimSpace(targetType))
	targetID = strings.TrimSpace(targetID)
	reporterID = strings.TrimSpace(reporterID)
	reason = strings.TrimSpace(reason)

	if !moderation.IsValidTargetType(targetType) || targetID == "" || reporterID == "" || reason == "" {
		return 0, false, errors.New("INVALID_REPORT")
	}

	event := moderation.ReportedEvent{
		TargetType: targetType,
		TargetID:   targetID,
		ReporterID: reporterID,
		Reason:     reason,
		OccurredAt: time.Now(),
	}

	key := targetType + ":" + targetID
	s.mu.Lock()
	if s.reports[key] == nil {
		s.reports[key] = make(map[string]moderation.ReportedEvent)
	}
	if _, exists := s.reports[key][reporterID]; exists {
		count := len(s.reports[key])
		s.mu.Unlock()
		return count, false, errors.New("DUPLICATE_REPORT")
	}
	s.reports[key][reporterID] = event
	count := len(s.reports[key])
	thresholdReached := count == reportThreshold
	s.mu.Unlock()

	if s.publisher != nil {
		if err := s.publisher.Report(ctx, event); err != nil {
			return count, thresholdReached, err
		}
		if thresholdReached {
			_ = s.publisher.RequestReview(ctx, moderation.RequestEvent{
				TargetType: targetType,
				TargetID:   targetID,
				Content:    reason,
				Source:     moderation.SourceReportThreshold,
				Priority:   "high",
				OccurredAt: time.Now(),
			})
		}
	}

	return count, thresholdReached, nil
}
