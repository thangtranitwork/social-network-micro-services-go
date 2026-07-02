package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"social-network-go/admin-service/model"
	"social-network-go/internal/moderation"
	"social-network-go/logger"
)

const (
	ModerationStatusPending   = "pending"
	ModerationStatusApproved  = "approved"
	ModerationStatusHidden    = "hidden"
	ModerationStatusDeleted   = "deleted"
	ModerationStatusSuspended = "author_suspended"
)

type ModerationQueueFilter struct {
	Status   string
	Category string
}

func (s *AdminService) DeletePost(ctx context.Context, postID string) error {
	return s.repo.DeletePost(ctx, postID)
}

func (s *AdminService) SuspendUser(ctx context.Context, userID string, duration time.Duration) error {
	return s.repo.SuspendUser(ctx, userID, duration)
}

func (s *AdminService) UnsuspendUser(ctx context.Context, userID string) error {
	return s.repo.UnsuspendUser(ctx, userID)
}

func moderationItemID(targetType, targetID string) string {
	return strings.ToUpper(targetType) + ":" + targetID
}

func (s *AdminService) UpsertModerationResult(ctx context.Context, event moderation.CompletedEvent) {
	if !moderation.IsValidTargetType(event.TargetType) || !moderation.IsValidVerdict(event.Verdict) || event.TargetID == "" {
		return
	}
	if event.Verdict == moderation.VerdictSafe {
		s.recordAudit(ctx, "", moderation.ActionNone, event.TargetType, event.TargetID, event.Reason)
		return
	}

	now := time.Now()
	id := moderationItemID(event.TargetType, event.TargetID)
	status := ModerationStatusPending
	if event.Action == moderation.ActionAutoHide {
		status = ModerationStatusHidden
		if event.TargetType == moderation.TargetPost && s.repo != nil {
			actionCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			_ = s.repo.DeletePost(actionCtx, event.TargetID)
			cancel()
		}
	}

	item := model.ModerationQueueItem{
		ID:         id,
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		AuthorID:   event.AuthorID,
		Status:     status,
		Verdict:    event.Verdict,
		Categories: moderation.NormalizeCategories(event.Categories),
		Confidence: event.Confidence,
		Reason:     event.Reason,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if s.moderationRepo != nil {
		if err := s.moderationRepo.UpsertQueueItem(ctx, item); err != nil {
			logger.WithContext(ctx).Err(err).Error("Failed to persist moderation queue item")
		} else {
			if event.Action == moderation.ActionAutoHide {
				s.recordAudit(ctx, "", moderation.ActionAutoHide, event.TargetType, event.TargetID, event.Reason)
			}
			return
		}
	}

	s.moderation.mu.Lock()
	defer s.moderation.mu.Unlock()

	stored := s.moderation.items[id]
	if stored == nil {
		stored = &model.ModerationQueueItem{
			ID:         id,
			TargetType: event.TargetType,
			TargetID:   event.TargetID,
			CreatedAt:  now,
		}
		s.moderation.items[id] = stored
	}
	stored.Status = status
	if event.AuthorID != "" {
		stored.AuthorID = event.AuthorID
	}
	stored.Verdict = event.Verdict
	stored.Categories = moderation.NormalizeCategories(event.Categories)
	stored.Confidence = event.Confidence
	stored.Reason = event.Reason
	stored.UpdatedAt = now
	if event.Action == moderation.ActionAutoHide {
		s.moderation.audits = append(s.moderation.audits, model.ModerationAuditLog{
			ID:         uuid.NewString(),
			Action:     moderation.ActionAutoHide,
			TargetType: event.TargetType,
			TargetID:   event.TargetID,
			Reason:     event.Reason,
			CreatedAt:  now,
		})
	}
}

func (s *AdminService) RecordReport(ctx context.Context, event moderation.ReportedEvent) int {
	if !moderation.IsValidTargetType(event.TargetType) || event.TargetID == "" || event.ReporterID == "" {
		return 0
	}
	now := time.Now()
	id := moderationItemID(event.TargetType, event.TargetID)

	if s.moderationRepo != nil {
		count, err := s.moderationRepo.RecordReport(ctx, event)
		if err != nil {
			logger.WithContext(ctx).Err(err).Error("Failed to persist moderation report")
		} else {
			item, err := s.moderationRepo.GetQueueItem(ctx, id)
			if err != nil {
				logger.WithContext(ctx).Err(err).Error("Failed to fetch existing moderation queue item")
			}
			if item == nil {
				item = &model.ModerationQueueItem{
					ID:         id,
					TargetType: event.TargetType,
					TargetID:   event.TargetID,
					Status:     ModerationStatusPending,
					Verdict:    moderation.VerdictNeedsReview,
					Categories: []string{},
					CreatedAt:  now,
				}
			}
			if item.Status == "" {
				item.Status = ModerationStatusPending
			}
			if item.Verdict == "" {
				item.Verdict = moderation.VerdictNeedsReview
			}
			item.Reason = "User report: " + event.Reason
			item.ReportCount = count
			item.UpdatedAt = now
			if err := s.moderationRepo.UpsertQueueItem(ctx, *item); err != nil {
				logger.WithContext(ctx).Err(err).Error("Failed to persist report moderation queue item")
			}
			return count
		}
	}

	s.moderation.mu.Lock()
	defer s.moderation.mu.Unlock()

	if s.moderation.reports[id] == nil {
		s.moderation.reports[id] = make(map[string]bool)
	}
	s.moderation.reports[id][event.ReporterID] = true
	count := len(s.moderation.reports[id])

	item := s.moderation.items[id]
	if item == nil {
		item = &model.ModerationQueueItem{
			ID:         id,
			TargetType: event.TargetType,
			TargetID:   event.TargetID,
			Status:     ModerationStatusPending,
			Verdict:    moderation.VerdictNeedsReview,
			Categories: []string{},
			CreatedAt:  now,
		}
		s.moderation.items[id] = item
	}
	item.ReportCount = count
	if event.Reason != "" {
		item.Reason = "User report: " + event.Reason
	}
	item.UpdatedAt = now
	return count
}

func (s *AdminService) ListModerationQueue(ctx context.Context, filter ModerationQueueFilter) []model.ModerationQueueItem {
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Category = strings.ToUpper(strings.TrimSpace(filter.Category))

	if s.moderationRepo != nil {
		items, err := s.moderationRepo.ListQueue(ctx, filter.Status, filter.Category)
		if err != nil {
			logger.WithContext(ctx).Err(err).Error("Failed to list moderation queue from PostgreSQL")
		} else {
			return items
		}
	}

	s.moderation.mu.RLock()
	defer s.moderation.mu.RUnlock()

	items := make([]model.ModerationQueueItem, 0, len(s.moderation.items))
	for _, item := range s.moderation.items {
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		if filter.Category != "" && !hasCategory(item.Categories, filter.Category) {
			continue
		}
		items = append(items, *item)
	}
	return items
}

func hasCategory(categories []string, category string) bool {
	for _, current := range categories {
		if current == category {
			return true
		}
	}
	return false
}

func (s *AdminService) ApproveModerationItem(ctx context.Context, id, actorID, reason string) error {
	return s.updateModerationStatus(ctx, id, actorID, moderation.ActionAdminApprove, ModerationStatusApproved, reason, nil)
}

func (s *AdminService) HideModerationItem(ctx context.Context, id, actorID, reason string) error {
	return s.updateModerationStatus(ctx, id, actorID, moderation.ActionAdminHide, ModerationStatusHidden, reason, func(item *model.ModerationQueueItem) error {
		if item.TargetType == moderation.TargetPost {
			return s.repo.DeletePost(ctx, item.TargetID)
		}
		return nil
	})
}

func (s *AdminService) DeleteModerationItem(ctx context.Context, id, actorID, reason string) error {
	return s.updateModerationStatus(ctx, id, actorID, moderation.ActionAdminDelete, ModerationStatusDeleted, reason, func(item *model.ModerationQueueItem) error {
		if item.TargetType == moderation.TargetPost {
			return s.repo.DeletePost(ctx, item.TargetID)
		}
		return nil
	})
}

func (s *AdminService) SuspendModerationAuthor(ctx context.Context, id, actorID, reason string, duration time.Duration) error {
	return s.updateModerationStatus(ctx, id, actorID, moderation.ActionSuspendUser, ModerationStatusSuspended, reason, func(item *model.ModerationQueueItem) error {
		if item.AuthorID == "" {
			return nil
		}
		return s.repo.SuspendUser(ctx, item.AuthorID, duration)
	})
}

func (s *AdminService) updateModerationStatus(ctx context.Context, id, actorID, action, status, reason string, apply func(*model.ModerationQueueItem) error) error {
	if s.moderationRepo != nil {
		item, err := s.moderationRepo.GetQueueItem(ctx, id)
		if err != nil {
			return err
		}
		if item == nil {
			return nil
		}
		if apply != nil {
			if err := apply(item); err != nil {
				return err
			}
		}
		if _, err := s.moderationRepo.UpdateQueueStatus(ctx, id, status); err != nil {
			return err
		}
		s.recordAudit(ctx, actorID, action, item.TargetType, item.TargetID, reason)
		return nil
	}

	s.moderation.mu.RLock()
	item := s.moderation.items[id]
	s.moderation.mu.RUnlock()
	if item == nil {
		return nil
	}
	if apply != nil {
		if err := apply(item); err != nil {
			return err
		}
	}

	s.moderation.mu.Lock()
	item.Status = status
	item.UpdatedAt = time.Now()
	s.moderation.mu.Unlock()

	s.recordAudit(ctx, actorID, action, item.TargetType, item.TargetID, reason)
	return nil
}

func (s *AdminService) recordAudit(ctx context.Context, actorID, action, targetType, targetID, reason string) {
	audit := model.ModerationAuditLog{
		ID:         uuid.NewString(),
		ActorID:    actorID,
		Action:     action,
		TargetType: targetType,
		TargetID:   targetID,
		Reason:     reason,
		CreatedAt:  time.Now(),
	}
	if s.moderationRepo != nil {
		if err := s.moderationRepo.RecordAudit(ctx, audit); err != nil {
			logger.WithContext(ctx).Err(err).Error("Failed to persist moderation audit")
		} else {
			return
		}
	}

	s.moderation.mu.Lock()
	defer s.moderation.mu.Unlock()
	s.moderation.audits = append(s.moderation.audits, audit)
}
