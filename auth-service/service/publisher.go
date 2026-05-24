package service

import (
	"context"
	"encoding/json"
	"time"

	"social-network-go/logger"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

type UserEventPublisher interface {
	PublishAccountCreated(ctx context.Context, accountID uuid.UUID, email, givenName, familyName, birthdate string) error
	PublishAccountVerified(ctx context.Context, accountID uuid.UUID) error
	Close() error
}

type KafkaUserEventPublisher struct {
	writer *kafka.Writer
}

func NewKafkaUserEventPublisher(kafkaAddr string) UserEventPublisher {
	w := &kafka.Writer{
		Addr:     kafka.TCP(kafkaAddr),
		Topic:    "user-events",
		Balancer: &kafka.LeastBytes{},
	}
	return &KafkaUserEventPublisher{writer: w}
}

func (p *KafkaUserEventPublisher) PublishAccountCreated(ctx context.Context, accountID uuid.UUID, email, givenName, familyName, birthdate string) error {
	event := map[string]interface{}{
		"event":       "AccountCreated",
		"account_id":  accountID.String(),
		"email":       email,
		"given_name":  givenName,
		"family_name": familyName,
		"birthdate":   birthdate,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	go func() {
		publishCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := p.writer.WriteMessages(publishCtx, kafka.Message{
			Key:   []byte(accountID.String()),
			Value: payload,
		}); err != nil {
			logger.Field("account_id", accountID).Field("error", err).Warn("Kafka failed to publish AccountCreated event")
		}
	}()

	return nil
}

func (p *KafkaUserEventPublisher) PublishAccountVerified(ctx context.Context, accountID uuid.UUID) error {
	event := map[string]interface{}{
		"event":      "AccountVerified",
		"account_id": accountID.String(),
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	go func() {
		publishCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := p.writer.WriteMessages(publishCtx, kafka.Message{
			Key:   []byte(accountID.String()),
			Value: payload,
		}); err != nil {
			logger.Field("account_id", accountID).Field("error", err).Warn("Kafka failed to publish AccountVerified event")
		}
	}()

	return nil
}

func (p *KafkaUserEventPublisher) Close() error {
	return p.writer.Close()
}
