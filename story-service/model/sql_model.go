package model

import (
	"time"

	"gorm.io/gorm"
)

// StoryEntity represents a 24-hour story in PostgreSQL
type StoryEntity struct {
	ID        string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	UserID    string         `gorm:"type:varchar(64);index:idx_story_user" json:"userId"`
	MediaURL  string         `gorm:"type:varchar(512)" json:"mediaUrl"`
	MediaType string         `gorm:"type:varchar(50);default:'IMAGE'" json:"mediaType"` // IMAGE, VIDEO
	Caption   string         `gorm:"type:text" json:"caption"`
	CreatedAt time.Time      `gorm:"autoCreateTime;index:idx_story_created" json:"createdAt"`
	ExpiresAt time.Time      `gorm:"index:idx_story_expires" json:"expiresAt"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (StoryEntity) TableName() string {
	return "stories"
}

// StoryViewEntity tracks who viewed a story
type StoryViewEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	StoryID   string    `gorm:"type:varchar(64);index:idx_story_viewer,unique" json:"storyId"`
	ViewerID  string    `gorm:"type:varchar(64);index:idx_story_viewer,unique" json:"viewerId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (StoryViewEntity) TableName() string {
	return "story_views"
}
