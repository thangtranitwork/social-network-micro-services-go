package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
	"social-network-go/notification-service/config"
	"social-network-go/notification-service/model"
	"social-network-go/notification-service/repository"
)

type NotificationService struct {
	connections map[string]*websocket.Conn
	mu          sync.RWMutex
	Upgrader    websocket.Upgrader
	cfg         *config.Config
	repo        repository.NotificationRepository
}

func NewNotificationService(cfg *config.Config, repo repository.NotificationRepository) *NotificationService {
	return &NotificationService{
		connections: make(map[string]*websocket.Conn),
		Upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				allowedOriginsEnv := os.Getenv("ALLOWED_ORIGINS")
				if allowedOriginsEnv == "" {
					u, err := url.Parse(origin)
					if err != nil {
						return false
					}
					hostname := u.Hostname()
					return hostname == "localhost" || hostname == "127.0.0.1"
				}
				if allowedOriginsEnv == "*" {
					return true
				}
				origins := strings.Split(allowedOriginsEnv, ",")
				for _, o := range origins {
					if strings.TrimSpace(o) == origin {
						return true
					}
				}
				return false
			},
		},
		cfg:  cfg,
		repo: repo,
	}
}

func (s *NotificationService) RegisterClient(userID string, conn *websocket.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connections[userID] = conn
	logger.Info("User %s connected to Notification WebSocket", userID)
}

func (s *NotificationService) UnregisterClient(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.connections[userID]; ok {
		conn.Close()
		delete(s.connections, userID)
		logger.Info("User %s disconnected from Notification WebSocket", userID)
	}
}

func (s *NotificationService) SaveFCMToken(userID, fcmToken, deviceType string) error {
	err := s.repo.SaveFCMToken(userID, fcmToken, deviceType)
	if err == nil {
		logger.Info("✅ FCM Token saved successfully for user %s", userID)
	}
	return err
}

func (s *NotificationService) PushNotification(receiverID string, notif model.Notification) {
	s.mu.RLock()
	conn, ok := s.connections[receiverID]
	s.mu.RUnlock()

	if ok {
		payload, _ := json.Marshal(notif)
		err := conn.WriteMessage(websocket.TextMessage, payload)
		if err != nil {
			logger.Error("Failed to deliver real-time push notification to %s: %v", receiverID, err)
			s.UnregisterClient(receiverID)
		} else {
			logger.Info("Real-time Push Notification delivered to %s: %s", receiverID, notif.Action)
		}
	}
}

func (s *NotificationService) GetNotifications(userID string, page, size int) ([]model.Notification, int64, error) {
	skip := page * size
	notifications, err := s.repo.GetNotifications(userID, skip, size)
	if err != nil {
		return nil, 0, err
	}
	unreadCount, _ := s.repo.GetUnreadCount(userID)
	return notifications, unreadCount, nil
}

func (s *NotificationService) GetUnreadCount(userID string) (int64, error) {
	return s.repo.GetUnreadCount(userID)
}

func (s *NotificationService) MarkAsRead(userID string, limit int) error {
	return s.repo.MarkAsRead(userID, limit)
}

func (s *NotificationService) StartKafkaConsumers() {
	go s.consumeUserEvents()
	go s.consumeNotificationEvents()
}

func (s *NotificationService) consumeUserEvents() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{s.cfg.KafkaAddr},
		GroupID:  "notification-user-group",
		Topic:    "user-events",
		MinBytes: 10e3,
		MaxBytes: 1e6,
	})
	defer reader.Close()

	logger.Info("Notification Service: Kafka Consumer listening on topic: user-events")
	for {
		m, err := reader.ReadMessage(context.Background())
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Group Coordinator Not Available") || strings.Contains(errStr, "[15]") {
				logger.Warn("Kafka user-events consumer: Group Coordinator preparing/creating offset topic, retrying in 5s...")
				time.Sleep(5 * time.Second)
			} else {
				logger.Error("Kafka user-events consumer error: %v", err)
				time.Sleep(3 * time.Second)
			}
			continue
		}

		var eventData map[string]interface{}
		if err := json.Unmarshal(m.Value, &eventData); err == nil {
			event, _ := eventData["event"].(string)
			accountID, _ := eventData["account_id"].(string)

			if event == "AccountCreated" {
				s.PushNotification(accountID, model.Notification{
					ID:               "n_" + time.Now().Format("20060102150405"),
					Action:           "SYSTEM",
					TargetType:       "SYSTEM",
					TargetID:         accountID,
					ShortenedContent: "Welcome to the Social Network! Please verify your email.",
					SentAt:           time.Now(),
					IsRead:           false,
				})
			}
		}
	}
}

