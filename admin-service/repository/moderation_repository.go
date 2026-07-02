package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"social-network-go/admin-service/model"
	"social-network-go/internal/moderation"
)

type ModerationRepository interface {
	UpsertQueueItem(ctx context.Context, item model.ModerationQueueItem) error
	RecordReport(ctx context.Context, event moderation.ReportedEvent) (int, error)
	GetQueueItem(ctx context.Context, id string) (*model.ModerationQueueItem, error)
	ListQueue(ctx context.Context, status, category string) ([]model.ModerationQueueItem, error)
	UpdateQueueStatus(ctx context.Context, id, status string) (*model.ModerationQueueItem, error)
	RecordAudit(ctx context.Context, audit model.ModerationAuditLog) error
}

type GormModerationRepository struct {
	db *gorm.DB
}

func NewModerationRepository(db *gorm.DB) ModerationRepository {
	if db == nil {
		return nil
	}
	return &GormModerationRepository{db: db}
}

func (r *GormModerationRepository) UpsertQueueItem(ctx context.Context, item model.ModerationQueueItem) error {
	now := time.Now()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = now
	}
	record := model.NewModerationQueueRecord(item)
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"target_type":  gorm.Expr("EXCLUDED.target_type"),
			"target_id":    gorm.Expr("EXCLUDED.target_id"),
			"author_id":    gorm.Expr("COALESCE(NULLIF(EXCLUDED.author_id, ''), moderation_queue_items.author_id)"),
			"status":       gorm.Expr("EXCLUDED.status"),
			"verdict":      gorm.Expr("EXCLUDED.verdict"),
			"categories":   gorm.Expr("EXCLUDED.categories"),
			"confidence":   gorm.Expr("EXCLUDED.confidence"),
			"reason":       gorm.Expr("EXCLUDED.reason"),
			"report_count": gorm.Expr("GREATEST(EXCLUDED.report_count, moderation_queue_items.report_count)"),
			"updated_at":   gorm.Expr("EXCLUDED.updated_at"),
		}),
	}).Create(&record).Error
}

func (r *GormModerationRepository) RecordReport(ctx context.Context, event moderation.ReportedEvent) (int, error) {
	record := model.ModerationReportRecord{
		ID:         uuid.NewString(),
		TargetType: event.TargetType,
		TargetID:   event.TargetID,
		ReporterID: event.ReporterID,
		Reason:     event.Reason,
		CreatedAt:  event.OccurredAt,
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now()
	}

	err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "target_type"}, {Name: "target_id"}, {Name: "reporter_id"}},
		DoNothing: true,
	}).Create(&record).Error
	if err != nil {
		return 0, err
	}

	var count int64
	err = r.db.WithContext(ctx).Model(&model.ModerationReportRecord{}).
		Where("target_type = ? AND target_id = ?", event.TargetType, event.TargetID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (r *GormModerationRepository) GetQueueItem(ctx context.Context, id string) (*model.ModerationQueueItem, error) {
	var record model.ModerationQueueRecord
	err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item := record.ToQueueItem()
	return &item, nil
}

func (r *GormModerationRepository) ListQueue(ctx context.Context, status, category string) ([]model.ModerationQueueItem, error) {
	query := r.db.WithContext(ctx).Model(&model.ModerationQueueRecord{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if category = strings.ToUpper(strings.TrimSpace(category)); category != "" {
		query = query.Where("categories LIKE ?", "%\""+category+"\"%")
	}

	var records []model.ModerationQueueRecord
	if err := query.Order("updated_at DESC").Find(&records).Error; err != nil {
		return nil, err
	}

	items := make([]model.ModerationQueueItem, 0, len(records))
	for _, record := range records {
		items = append(items, record.ToQueueItem())
	}
	return items, nil
}

func (r *GormModerationRepository) UpdateQueueStatus(ctx context.Context, id, status string) (*model.ModerationQueueItem, error) {
	var record model.ModerationQueueRecord
	err := r.db.WithContext(ctx).First(&record, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.Status = status
	record.UpdatedAt = time.Now()
	if err := r.db.WithContext(ctx).Save(&record).Error; err != nil {
		return nil, err
	}
	item := record.ToQueueItem()
	return &item, nil
}

func (r *GormModerationRepository) RecordAudit(ctx context.Context, audit model.ModerationAuditLog) error {
	if audit.ID == "" {
		audit.ID = uuid.NewString()
	}
	if audit.CreatedAt.IsZero() {
		audit.CreatedAt = time.Now()
	}
	record := model.NewModerationAuditRecord(audit)
	return r.db.WithContext(ctx).Create(&record).Error
}
