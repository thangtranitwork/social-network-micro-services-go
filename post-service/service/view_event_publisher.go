package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
	"social-network-go/post-service/dto"
)

type QualifiedViewEventMessage struct {
	EventID           string    `json:"eventId"`
	UserID            string    `json:"userId"`
	PostID            string    `json:"postId"`
	ObservedAt        time.Time `json:"observedAt"`
	VisibleDurationMs int       `json:"visibleDurationMs"`
	VisibleRatio      float64   `json:"visibleRatio"`
	Surface           string    `json:"surface"`
	SchemaVersion     int       `json:"schemaVersion"`
	IngestedAt        time.Time `json:"ingestedAt"`
	TraceID           string    `json:"traceId,omitempty"`
	RequestID         string    `json:"requestId,omitempty"`
}

type ViewEventPublisher interface {
	PublishViewEvents(ctx context.Context, userID string, events []dto.ViewEventDTO) error
	Close() error
}

type KafkaViewEventPublisher struct {
	writer *kafka.Writer
}

func NewKafkaViewEventPublisher(kafkaAddr string, topic string) *KafkaViewEventPublisher {
	if topic == "" {
		topic = "post-view-events"
	}
	return &KafkaViewEventPublisher{
		writer: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        topic,
			Balancer:     &kafka.Hash{},
			Async:        false, // Synchronous write to guarantee broker acknowledgement
			RequiredAcks: kafka.RequireOne,
			WriteTimeout: 3 * time.Second,
		},
	}
}

func (p *KafkaViewEventPublisher) PublishViewEvents(ctx context.Context, userID string, events []dto.ViewEventDTO) error {
	if p == nil || p.writer == nil {
		return nil
	}
	if len(events) == 0 {
		return nil
	}

	traceID, requestID := traceIDsFromContext(ctx)
	ingestedAt := time.Now().UTC()

	msgs := make([]kafka.Message, 0, len(events))
	for _, event := range events {
		msgPayload := QualifiedViewEventMessage{
			EventID:           event.EventID,
			UserID:            userID,
			PostID:            event.PostID,
			ObservedAt:        event.ObservedAt,
			VisibleDurationMs: event.VisibleDurationMs,
			VisibleRatio:      event.VisibleRatio,
			Surface:           event.Surface,
			SchemaVersion:     event.SchemaVersion,
			IngestedAt:        ingestedAt,
			TraceID:           traceID,
			RequestID:         requestID,
		}

		bytes, err := json.Marshal(msgPayload)
		if err != nil {
			logger.WithContext(ctx).Err(err).Error("failed to serialize qualified view event")
			return err
		}

		msgs = append(msgs, kafka.Message{
			Key:   []byte(userID),
			Value: bytes,
			Time:  event.ObservedAt,
		})
	}

	err := p.writer.WriteMessages(ctx, msgs...)
	if err != nil {
		logger.WithContext(ctx).Err(err).Error("failed to publish view events to kafka with ack")
		return err
	}

	return nil
}

func (p *KafkaViewEventPublisher) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}
