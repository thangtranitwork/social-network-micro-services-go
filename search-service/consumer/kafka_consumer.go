package consumer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/search-service/config"
	"social-network-go/search-service/db"
	"social-network-go/search-service/repository"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/segmentio/kafka-go"
)

type SearchEventConsumer struct {
	cfg       *config.Config
	esRepo    *repository.ESRepository
	dlqWriter *kafka.Writer
}

func NewSearchEventConsumer(cfg *config.Config, esRepo *repository.ESRepository) *SearchEventConsumer {
	var dlqWriter *kafka.Writer
	if cfg.KafkaAddr != "" {
		dlqWriter = &kafka.Writer{
			Addr:         kafka.TCP(cfg.KafkaAddr),
			Topic:        "search-events-dlq",
			Balancer:     &kafka.LeastBytes{},
			BatchTimeout: 10 * time.Millisecond,
		}
	}
	return &SearchEventConsumer{
		cfg:       cfg,
		esRepo:    esRepo,
		dlqWriter: dlqWriter,
	}
}

// Close gracefully closes the DLQ writer
func (c *SearchEventConsumer) Close() error {
	if c.dlqWriter != nil {
		return c.dlqWriter.Close()
	}
	return nil
}

type DLQMessage struct {
	SourceTopic string    `json:"source_topic"`
	Key         string    `json:"key"`
	Payload     string    `json:"payload"`
	Error       string    `json:"error"`
	FailedAt    time.Time `json:"failed_at"`
}

func executeWithRetry(ctx context.Context, maxRetries int, initialInterval time.Duration, op func() error) error {
	var err error
	interval := initialInterval
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err = op()
		if err == nil {
			return nil
		}
		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
				interval *= 2
			}
		}
	}
	return err
}

func (c *SearchEventConsumer) publishDLQ(ctx context.Context, sourceTopic, key string, rawPayload []byte, failureErr error) {
	if c.dlqWriter == nil || failureErr == nil {
		return
	}
	dlqMsg := DLQMessage{
		SourceTopic: sourceTopic,
		Key:         key,
		Payload:     string(rawPayload),
		Error:       failureErr.Error(),
		FailedAt:    time.Now().UTC(),
	}
	data, err := json.Marshal(dlqMsg)
	if err != nil {
		logger.WithContext(ctx).Err(err).Error("Failed to marshal DLQ message")
		return
	}

	dlqCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := c.dlqWriter.WriteMessages(dlqCtx, kafka.Message{
		Key:   []byte(key),
		Value: data,
	}); err != nil {
		logger.WithContext(ctx).Err(err).Error("CRITICAL: Failed to publish message to search-events-dlq: %v", err)
	} else {
		logger.WithContext(ctx).Warn("Published failed event (key: %s) to DLQ topic 'search-events-dlq'", key)
	}
}

// Start initiates background consumers for post-events and user-events
func (c *SearchEventConsumer) Start(ctx context.Context) {
	go c.consumePostEvents(ctx)
	go c.consumeUserEvents(ctx)
}

