package service

import (
	"context"
	"encoding/json"
	"time"

	"social-network-go/logger"

	"github.com/segmentio/kafka-go"
)

type NotificationEvent struct {
	Type             string `json:"type"` // "SINGLE" or "FRIENDS"
	Action           string `json:"action"`
	CreatorID        string `json:"creatorId"`
	ReceiverID       string `json:"receiverId,omitempty"`
	TargetID         string `json:"targetId"`
	TargetType       string `json:"targetType"`
	ShortenedContent string `json:"shortenedContent"`
}

type KafkaNotificationPublisher struct {
	writer *kafka.Writer
}

func NewKafkaNotificationPublisher(kafkaAddr string) *KafkaNotificationPublisher {
	w := &kafka.Writer{
		Addr:         kafka.TCP(kafkaAddr),
		Topic:        "notification-events",
		Balancer:     &kafka.LeastBytes{},
		Async:        true,
		BatchSize:    100,
		BatchTimeout: 50 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
	}
	return &KafkaNotificationPublisher{writer: w}
}

func (p *KafkaNotificationPublisher) Send(ctx context.Context, action string, creatorID string, receiverID string, targetID string, targetType string, shortenedContent string) error {
	event := NotificationEvent{
		Type:             "SINGLE",
		Action:           action,
		CreatorID:        creatorID,
		ReceiverID:       receiverID,
		TargetID:         targetID,
		TargetType:       targetType,
		ShortenedContent: shortenedContent,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		logger.Err(err).Error("Failed to marshal SINGLE notification event")
		return err
	}

	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	err = p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(receiverID),
		Value: payload,
	})
	if err != nil {
		logger.Err(err).Error("Failed to publish SINGLE notification event to Kafka")
	}
	return err
}

func (p *KafkaNotificationPublisher) SendToFriends(ctx context.Context, action string, creatorID string, targetID string, targetType string, shortenedContent string) error {
	event := NotificationEvent{
		Type:             "FRIENDS",
		Action:           action,
		CreatorID:        creatorID,
		TargetID:         targetID,
		TargetType:       targetType,
		ShortenedContent: shortenedContent,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		logger.Err(err).Error("Failed to marshal FRIENDS notification event")
		return err
	}

	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	err = p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(creatorID),
		Value: payload,
	})
	if err != nil {
		logger.Err(err).Error("Failed to publish FRIENDS notification event to Kafka")
	}
	return err
}

func (p *KafkaNotificationPublisher) Close() error {
	return p.writer.Close()
}
