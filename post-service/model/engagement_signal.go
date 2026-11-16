package model

import "time"

const (
	NegativeSignalHidePost      = "hide_post"
	NegativeSignalNotInterested = "not_interested"
	NegativeSignalQuickSkip     = "quick_skip"
	NegativeSignalMuteAuthor    = "mute_author"
	NegativeSignalReport        = "report"
)

// PostNegativeSignalEntity persists explicit or implicit negative feedback from a user.
type PostNegativeSignalEntity struct {
	ID         string    `gorm:"primaryKey;size:128" json:"id"`
	UserID     string    `gorm:"size:128;not null;index" json:"userId"`
	PostID     string    `gorm:"size:128;not null;index" json:"postId"`
	AuthorID   string    `gorm:"size:128;index" json:"authorId"`
	SignalType string    `gorm:"size:32;not null;index" json:"signalType"`
	CreatedAt  time.Time `gorm:"index" json:"createdAt"`
}

func (PostNegativeSignalEntity) TableName() string {
	return "post_negative_signals"
}

type MediaWatchEventRequest struct {
	PostID          string  `json:"postId" binding:"required"`
	AuthorID        string  `json:"authorId"`
	MediaID         string  `json:"mediaId"`
	MediaType       string  `json:"mediaType"` // video | audio
	WatchDurationMs int     `json:"watchDurationMs"`
	TotalDurationMs int     `json:"totalDurationMs"`
	ProgressRatio   float64 `json:"progressRatio"` // 0.0 - 1.0
	Muted           bool    `json:"muted"`
	ReplayCount     int     `json:"replayCount"`
}

type NegativeSignalRequest struct {
	PostID     string `json:"postId" binding:"required"`
	AuthorID   string `json:"authorId"`
	SignalType string `json:"signalType" binding:"required"`
}
