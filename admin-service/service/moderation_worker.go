package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/segmentio/kafka-go"
	"social-network-go/internal/moderation"
	"social-network-go/logger"
)

func (s *AdminService) StartModerationWorker(kafkaAddr string) {
	s.startCompletedWorker(kafkaAddr)
	s.startReportedWorker(kafkaAddr)
}

func (s *AdminService) startCompletedWorker(kafkaAddr string) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{kafkaAddr},
		GroupID:  "admin-moderation-completed-group",
		Topic:    moderation.TopicCompleted,
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	go func() {
		defer reader.Close()
		for {
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				logger.Error("admin moderation completed worker read error: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}
			var event moderation.CompletedEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				logger.Error("admin moderation completed worker unmarshal error: %v", err)
				continue
			}
			s.UpsertModerationResult(context.Background(), event)
		}
	}()
}

func (s *AdminService) startReportedWorker(kafkaAddr string) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{kafkaAddr},
		GroupID:  "admin-moderation-reported-group",
		Topic:    moderation.TopicReported,
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	go func() {
		defer reader.Close()
		for {
			msg, err := reader.ReadMessage(context.Background())
			if err != nil {
				logger.Error("admin moderation report worker read error: %v", err)
				time.Sleep(3 * time.Second)
				continue
			}
			var event moderation.ReportedEvent
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				logger.Error("admin moderation report worker unmarshal error: %v", err)
				continue
			}
			s.RecordReport(context.Background(), event)
		}
	}()
}
