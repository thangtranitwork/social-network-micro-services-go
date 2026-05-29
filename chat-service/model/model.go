package model

import "time"

type Message struct {
	ID          string     `json:"id" bson:"_id"`
	ChatID      string     `json:"chatId" bson:"chat_id"`
	SenderID    string     `json:"senderId" bson:"sender_id"`
	RecipientID string     `json:"recipientId" bson:"recipient_id"`
	Content     string     `json:"content" bson:"content"`
	Timestamp   time.Time  `json:"timestamp" bson:"timestamp"`
	Type        string     `json:"type" bson:"type"`     // TEXT, FILE, GIF, VOICE, CALL
	Status      string     `json:"status" bson:"status"` // SENT, READ
	CallID      string     `json:"callId,omitempty" bson:"call_id,omitempty"`
	CallAt      *time.Time `json:"callAt,omitempty" bson:"call_at,omitempty"`
	AnswerAt    *time.Time `json:"answerAt,omitempty" bson:"answer_at,omitempty"`
	EndAt       *time.Time `json:"endAt,omitempty" bson:"end_at,omitempty"`
	IsAnswered  *bool      `json:"isAnswered,omitempty" bson:"is_answered,omitempty"`
	IsVideoCall *bool      `json:"isVideoCall,omitempty" bson:"is_video_call,omitempty"`
	IsRejected  *bool      `json:"isRejected,omitempty" bson:"is_rejected,omitempty"`
}
