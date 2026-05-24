package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

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
		Addr:     kafka.TCP(kafkaAddr),
		Topic:    "notification-events",
		Balancer: &kafka.LeastBytes{},
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
		return err
	}

	log.Printf("Publishing SINGLE notification event to Kafka: %s", string(payload))

	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(receiverID),
		Value: payload,
	})
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
		return err
	}

	log.Printf("Publishing FRIENDS notification event to Kafka: %s", string(payload))

	publishCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	return p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(creatorID),
		Value: payload,
	})
}

func (p *KafkaNotificationPublisher) Close() error {
	return p.writer.Close()
}
