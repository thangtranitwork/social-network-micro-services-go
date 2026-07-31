package model

import (
	"time"

	"gorm.io/gorm"
)

// UserEntity represents the SQL table model for user profiles
type UserEntity struct {
	ID                      string         `gorm:"primaryKey;type:varchar(64)" json:"id"`
	Email                   string         `gorm:"type:varchar(255);uniqueIndex" json:"email"`
	GivenName               string         `gorm:"type:varchar(64)" json:"givenName"`
	FamilyName              string         `gorm:"type:varchar(64)" json:"familyName"`
	Username                string         `gorm:"type:varchar(32);uniqueIndex" json:"username"`
	Bio                     string         `gorm:"type:text" json:"bio"`
	Birthdate               time.Time      `json:"birthdate"`
	ProfilePictureID        string         `gorm:"type:varchar(255)" json:"profilePictureId"`
	EmailNotifications      bool           `gorm:"default:true" json:"emailNotifications"`
	PushNotifications       bool           `gorm:"default:true" json:"pushNotifications"`
	DigestFrequency         string         `gorm:"type:varchar(20);default:'DAILY'" json:"digestFrequency"`
	NextChangeNameDate      time.Time      `json:"nextChangeNameDate"`
	NextChangeBirthdateDate time.Time      `json:"nextChangeBirthdateDate"`
	NextChangeUsernameDate  time.Time      `json:"nextChangeUsernameDate"`
	CreatedAt               time.Time      `gorm:"autoCreateTime" json:"createdAt"`
	UpdatedAt               time.Time      `gorm:"autoUpdateTime" json:"updatedAt"`
	DeletedAt               gorm.DeletedAt `gorm:"index" json:"-"`
}

func (UserEntity) TableName() string {
	return "users"
}

// FriendEntity represents a bidirectional friendship between two users
type FriendEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    string    `gorm:"type:varchar(64);index:idx_user_friend,unique" json:"userId"`
	FriendID  string    `gorm:"type:varchar(64);index:idx_user_friend,unique" json:"friendId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (FriendEntity) TableName() string {
	return "friends"
}

// FriendRequestEntity represents a pending/processed friend request
type FriendRequestEntity struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	SenderID   string    `gorm:"type:varchar(64);index:idx_sender_receiver,unique" json:"senderId"`
	ReceiverID string    `gorm:"type:varchar(64);index:idx_sender_receiver,unique;index:idx_receiver_status" json:"receiverId"`
	Status     string    `gorm:"type:varchar(20);default:'PENDING';index:idx_receiver_status" json:"status"` // PENDING, ACCEPTED, DECLINED
	CreatedAt  time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (FriendRequestEntity) TableName() string {
	return "friend_requests"
}

// UserBlockEntity represents a user blocking another user
type UserBlockEntity struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	BlockerID string    `gorm:"type:varchar(64);index:idx_blocker_blocked,unique" json:"blockerId"`
	BlockedID string    `gorm:"type:varchar(64);index:idx_blocker_blocked,unique" json:"blockedId"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"createdAt"`
}

func (UserBlockEntity) TableName() string {
	return "user_blocks"
}
