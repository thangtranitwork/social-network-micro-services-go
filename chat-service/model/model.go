package model

import "time"

type Message struct {
	ID          string    `json:"id" bson:"_id"`
	ChatID      string    `json:"chatId" bson:"chat_id"`
	SenderID    string    `json:"senderId" bson:"sender_id"`
	RecipientID string    `json:"recipientId" bson:"recipient_id"`
	Content     string    `json:"content" bson:"content"`
	Timestamp   time.Time `json:"timestamp" bson:"timestamp"`
	Type        string    `json:"type" bson:"type"` // TEXT, FILE, GIF, VOICE
	Status      string    `json:"status" bson:"status"` // SENT, READ
}
