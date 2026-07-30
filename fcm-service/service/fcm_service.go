package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/segmentio/kafka-go"
	"social-network-go/fcm-service/config"
	"social-network-go/fcm-service/fcm"
	"social-network-go/logger"
)

type NotificationKafkaEvent struct {
	Type             string `json:"type"` // "SINGLE" or "FRIENDS"
	Action           string `json:"action"`
	CreatorID        string `json:"creatorId"`
	ReceiverID       string `json:"receiverId,omitempty"`
	TargetID         string `json:"targetId"`
	TargetType       string `json:"targetType"`
	ShortenedContent string `json:"shortenedContent"`
}

type FCMService struct {
	cfg       *config.Config
	driver    neo4j.DriverWithContext
	fcmClient *fcm.Client
}

func NewFCMService(cfg *config.Config, driver neo4j.DriverWithContext) *FCMService {
	fcmCfg := fcm.Config{
		ProjectID:       cfg.FirebaseProjectID,
		CredentialsFile: cfg.FirebaseCredentialsFile,
		CredentialsJSON: cfg.FirebaseCredentialsJSON,
	}

	fcmClient, err := fcm.NewClient(fcmCfg)
	if err != nil {
		logger.Warn("FCM Microservice: Firebase Admin SDK init warning: %v", err)
	}

	return &FCMService{
		cfg:       cfg,
		driver:    driver,
		fcmClient: fcmClient,
	}
}

// StartKafkaConsumer listens on notification-events topic for incoming push events
func (s *FCMService) StartKafkaConsumer() {
	go s.consumeNotificationEvents()
}

func (s *FCMService) consumeNotificationEvents() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{s.cfg.KafkaAddr},
		Topic:    "notification-events",
		GroupID:  "fcm-service-group",
		MinBytes: 10,
		MaxBytes: 1e6,
	})
	defer reader.Close()

	logger.Info("🔥 FCM Microservice: Kafka Consumer listening on topic: notification-events")
	for {
		m, err := reader.ReadMessage(context.Background())
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Group Coordinator Not Available") || strings.Contains(errStr, "[15]") {
				time.Sleep(5 * time.Second)
			} else {
				logger.Error("FCM Microservice Kafka error: %v", err)
				time.Sleep(3 * time.Second)
			}
			continue
		}

		var event NotificationKafkaEvent
		if err := json.Unmarshal(m.Value, &event); err != nil {
			continue
		}

		s.handleFCMNotificationEvent(event)
	}
}

func (s *FCMService) handleFCMNotificationEvent(event NotificationKafkaEvent) {
	if event.Type == "FRIENDS" {
		friends, err := s.getFriendsIDs(event.CreatorID)
		if err != nil {
			logger.Error("FCM Service: Failed to fetch friends for creator %s: %v", event.CreatorID, err)
			return
		}
		logger.Info("🔥 FCM Service: Dispatching FRIENDS event [%s] from creator %s to %d friends", event.Action, event.CreatorID, len(friends))
		for _, friendID := range friends {
			s.sendPushToUser(friendID, event)
		}
		return
	}

	if event.ReceiverID == "" || event.CreatorID == event.ReceiverID {
		return
	}

	s.sendPushToUser(event.ReceiverID, event)
}

func (s *FCMService) sendPushToUser(receiverID string, event NotificationKafkaEvent) {
	tokens, err := s.getFCMTokens(receiverID)
	if err != nil || len(tokens) == 0 {
		return
	}

	title := "PocPoc"
	body := event.ShortenedContent
	if event.Action == "NEW_MESSAGE" {
		title = "Tin nhắn mới từ PocPoc"
	} else if event.Action == "POST" {
		title = "Bài viết mới từ bạn bè"
	}

	logger.Info("🔥 [FCM Push] Dispatching notification to %d FCM tokens for user %s: [%s] %s", len(tokens), receiverID, title, body)

	dataMap := map[string]string{
		"title":    title,
		"body":     body,
		"action":   event.Action,
		"targetId": event.TargetID,
	}

	if s.fcmClient != nil {
		staleTokens, _ := s.fcmClient.SendMulticastPush(context.Background(), tokens, title, body, dataMap)
		if len(staleTokens) > 0 {
			_ = s.deleteFCMTokens(staleTokens)
		}
	}
}

func (s *FCMService) SendDirectPush(receiverID, title, body string, data map[string]string) error {
	tokens, err := s.getFCMTokens(receiverID)
	if err != nil || len(tokens) == 0 {
		logger.Warn("FCM Service: No active FCM tokens found for user %s", receiverID)
		return nil
	}

	if s.fcmClient != nil {
		staleTokens, _ := s.fcmClient.SendMulticastPush(context.Background(), tokens, title, body, data)
		if len(staleTokens) > 0 {
			_ = s.deleteFCMTokens(staleTokens)
		}
	}
	return nil
}

func (s *FCMService) getFriendsIDs(creatorID string) ([]string, error) {
	if s.driver == nil {
		return nil, nil
	}
	ctx := context.Background()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $creatorId})-[:FRIEND]-(f:User)
		RETURN f.id
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{"creatorId": creatorID})
		if err != nil {
			return nil, err
		}
		var list []string
		for result.Next(ctx) {
			if val, ok := result.Record().Values[0].(string); ok && val != "" {
				list = append(list, val)
			}
		}
		return list, nil
	})
	if err != nil || res == nil {
		return nil, err
	}
	return res.([]string), nil
}

func (s *FCMService) getFCMTokens(receiverID string) ([]string, error) {
	if s.driver == nil {
		return nil, nil
	}
	ctx := context.Background()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $receiverId})-[:HAS_FCM_TOKEN]->(t:FCMToken)
		RETURN t.token
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{"receiverId": receiverID})
		if err != nil {
			return nil, err
		}
		var tokens []string
		for result.Next(ctx) {
			if val, ok := result.Record().Values[0].(string); ok && val != "" {
				tokens = append(tokens, val)
			}
		}
		return tokens, nil
	})
	if err != nil || res == nil {
		return nil, err
	}
	return res.([]string), nil
}

func (s *FCMService) deleteFCMTokens(tokens []string) error {
	if s.driver == nil || len(tokens) == 0 {
		return nil
	}
	ctx := context.Background()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	query := `
		UNWIND $tokens AS token
		MATCH (t:FCMToken {token: token})
		DETACH DELETE t
	`
	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, query, map[string]interface{}{"tokens": tokens})
	})
	if err == nil {
		logger.Info("🧹 [FCM Service] Auto-cleaned %d stale FCM tokens from Neo4j", len(tokens))
	}
	return err
}
