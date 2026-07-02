package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/internal/moderation"
	"social-network-go/logger"
)

type ModerationPublisher interface {
	RequestReview(ctx context.Context, event moderation.RequestEvent) error
	Report(ctx context.Context, event moderation.ReportedEvent) error
	Close() error
}

type KafkaModerationPublisher struct {
	requestWriter *kafka.Writer
	reportWriter  *kafka.Writer
}

func NewKafkaModerationPublisher(kafkaAddr string) *KafkaModerationPublisher {
	return &KafkaModerationPublisher{
		requestWriter: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        moderation.TopicRequested,
			Balancer:     &kafka.LeastBytes{},
			Async:        true,
			BatchSize:    100,
			BatchTimeout: 50 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
		},
		reportWriter: &kafka.Writer{
			Addr:         kafka.TCP(kafkaAddr),
			Topic:        moderation.TopicReported,
			Balancer:     &kafka.LeastBytes{},
			Async:        true,
			BatchSize:    100,
			BatchTimeout: 50 * time.Millisecond,
			WriteTimeout: 500 * time.Millisecond,
		},
	}
}

func (p *KafkaModerationPublisher) RequestReview(ctx context.Context, event moderation.RequestEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	event.TraceID, event.RequestID = traceIDsFromContext(ctx)
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = p.requestWriter.WriteMessages(publishCtx, kafka.Message{
		Key:     []byte(event.TargetID),
		Value:   payload,
		Headers: kafkaTraceHeaders(event.TraceID, event.RequestID),
	})
	if err != nil {
		logger.Err(err).Error("Failed to publish moderation request event")
	}
	return err
}

func (p *KafkaModerationPublisher) Report(ctx context.Context, event moderation.ReportedEvent) error {
	if event.OccurredAt.IsZero() {
		event.OccurredAt = time.Now()
	}
	event.TraceID, event.RequestID = traceIDsFromContext(ctx)
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	err = p.reportWriter.WriteMessages(publishCtx, kafka.Message{
		Key:     []byte(event.TargetID),
		Value:   payload,
		Headers: kafkaTraceHeaders(event.TraceID, event.RequestID),
	})
	if err != nil {
		logger.Err(err).Error("Failed to publish content reported event")
	}
	return err
}

func traceIDsFromContext(ctx context.Context) (string, string) {
	var traceID, requestID string
	if ctx != nil {
		traceID, _ = ctx.Value("trace_id").(string)
		requestID, _ = ctx.Value("request_id").(string)
	}
	return traceID, requestID
}

func kafkaTraceHeaders(traceID, requestID string) []kafka.Header {
	headers := make([]kafka.Header, 0, 2)
	if traceID != "" {
		headers = append(headers, kafka.Header{Key: "X-Trace-ID", Value: []byte(traceID)})
	}
	if requestID != "" {
		headers = append(headers, kafka.Header{Key: "X-Request-ID", Value: []byte(requestID)})
	}
	return headers
}

func (p *KafkaModerationPublisher) Close() error {
	if err := p.requestWriter.Close(); err != nil {
		return err
	}
	return p.reportWriter.Close()
}
