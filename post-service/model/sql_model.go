package model

import (
	"time"

	"gorm.io/gorm"
)

// PostEntity represents a post stored in PostgreSQL
type PostEntity struct {
	ID           string           `gorm:"primaryKey;type:varchar(64)" json:"id"`
	UserID       string           `gorm:"type:varchar(64);index:idx_post_user" json:"userId"`
	Content      string           `gorm:"type:text" json:"content"`
	Privacy      string           `gorm:"type:varchar(20);default:'PUBLIC'" json:"privacy"` // PUBLIC, FRIEND, PRIVATE
	LikeCount    int              `gorm:"default:0" json:"likeCount"`
	CommentCount int              `gorm:"default:0" json:"commentCount"`
	ShareCount   int              `gorm:"default:0" json:"shareCount"`
	OriginalID   string           `gorm:"type:varchar(64)" json:"originalId,omitempty"` // For shared posts
	CreatedAt    time.Time        `gorm:"autoCreateTime;index:idx_post_created" json:"createdAt"`
	UpdatedAt    time.Time        `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt    gorm.DeletedAt   `gorm:"index" json:"-"`
	MediaFiles   []PostMediaEntity `gorm:"foreignKey:PostID;constraint:OnDelete:CASCADE" json:"mediaFiles"`
}

func (PostEntity) TableName() string {
	return "posts"
}

// PostMediaEntity represents attached images or files for a post
type PostMediaEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID    string    `gorm:"type:varchar(64);index:idx_media_post" json:"postId"`
	FileID    string    `gorm:"type:varchar(255)" json:"fileId"`
	MediaURL  string    `gorm:"type:varchar(512)" json:"mediaUrl"`
	MediaType string    `gorm:"type:varchar(50)" json:"mediaType"` // IMAGE, VIDEO, FILE
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (PostMediaEntity) TableName() string {
	return "post_media"
}

// CommentEntity represents a post comment
type CommentEntity struct {
	ID         string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	PostID     string         `gorm:"type:varchar(64);index:idx_comment_post" json:"postId"`
	UserID     string         `gorm:"type:varchar(64);index:idx_comment_user" json:"userId"`
	ParentID   string         `gorm:"type:varchar(64);index:idx_comment_parent" json:"parentId,omitempty"`
	Content    string         `gorm:"type:text" json:"content"`
	LikeCount  int            `gorm:"default:0" json:"likeCount"`
	ReplyCount int            `gorm:"default:0" json:"replyCount"`
	CreatedAt  time.Time      `gorm:"autoCreateTime;index:idx_comment_created" json:"createdAt"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (CommentEntity) TableName() string {
	return "comments"
}

// PostLikeEntity represents a like reaction on a post
type PostLikeEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID    string    `gorm:"type:varchar(64);index:idx_post_like,unique" json:"postId"`
	UserID    string    `gorm:"type:varchar(64);index:idx_post_like,unique" json:"userId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (PostLikeEntity) TableName() string {
	return "post_likes"
}

// CommentLikeEntity represents a like reaction on a comment
type CommentLikeEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	CommentID string    `gorm:"type:varchar(64);index:idx_comment_like,unique" json:"commentId"`
	UserID    string    `gorm:"type:varchar(64);index:idx_comment_like,unique" json:"userId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (CommentLikeEntity) TableName() string {
	return "comment_likes"
}

// PostShareEntity represents a share action on a post
type PostShareEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	PostID    string    `gorm:"type:varchar(64);index:idx_post_share" json:"postId"`
	UserID    string    `gorm:"type:varchar(64);index:idx_user_share" json:"userId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (PostShareEntity) TableName() string {
	return "post_shares"
}