func (s *NotificationService) consumeNotificationEvents() {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{s.cfg.KafkaAddr},
		GroupID:  "notification-events-group",
		Topic:    "notification-events",
		MinBytes: 10e3,
		MaxBytes: 1e6,
	})
	defer reader.Close()

	logger.Info("Notification Service: Kafka Consumer listening on topic: notification-events")
	for {
		m, err := reader.ReadMessage(context.Background())
		if err != nil {
			errStr := err.Error()
			if strings.Contains(errStr, "Group Coordinator Not Available") || strings.Contains(errStr, "[15]") {
				logger.Warn("Kafka notification-events consumer: Group Coordinator preparing/creating offset topic, retrying in 5s...")
				time.Sleep(5 * time.Second)
			} else {
				logger.Error("Kafka notification-events consumer error: %v", err)
				time.Sleep(3 * time.Second)
			}
			continue
		}

		logger.Info("Received notification event: %s", string(m.Value))

		var event model.NotificationKafkaEvent
		if err := json.Unmarshal(m.Value, &event); err != nil {
			logger.Error("Failed to unmarshal notification event: %v", err)
			continue
		}

		s.handleNotificationEvent(event)
	}
}

func (s *NotificationService) handleNotificationEvent(event model.NotificationKafkaEvent) {
	if event.Type == "SINGLE" {
		if event.CreatorID == event.ReceiverID {
			logger.Info("ℹ️ [Notification Service] Creator %s is receiver - skipping self notification", event.CreatorID)
			return
		}

		notifID, err := s.repo.CreateSingleNotification(event)
		if err != nil {
			logger.Error("❌ [Notification Service] Failed to write single notification: %v", err)
			return
		}

		logger.Info("✅ [Notification Service] Created notification %s for user %s", notifID, event.ReceiverID)
		s.fetchAndPushNotification(event.ReceiverID, notifID)

	} else if event.Type == "FRIENDS" {
		friends, err := s.repo.GetFriendsIDs(event.CreatorID)
		if err != nil {
			logger.Error("❌ [Notification Service] Failed to fetch friends for creator %s: %v", event.CreatorID, err)
			return
		}

		logger.Info("👥 [Notification Service] FRIENDS event [%s] from creator %s: found %d friends in Neo4j", event.Action, event.CreatorID, len(friends))
		if len(friends) == 0 {
			logger.Info("ℹ️ [Notification Service] Creator %s has 0 friends in Neo4j - skipping notification dispatch", event.CreatorID)
			return
		}

		for _, friendID := range friends {
			singleEvent := event
			singleEvent.Type = "SINGLE"
			singleEvent.ReceiverID = friendID
			notifID, err := s.repo.CreateSingleNotification(singleEvent)
			if err != nil {
				logger.Error("❌ [Notification Service] Failed to create notification for friend %s: %v", friendID, err)
			} else if notifID != "" {
				logger.Info("✅ [Notification Service] Created notification %s for friend %s", notifID, friendID)
				s.fetchAndPushNotification(friendID, notifID)
			}
		}
	}
}

func (s *NotificationService) fetchAndPushNotification(receiverID string, notifID string) {
	if notifID == "" {
		return
	}
	notif, err := s.repo.FetchNotificationDetails(notifID)
	if err != nil {
		logger.Error("❌ [Notification Service] Failed to fetch notification details for %s: %v", notifID, err)
		return
	}
	if notif == nil {
		logger.Warn("⚠️ [Notification Service] Notification details %s not found in Neo4j", notifID)
		return
	}
	s.PushNotification(receiverID, *notif)
}