func (c *SearchEventConsumer) consumePostEvents(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{c.cfg.KafkaAddr},
		Topic:          "post-events",
		GroupID:        "search-service-post-sync-group",
		MinBytes:       10e3, // 10KB
		MaxBytes:       10e6, // 10MB
		CommitInterval: 1 * time.Second,
	})
	defer reader.Close()

	logger.Info("Search Service: Kafka Consumer started on topic: post-events")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Search Service: Stopping post-events consumer")
			return
		default:
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Err(err).Warn("Error reading post-events message in Search Service")
				time.Sleep(2 * time.Second)
				continue
			}

			var event struct {
				Event     string    `json:"event"`
				PostID    string    `json:"postId"`
				AuthorID  string    `json:"authorId"`
				Content   string    `json:"content"`
				IsUpdate  bool      `json:"isUpdate"`
				Privacy   string    `json:"privacy"`
				Files     []string  `json:"files"`
				Timestamp time.Time `json:"timestamp"`
			}

			if err := json.Unmarshal(msg.Value, &event); err != nil {
				logger.Err(err).Warn("Failed to unmarshal post-events payload")
				continue
			}

			postID := event.PostID
			if postID == "" {
				postID = string(msg.Key)
			}
			if postID == "" {
				continue
			}

			syncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)

			// If event came from keyword extraction without authorID, enrich from Neo4j
			if event.AuthorID == "" && db.Neo4jDriver != nil {
				session := db.Neo4jDriver.NewSession(syncCtx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
				resData, err := session.ExecuteRead(syncCtx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
					res, err := tx.Run(syncCtx, "MATCH (a:User)-[:POSTED]->(p:Post {id: $id}) RETURN a.id, p.privacy, p.files", map[string]interface{}{"id": postID})
					if err != nil || !res.Next(syncCtx) {
						return nil, err
					}
					return res.Record().Values, nil
				})
				session.Close(syncCtx)
				if err == nil && resData != nil {
					vals := resData.([]interface{})
					if len(vals) > 0 && vals[0] != nil {
						event.AuthorID = fmt.Sprintf("%v", vals[0])
					}
					if len(vals) > 1 && vals[1] != nil {
						if priv := fmt.Sprintf("%v", vals[1]); priv != "" {
							event.Privacy = priv
						}
					}
					if len(vals) > 2 && vals[2] != nil {
						if filesArr, ok := vals[2].([]interface{}); ok {
							for _, f := range filesArr {
								if s, ok := f.(string); ok && s != "" {
									event.Files = append(event.Files, s)
								}
							}
						}
					}
				}
			}

			var opErr error
			switch event.Event {
			case "post_created":
				privacy := event.Privacy
				if privacy == "" {
					privacy = "PUBLIC"
				}
				opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
					return c.esRepo.IndexPost(syncCtx, map[string]interface{}{
						"id":         postID,
						"author_id":  event.AuthorID,
						"content":    event.Content,
						"privacy":    privacy,
						"files":      event.Files,
						"created_at": time.Now().UTC(),
					})
				})
			case "post_updated":
				opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
					return c.esRepo.UpdatePost(syncCtx, postID, map[string]interface{}{
						"content":    event.Content,
						"updated_at": time.Now().UTC(),
					})
				})
			case "post_privacy_updated":
				privacy := event.Privacy
				if privacy == "" {
					privacy = "PUBLIC"
				}
				opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
					if err := c.esRepo.UpdatePost(syncCtx, postID, map[string]interface{}{
						"privacy":    privacy,
						"updated_at": time.Now().UTC(),
					}); err != nil {
						return err
					}
					if strings.ToUpper(privacy) == "PRIVATE" {
						return c.esRepo.DeletePostImagesByPost(syncCtx, postID)
					}
					return c.esRepo.UpdatePostImagesPrivacy(syncCtx, postID, privacy)
				})
			case "post_deleted":
				opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
					_ = c.esRepo.DeletePostImagesByPost(syncCtx, postID)
					return c.esRepo.DeletePost(syncCtx, postID)
				})
			default:
				// Fallback if event is based on IsUpdate boolean
				if event.IsUpdate {
					opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
						return c.esRepo.UpdatePost(syncCtx, postID, map[string]interface{}{
							"content":    event.Content,
							"updated_at": time.Now().UTC(),
						})
					})
				} else {
					opErr = executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
						return c.esRepo.IndexPost(syncCtx, map[string]interface{}{
							"id":         postID,
							"author_id":  event.AuthorID,
							"content":    event.Content,
							"privacy":    "PUBLIC",
							"created_at": time.Now().UTC(),
						})
					})
				}
			}

			if opErr != nil {
				logger.WithContext(syncCtx).Err(opErr).Error("Search Service: Post event processing failed after retries for post %s", postID)
				c.publishDLQ(syncCtx, "post-events", postID, msg.Value, opErr)
			}
			cancel()
		}
	}
}

func (c *SearchEventConsumer) consumeUserEvents(ctx context.Context) {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{c.cfg.KafkaAddr},
		Topic:          "user-events",
		GroupID:        "search-service-user-sync-group",
		MinBytes:       10e3,
		MaxBytes:       10e6,
		CommitInterval: 1 * time.Second,
	})
	defer reader.Close()

	logger.Info("Search Service: Kafka Consumer started on topic: user-events")

	for {
		select {
		case <-ctx.Done():
			logger.Info("Search Service: Stopping user-events consumer")
			return
		default:
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Err(err).Warn("Error reading user-events message in Search Service")
				time.Sleep(2 * time.Second)
				continue
			}

			var event map[string]interface{}
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				continue
			}

			eventType, _ := event["event"].(string)
			if eventType == "AccountCreated" || eventType == "UserRegistered" {
				accountID, _ := event["account_id"].(string)
				email, _ := event["email"].(string)
				givenName, _ := event["given_name"].(string)
				familyName, _ := event["family_name"].(string)
				username, _ := event["username"].(string)
				if username == "" {
					username = strings.Split(email, "@")[0]
				}

				fullName := strings.TrimSpace(familyName + " " + givenName)
				if fullName == "" {
					fullName = username
				}

				syncCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				opErr := executeWithRetry(syncCtx, 3, 100*time.Millisecond, func() error {
					return c.esRepo.IndexUser(syncCtx, map[string]interface{}{
						"id":          accountID,
						"username":    username,
						"given_name":  givenName,
						"family_name": familyName,
						"full_name":   fullName,
						"email":       email,
						"created_at":  time.Now().UTC(),
					})
				})
				if opErr != nil {
					logger.WithContext(syncCtx).Err(opErr).Error("Search Service: User event indexing failed after retries for user %s", accountID)
					c.publishDLQ(syncCtx, "user-events", accountID, msg.Value, opErr)
				}
				cancel()
			}
		}
	}
}
