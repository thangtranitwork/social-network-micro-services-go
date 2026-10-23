package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"social-network-go/internal/events"
	"social-network-go/logger"
	"social-network-go/search-service/config"
	"social-network-go/search-service/model"
	"social-network-go/search-service/repository"

	"github.com/segmentio/kafka-go"
)

type PostImageConsumer struct {
	cfg    *config.Config
	esRepo *repository.ESRepository
}

func NewPostImageConsumer(cfg *config.Config, esRepo *repository.ESRepository) *PostImageConsumer {
	return &PostImageConsumer{
		cfg:    cfg,
		esRepo: esRepo,
	}
}

// Start launches the Kafka reader listening on post-image-events topic
func (c *PostImageConsumer) Start(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{c.cfg.KafkaAddr},
		Topic:          events.TopicPostImageEvents,
		GroupID:        "search-service-post-image-sync-group",
		MinBytes:       1,
		MaxBytes:       10e6, // 10MB
		CommitInterval: 1 * time.Second,
	})

	logger.Info("Search Service: Kafka Consumer started on topic: %s", events.TopicPostImageEvents)

	go func() {
		defer reader.Close()
		for {
			select {
			case <-ctx.Done():
				logger.Info("Search Service: Stopping post-image-events consumer")
				return
			default:
				msg, err := reader.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					logger.Err(err).Warn("Error reading message from topic %s", events.TopicPostImageEvents)
					time.Sleep(2 * time.Second)
					continue
				}

				var event events.PostImageEmbeddedEvent
				if err := json.Unmarshal(msg.Value, &event); err != nil {
					logger.Err(err).Warn("Failed to unmarshal PostImageEmbeddedEvent payload")
					continue
				}

				syncCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

				switch event.EventType {
				case events.EventTypePostImageEmbedded:
					doc := &model.PostImageDoc{
						ID:             fmt.Sprintf("%s_%s", event.PostID, event.FileID),
						PostID:         event.PostID,
						FileID:         event.FileID,
						AuthorID:       event.AuthorID,
						Privacy:        event.Privacy,
						EmbeddingModel: event.EmbeddingModel,
						ModelVersion:   event.ModelVersion,
						ImageVector:    event.Vector,
						CreatedAt:      event.CreatedAt,
					}
					if err := c.esRepo.IndexPostImage(syncCtx, doc); err != nil {
						logger.WithContext(syncCtx).Err(err).Error("Failed to index post image vector into ES for post %s, file %s", event.PostID, event.FileID)
					} else {
						logger.WithContext(syncCtx).Info("ES indexed image vector: post %s, file %s", event.PostID, event.FileID)
					}

				case events.EventTypePostImageDeleted:
					if event.FileID != "" {
						if err := c.esRepo.DeletePostImageByFile(syncCtx, event.PostID, event.FileID); err != nil {
							logger.WithContext(syncCtx).Err(err).Warn("Failed to delete post image vector for post %s, file %s", event.PostID, event.FileID)
						}
					}

				case events.EventTypePostImagesCleared:
					if event.PostID != "" {
						if err := c.esRepo.DeletePostImagesByPost(syncCtx, event.PostID); err != nil {
							logger.WithContext(syncCtx).Err(err).Warn("Failed to clear post image vectors for post %s", event.PostID)
						}
					}
				}
				cancel()
			}
		}
	}()
}
