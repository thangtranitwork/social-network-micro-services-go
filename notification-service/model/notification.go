package model

import "time"

type Notification struct {
	ID               string      `json:"id"`
	Action           string      `json:"action"`
	TargetType       string      `json:"targetType"`
	TargetID         string      `json:"targetId"`
	PostID           string      `json:"postId,omitempty"`
	CommentID        string      `json:"commentId,omitempty"`
	RepliedCommentID string      `json:"repliedCommentId,omitempty"`
	Username         string      `json:"username"`
	ShortenedContent string      `json:"shortenedContent"`
	Creator          CreatorInfo `json:"creator"`
	SentAt           time.Time   `json:"sentAt"`
	IsRead           bool        `json:"isRead"`
}

type CreatorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type NotificationKafkaEvent struct {
	Type             string `json:"type"` // "SINGLE" or "FRIENDS"
	Action           string `json:"action"`
	CreatorID        string `json:"creatorId"`
	ReceiverID       string `json:"receiverId,omitempty"`
	TargetID         string `json:"targetId"`
	TargetType       string `json:"targetType"`
	ShortenedContent string `json:"shortenedContent"`
}

type FCMTokenRequest struct {
	UserID     string `json:"userId"`
	FCMToken   string `json:"fcmToken" binding:"required"`
	DeviceType string `json:"deviceType"`
}
