package model

import (
	"encoding/json"
	"time"
)

type ModerationQueueItem struct {
	ID          string    `json:"id"`
	TargetType  string    `json:"targetType"`
	TargetID    string    `json:"targetId"`
	AuthorID    string    `json:"authorId"`
	Status      string    `json:"status"`
	Verdict     string    `json:"verdict"`
	Categories  []string  `json:"categories"`
	Confidence  float64   `json:"confidence"`
	Reason      string    `json:"reason"`
	ReportCount int       `json:"reportCount"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type ModerationAuditLog struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actorId"`
	Action     string    `json:"action"`
	TargetType string    `json:"targetType"`
	TargetID   string    `json:"targetId"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"createdAt"`
}

type ModerationQueueRecord struct {
	ID             string `gorm:"primaryKey;size:128"`
	TargetType     string `gorm:"size:32;index:idx_moderation_target,priority:1;index"`
	TargetID       string `gorm:"size:128;index:idx_moderation_target,priority:2;index"`
	AuthorID       string `gorm:"size:128;index"`
	Status         string `gorm:"size:32;index"`
	Verdict        string `gorm:"size:32;index"`
	CategoriesJSON string `gorm:"column:categories;type:text"`
	Confidence     float64
	Reason         string `gorm:"type:text"`
	ReportCount    int
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (ModerationQueueRecord) TableName() string {
	return "moderation_queue_items"
}

func NewModerationQueueRecord(item ModerationQueueItem) ModerationQueueRecord {
	categories, _ := json.Marshal(item.Categories)
	return ModerationQueueRecord{
		ID:             item.ID,
		TargetType:     item.TargetType,
		TargetID:       item.TargetID,
		AuthorID:       item.AuthorID,
		Status:         item.Status,
		Verdict:        item.Verdict,
		CategoriesJSON: string(categories),
		Confidence:     item.Confidence,
		Reason:         item.Reason,
		ReportCount:    item.ReportCount,
		CreatedAt:      item.CreatedAt,
		UpdatedAt:      item.UpdatedAt,
	}
}

func (r ModerationQueueRecord) ToQueueItem() ModerationQueueItem {
	var categories []string
	_ = json.Unmarshal([]byte(r.CategoriesJSON), &categories)
	if categories == nil {
		categories = []string{}
	}
	return ModerationQueueItem{
		ID:          r.ID,
		TargetType:  r.TargetType,
		TargetID:    r.TargetID,
		AuthorID:    r.AuthorID,
		Status:      r.Status,
		Verdict:     r.Verdict,
		Categories:  categories,
		Confidence:  r.Confidence,
		Reason:      r.Reason,
		ReportCount: r.ReportCount,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

type ModerationReportRecord struct {
	ID         string `gorm:"primaryKey;size:128"`
	TargetType string `gorm:"size:32;uniqueIndex:idx_unique_moderation_report,priority:1;index"`
	TargetID   string `gorm:"size:128;uniqueIndex:idx_unique_moderation_report,priority:2;index"`
	ReporterID string `gorm:"size:128;uniqueIndex:idx_unique_moderation_report,priority:3;index"`
	Reason     string `gorm:"type:text"`
	CreatedAt  time.Time
}

func (ModerationReportRecord) TableName() string {
	return "moderation_reports"
}

type ModerationAuditRecord struct {
	ID         string    `gorm:"primaryKey;size:128"`
	ActorID    string    `gorm:"size:128;index"`
	Action     string    `gorm:"size:64;index"`
	TargetType string    `gorm:"size:32;index:idx_moderation_audit_target,priority:1"`
	TargetID   string    `gorm:"size:128;index:idx_moderation_audit_target,priority:2"`
	Reason     string    `gorm:"type:text"`
	CreatedAt  time.Time `gorm:"index"`
}

func (ModerationAuditRecord) TableName() string {
	return "moderation_audit_logs"
}

func NewModerationAuditRecord(audit ModerationAuditLog) ModerationAuditRecord {
	return ModerationAuditRecord{
		ID:         audit.ID,
		ActorID:    audit.ActorID,
		Action:     audit.Action,
		TargetType: audit.TargetType,
		TargetID:   audit.TargetID,
		Reason:     audit.Reason,
		CreatedAt:  audit.CreatedAt,
	}
}
