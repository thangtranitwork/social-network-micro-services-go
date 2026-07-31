package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
)

type PostKeywordEvent struct {
	Event     string    `json:"event"`
	PostID    string    `json:"postId"`
	Content   string    `json:"content"`
	IsUpdate  bool      `json:"isUpdate"`
	AuthorID  string    `json:"authorId,omitempty"`
	TraceID   string    `json:"traceId,omitempty"`
	RequestID string    `json:"requestId,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type KafkaKeywordPublisher struct {
	writer *kafka.Writer
}

func NewKafkaKeywordPublisher(kafkaAddr string) *KafkaKeywordPublisher {
	return &KafkaKeywordPublisher{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        "post-events",
			Balancer:     &kafka.LeastBytes{},
			Async:        true,
			BatchSize:    100,
			BatchTimeout: 50 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
		},
	}
}

func (p *KafkaKeywordPublisher) ExtractPostKeywords(ctx context.Context, postID string, content string, isUpdate bool) error {
	traceID, requestID := traceIDsFromContext(ctx)
	eventName := "post_created"
	if isUpdate {
		eventName = "post_updated"
	}
	event := PostKeywordEvent{
		Event:     eventName,
		PostID:    postID,
		Content:   content,
		IsUpdate:  isUpdate,
		TraceID:   traceID,
		RequestID: requestID,
		Timestamp: time.Now(),
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:     []byte(postID),
		Value:   payload,
		Headers: kafkaTraceHeaders(traceID, requestID),
	})
	if err != nil {
		logger.WithContext(ctx).Err(err).Error("Failed to publish keyword extraction event")
	}
	return err
}

func (p *KafkaKeywordPublisher) Interact(ctx context.Context, postID string, actorID string, score string) error {
	return nil
}

func (p *KafkaKeywordPublisher) Close() error {
	if p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
