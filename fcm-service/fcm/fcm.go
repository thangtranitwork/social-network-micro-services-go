package fcm

import (
	"context"
	"encoding/json"
	"os"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/messaging"
	"github.com/segmentio/kafka-go"
	"google.golang.org/api/option"
	"social-network-go/logger"
)

// Client handles direct Firebase Admin SDK Push Notifications
type Client struct {
	Messaging *messaging.Client
	ProjectID string
}

type Config struct {
	ProjectID       string
	CredentialsFile string
	CredentialsJSON string
}

func LoadConfigFromEnv() Config {
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}
	return Config{
		ProjectID:       getEnv("FIREBASE_PROJECT_ID", "pocpoc-498009"),
		CredentialsFile: getEnv("FIREBASE_CREDENTIALS_FILE", ""),
		CredentialsJSON: getEnv("FIREBASE_CREDENTIALS_JSON", ""),
	}
}

func NewClient(cfg Config) (*Client, error) {
	ctx := context.Background()
	fbConfig := &firebase.Config{ProjectID: cfg.ProjectID}
	var app *firebase.App
	var err error

	if cfg.CredentialsFile != "" {
		if _, statErr := os.Stat(cfg.CredentialsFile); statErr == nil {
			logger.Info("[FCM Service] Initializing with credential file: %s", cfg.CredentialsFile)
			app, err = firebase.NewApp(ctx, fbConfig, option.WithCredentialsFile(cfg.CredentialsFile))
		}
	}

	if app == nil && cfg.CredentialsJSON != "" {
		logger.Info("[FCM Service] Initializing with JSON environment string")
		app, err = firebase.NewApp(ctx, fbConfig, option.WithCredentialsJSON([]byte(cfg.CredentialsJSON)))
	}

	if app == nil {
		logger.Info("[FCM Service] Initializing with project ID: %s", cfg.ProjectID)
		app, err = firebase.NewApp(ctx, fbConfig)
	}

	if err != nil {
		logger.Warn("[FCM Service] Failed to initialize Firebase App: %v", err)
		return nil, err
	}

	messagingClient, err := app.Messaging(ctx)
	if err != nil {
		logger.Warn("[FCM Service] Failed to initialize Firebase Messaging client: %v", err)
		return nil, err
	}

	logger.Info("✅ [FCM Service] Firebase Admin SDK initialized for project %s", cfg.ProjectID)
	return &Client{Messaging: messagingClient, ProjectID: cfg.ProjectID}, nil
}

func (c *Client) SendMulticastPush(ctx context.Context, tokens []string, title, body string, data map[string]string) ([]string, error) {
	if c == nil || c.Messaging == nil || len(tokens) == 0 {
		return nil, nil
	}

	message := &messaging.MulticastMessage{
		Tokens: tokens,
		Notification: &messaging.Notification{
			Title: title,
			Body:  body,
		},
		Data: data,
	}

	br, err := c.Messaging.SendEachForMulticast(ctx, message)
	if err != nil {
		logger.Error("❌ [FCM Service] Multicast Push error: %v", err)
		return nil, err
	}

	logger.Info("🔥 [FCM Service] Multicast Push (Total: %d, Success: %d, Failure: %d)", len(tokens), br.SuccessCount, br.FailureCount)

	var staleTokens []string
	if br.FailureCount > 0 {
		for idx, resp := range br.Responses {
			if !resp.Success {
				staleTokens = append(staleTokens, tokens[idx])
				tokenSnippet := tokens[idx]
				if len(tokenSnippet) > 15 {
					tokenSnippet = tokenSnippet[:15] + "..."
				}
				logger.Warn("🧹 [FCM Service] Stale token [%s] (%v)", tokenSnippet, resp.Error)
			}
		}
	}
	return staleTokens, nil
}

// NotificationPublisher handles publishing notification events to Kafka
type NotificationPublisher struct {
	writer *kafka.Writer
}

type NotificationEvent struct {
	Type             string `json:"type"` // "SINGLE" or "FRIENDS"
	Action           string `json:"action"`
	CreatorID        string `json:"creatorId"`
	ReceiverID       string `json:"receiverId,omitempty"`
	TargetID         string `json:"targetId"`
	TargetType       string `json:"targetType"`
	ShortenedContent string `json:"shortenedContent"`
}

func NewNotificationPublisher(kafkaAddr string) *NotificationPublisher {
	w := &kafka.Writer{
		Addr:         kafka.TCP(kafkaAddr),
		Topic:        "notification-events",
		Balancer:     &kafka.LeastBytes{},
		Async:        true,
		BatchSize:    100,
		BatchTimeout: 50 * time.Millisecond,
		WriteTimeout: 500 * time.Millisecond,
	}
	return &NotificationPublisher{writer: w}
}

func (p *NotificationPublisher) Send(ctx context.Context, action, creatorID, receiverID, targetID, targetType, shortenedContent string) error {
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
		logger.Err(err).Error("Failed to marshal notification event")
		return err
	}

	publishCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()

	err = p.writer.WriteMessages(publishCtx, kafka.Message{
		Key:   []byte(receiverID),
		Value: payload,
	})
	if err != nil {
		logger.Err(err).Error("Failed to publish notification event %s", action)
		return err
	}

	logger.Info("🔥 [FCM Service] Published notification event %s to Kafka (receiver: %s)", action, receiverID)
	return nil
}

func (p *NotificationPublisher) Close() error {
	if p.writer != nil {
		return p.writer.Close()
	}
	return nil
}
