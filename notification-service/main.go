package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j/dbtype"
	"github.com/segmentio/kafka-go"
	"social-network-go/logger"
	"social-network-go/profiler"
)

type Config struct {
	HTTPPort  string
	KafkaAddr string
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string
}

type Notification struct {
	ID               string               `json:"id"`
	Action           string               `json:"action"`
	TargetType       string               `json:"targetType"`
	TargetID         string               `json:"targetId"`
	PostID           string               `json:"postId,omitempty"`
	CommentID        string               `json:"commentId,omitempty"`
	RepliedCommentID string               `json:"repliedCommentId,omitempty"`
	Username         string               `json:"username"`
	ShortenedContent string               `json:"shortenedContent"`
	Creator          CreatorInfo          `json:"creator"`
	SentAt           time.Time            `json:"sentAt"`
	IsRead           bool                 `json:"isRead"`
}

type CreatorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type NotificationKafkaEvent struct {
	Type             string `json:"type"` // "SINGLE" or "FRIENDS"
	Action           string `json:"action"`
	CreatorID        string `json:"creatorId"`
	ReceiverID       string `json:"receiverId,omitempty"`
	TargetID         string `json:"targetId"`
	TargetType       string `json:"targetType"`
	ShortenedContent string `json:"shortenedContent"`
}

type NotificationService struct {
	connections map[string]*websocket.Conn
	mu          sync.RWMutex
	upgrader    websocket.Upgrader
	cfg         *Config
	driver      neo4j.DriverWithContext
}

func NewNotificationService(cfg *Config, driver neo4j.DriverWithContext) *NotificationService {
	return &NotificationService{
		connections: make(map[string]*websocket.Conn),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     func(r *http.Request) bool { return true },
		},
		cfg:    cfg,
		driver: driver,
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

func (s *NotificationService) PushNotification(receiverID string, notif Notification) {
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

func (s *NotificationService) StartKafkaConsumers() {
	// 1. Consumer for user-events (Existing)
	go s.consumeUserEvents()

	// 2. Consumer for notification-events (New)
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
			logger.Error("Kafka user-events consumer error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		var eventData map[string]interface{}
		if err := json.Unmarshal(m.Value, &eventData); err == nil {
			event, _ := eventData["event"].(string)
			accountID, _ := eventData["account_id"].(string)

			if event == "AccountCreated" {
				s.PushNotification(accountID, Notification{
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
			logger.Error("Kafka notification-events consumer error: %v", err)
			time.Sleep(3 * time.Second)
			continue
		}

		logger.Info("Received notification event: %s", string(m.Value))

		var event NotificationKafkaEvent
		if err := json.Unmarshal(m.Value, &event); err != nil {
			logger.Error("Failed to unmarshal notification event: %v", err)
			continue
		}

		s.handleNotificationEvent(event)
	}
}

func (s *NotificationService) handleNotificationEvent(event NotificationKafkaEvent) {
	if s.driver == nil {
		logger.Warn("Neo4j driver is nil, skipping notification storage")
		return
	}

	ctx := context.Background()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	if event.Type == "SINGLE" {
		if event.CreatorID == event.ReceiverID {
			return
		}

		var notifID string
		_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			// Handle repeatable actions (LIKE_POST, LIKE_COMMENT)
			if event.Action == "LIKE_POST" || event.Action == "LIKE_COMMENT" {
				checkQuery := `
					MATCH (creator:User {id: $creatorId})<-[:BY_USER]-(n:Notification)<-[:HAS_NOTIFICATION]-(receiver:User {id: $receiverId})
					WHERE n.action = $action
					  AND n.targetId = $targetId
					  AND n.targetType = $targetType
					RETURN n.id
					LIMIT 1
				`
				res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
					"creatorId":  event.CreatorID,
					"receiverId": event.ReceiverID,
					"action":      event.Action,
					"targetId":    event.TargetID,
					"targetType":  event.TargetType,
				})
				if err == nil && res.Next(ctx) {
					notifID = res.Record().Values[0].(string)
					// Update existing
					_, updateErr := tx.Run(ctx, `
						MATCH (n:Notification {id: $id})
						SET n.sentAt = datetime(), n.isRead = false
					`, map[string]interface{}{"id": notifID})
					return nil, updateErr
				}
			}

			// Create new
			notifID = uuid.NewString()
			createQuery := `
				MATCH (creator:User {id: $creatorId}), (receiver:User {id: $receiverId})
				CREATE (receiver)-[:HAS_NOTIFICATION]->(n:Notification {
					id: $id,
					action: $action,
					targetType: $targetType,
					targetId: $targetId,
					shortenedContent: $shortenedContent,
					isRead: false,
					sentAt: datetime()
				})-[:BY_USER]->(creator)
			`
			_, createErr := tx.Run(ctx, createQuery, map[string]interface{}{
				"id":               notifID,
				"creatorId":        event.CreatorID,
				"receiverId":       event.ReceiverID,
				"action":           event.Action,
				"targetType":       event.TargetType,
				"targetId":         event.TargetID,
				"shortenedContent": event.ShortenedContent,
			})
			return nil, createErr
		})

		if err != nil {
			logger.Error("Failed to write single notification: %v", err)
			return
		}

		// Fetch and push
		s.fetchAndPushNotification(event.ReceiverID, notifID)

	} else if event.Type == "FRIENDS" {
		// Get all friends
		sessionRead := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		friendsRes, err := sessionRead.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (u:User {id: $creatorId})-[:FRIEND]-(f:User)
				RETURN f.id
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{"creatorId": event.CreatorID})
			if err != nil {
				return nil, err
			}
			var list []string
			for res.Next(ctx) {
				list = append(list, res.Record().Values[0].(string))
			}
			return list, nil
		})
		sessionRead.Close(ctx)

		if err != nil {
			logger.Error("Failed to fetch friends for notification: %v", err)
			return
		}

		friends := friendsRes.([]string)
		for _, friendID := range friends {
			notifID := uuid.NewString()
			_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
				createQuery := `
					MATCH (creator:User {id: $creatorId}), (receiver:User {id: $receiverId})
					CREATE (receiver)-[:HAS_NOTIFICATION]->(n:Notification {
						id: $id,
						action: $action,
						targetType: $targetType,
						targetId: $targetId,
						shortenedContent: $shortenedContent,
						isRead: false,
						sentAt: datetime()
					})-[:BY_USER]->(creator)
				`
				_, createErr := tx.Run(ctx, createQuery, map[string]interface{}{
					"id":               notifID,
					"creatorId":        event.CreatorID,
					"receiverId":       friendID,
					"action":           event.Action,
					"targetType":       event.TargetType,
					"targetId":         event.TargetID,
					"shortenedContent": event.ShortenedContent,
				})
				return nil, createErr
			})

			if err == nil {
				s.fetchAndPushNotification(friendID, notifID)
			}
		}
	}
}

func (s *NotificationService) fetchAndPushNotification(receiverID string, notifID string) {
	ctx := context.Background()
	session := s.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	notifRes, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		query := `
			MATCH (n:Notification {id: $id})-[:BY_USER]->(creator:User)
			OPTIONAL MATCH (creator)-[:HAS_PROFILE_PICTURE]->(pf:File)
			OPTIONAL MATCH (post:Post {id: n.targetId}) WHERE n.targetType = 'POST'
			OPTIONAL MATCH (comment:Comment {id: n.targetId}) WHERE n.targetType = 'COMMENT'
			OPTIONAL MATCH (comment)-[:REPLIED]->(originalComment:Comment)
			OPTIONAL MATCH (postFromComment:Post)-[:HAS_COMMENT]-(commentWithPost:Comment)
			WHERE commentWithPost = CASE
				WHEN originalComment IS NOT NULL THEN originalComment
				ELSE comment
			END
			RETURN n.id, n.action, n.targetType, n.targetId, n.shortenedContent, n.sentAt, n.isRead,
			       creator.id, creator.username, creator.givenName, creator.familyName, pf.id,
			       post.id, postFromComment.id, comment.id, originalComment.id
		`
		res, err := tx.Run(ctx, query, map[string]interface{}{"id": notifID})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			vals := res.Record().Values
			
			sentAtTime := time.Now()
			if val, ok := vals[5].(dbtype.LocalDateTime); ok {
				sentAtTime = val.Time()
			}

			creatorImg := ""
			if vals[11] != nil {
				creatorImg = vals[11].(string)
			}

			creatorInfo := CreatorInfo{
				ID:                getString(vals[7]),
				Username:          getString(vals[8]),
				GivenName:         getString(vals[9]),
				FamilyName:        getString(vals[10]),
				ProfilePictureUrl: creatorImg,
			}

			n := Notification{
				ID:               getString(vals[0]),
				Action:           getString(vals[1]),
				TargetType:       getString(vals[2]),
				TargetID:         getString(vals[3]),
				ShortenedContent: getString(vals[4]),
				SentAt:           sentAtTime,
				IsRead:           vals[6].(bool),
				Creator:          creatorInfo,
				Username:         creatorInfo.Username,
			}

			// Map target links
			if n.TargetType == "POST" {
				n.PostID = getString(vals[12])
			} else if n.TargetType == "COMMENT" {
				n.PostID = getString(vals[13])
				if vals[15] != nil {
					n.CommentID = getString(vals[15])
					n.RepliedCommentID = getString(vals[14])
				} else {
					n.CommentID = getString(vals[14])
				}
			}

			return n, nil
		}
		return nil, nil
	})

	if err == nil && notifRes != nil {
		s.PushNotification(receiverID, notifRes.(Notification))
	}
}

func getString(val interface{}) string {
	if val == nil {
		return ""
	}
	return val.(string)
}

func main() {
	logger.Info("Starting Notification Service...")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	cfg := &Config{
		HTTPPort:  getEnv("NOTIF_HTTP_PORT", "8085"),
		KafkaAddr: getEnv("KAFKA_ADDR", "localhost:9092"),
		Neo4jURI:  getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser: getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass: getEnv("NEO4J_PASS", "password"),
	}

	// Connect to Neo4j
	var driver neo4j.DriverWithContext
	var err error
	driver, err = neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		logger.Warn("Warning: Failed to connect to Neo4j at %s: %v", cfg.Neo4jURI, err)
	} else {
		logger.Info("Connected to Neo4j successfully")
	}

	s := NewNotificationService(cfg, driver)
	s.StartKafkaConsumers()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(profiler.Middleware("notification-service"))
	r.Use(logger.GinMiddleware())

	r.GET("/debug/profiler", profiler.Handler)
	r.POST("/debug/profiler/reset", func(c *gin.Context) {
		profiler.Reset()
		c.JSON(http.StatusOK, gin.H{"status": "success"})
	})

	// Upgrade notification WS
	r.GET("/v1/notifications/ws", func(c *gin.Context) {
		userID := c.Query("userId")
		if userID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "userId required"})
			return
		}

		conn, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			logger.Error("Failed to upgrade connection: %v", err)
			return
		}

		s.RegisterClient(userID, conn)

		// Ping loop to keep connection alive
		go func() {
			defer s.UnregisterClient(userID)
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					break
				}
			}
		}()
	})

	// GET /v1/notifications - Retrieve user notifications from Neo4j
	r.GET("/v1/notifications", func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = c.Query("current_user_id")
		}
		if userID == "" {
			c.JSON(http.StatusOK, gin.H{
				"code":      200,
				"message":   "OK",
				"timestamp": time.Now().Format(time.RFC3339),
				"body": gin.H{
					"notifications":           []interface{}{},
					"unreadNotificationCount": 0,
				},
			})
			return
		}

		pageStr := c.DefaultQuery("page", "0")
		sizeStr := c.DefaultQuery("size", "10")
		page, _ := strconv.Atoi(pageStr)
		size, _ := strconv.Atoi(sizeStr)
		skip := page * size

		if driver == nil {
			c.JSON(http.StatusOK, gin.H{
				"code":      200,
				"message":   "OK",
				"timestamp": time.Now().Format(time.RFC3339),
				"body": gin.H{
					"notifications":           []interface{}{},
					"unreadNotificationCount": 0,
				},
			})
			return
		}

		ctx := c.Request.Context()
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		notificationsRes, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (receiver:User {id: $userId})
				MATCH (receiver)-[:HAS_NOTIFICATION]->(n:Notification)-[:BY_USER]->(creator:User)
				OPTIONAL MATCH (creator)-[:HAS_PROFILE_PICTURE]->(pf:File)
				OPTIONAL MATCH (post:Post {id: n.targetId}) WHERE n.targetType = 'POST'
				OPTIONAL MATCH (comment:Comment {id: n.targetId}) WHERE n.targetType = 'COMMENT'
				OPTIONAL MATCH (comment)-[:REPLIED]->(originalComment:Comment)
				OPTIONAL MATCH (postFromComment:Post)-[:HAS_COMMENT]-(commentWithPost:Comment)
				WHERE commentWithPost = CASE
					WHEN originalComment IS NOT NULL THEN originalComment
					ELSE comment
				END
				WITH n, creator, pf, post, comment, originalComment, postFromComment
				ORDER BY n.sentAt DESC
				SKIP $skip LIMIT $limit
				SET n.isRead = true
				RETURN n.id, n.action, n.targetType, n.targetId, n.shortenedContent, n.sentAt,
				       creator.id, creator.username, creator.givenName, creator.familyName, pf.id,
				       post.id, postFromComment.id, comment.id, originalComment.id
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{
				"userId": userID,
				"skip":   int64(skip),
				"limit":  int64(size),
			})
			if err != nil {
				return nil, err
			}
			var list []Notification
			for res.Next(ctx) {
				vals := res.Record().Values
				
				sentAtTime := time.Now()
				if val, ok := vals[5].(dbtype.LocalDateTime); ok {
					sentAtTime = val.Time()
				}

				creatorImg := ""
				if vals[10] != nil {
					creatorImg = vals[10].(string)
				}

				creatorInfo := CreatorInfo{
					ID:                getString(vals[6]),
					Username:          getString(vals[7]),
					GivenName:         getString(vals[8]),
					FamilyName:        getString(vals[9]),
					ProfilePictureUrl: creatorImg,
				}

				n := Notification{
					ID:               getString(vals[0]),
					Action:           getString(vals[1]),
					TargetType:       getString(vals[2]),
					TargetID:         getString(vals[3]),
					ShortenedContent: getString(vals[4]),
					SentAt:           sentAtTime,
					IsRead:           true,
					Creator:          creatorInfo,
					Username:         creatorInfo.Username,
				}

				if n.TargetType == "POST" {
					n.PostID = getString(vals[11])
				} else if n.TargetType == "COMMENT" {
					n.PostID = getString(vals[12])
					if vals[14] != nil {
						n.CommentID = getString(vals[14])
						n.RepliedCommentID = getString(vals[13])
					} else {
						n.CommentID = getString(vals[13])
					}
				}

				list = append(list, n)
			}
			return list, nil
		})

		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		// Count unread
		unreadCount := int64(0)
		sessionRead := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		countRes, err := sessionRead.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (receiver:User {id: $userId})-[:HAS_NOTIFICATION]->(n:Notification)
				WHERE n.isRead = false OR n.isRead IS NULL
				RETURN count(n) AS unreadCount
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{"userId": userID})
			if err != nil {
				return int64(0), err
			}
			if res.Next(ctx) {
				return res.Record().Values[0].(int64), nil
			}
			return int64(0), nil
		})
		sessionRead.Close(ctx)

		if err == nil {
			unreadCount = countRes.(int64)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
			"body": gin.H{
				"notifications":           notificationsRes,
				"unreadNotificationCount": unreadCount,
			},
		})
	})

	// GET /v1/notifications/unread-count
	r.GET("/v1/notifications/unread-count", func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = c.Query("current_user_id")
		}
		if userID == "" || driver == nil {
			c.JSON(http.StatusOK, gin.H{
				"code":      200,
				"message":   "OK",
				"timestamp": time.Now().Format(time.RFC3339),
				"body":      0,
			})
			return
		}

		ctx := c.Request.Context()
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
		defer session.Close(ctx)

		countRes, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (receiver:User {id: $userId})-[:HAS_NOTIFICATION]->(n:Notification)
				WHERE n.isRead = false OR n.isRead IS NULL
				RETURN count(n) AS unreadCount
			`
			res, err := tx.Run(ctx, query, map[string]interface{}{"userId": userID})
			if err != nil {
				return int64(0), err
			}
			if res.Next(ctx) {
				return res.Record().Values[0].(int64), nil
			}
			return int64(0), nil
		})

		unreadCount := int64(0)
		if err == nil {
			unreadCount = countRes.(int64)
		}

		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
			"body":      unreadCount,
		})
	})

	// PATCH /v1/notifications/mark-as-read
	r.PATCH("/v1/notifications/mark-as-read", func(c *gin.Context) {
		userID := c.GetHeader("X-User-ID")
		if userID == "" {
			userID = c.Query("current_user_id")
		}
		if userID == "" || driver == nil {
			c.JSON(http.StatusOK, gin.H{
				"code":      200,
				"message":   "OK",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}

		limitStr := c.DefaultQuery("limit", "10")
		limit, _ := strconv.Atoi(limitStr)

		ctx := c.Request.Context()
		session := driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)

		_, _ = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			query := `
				MATCH (receiver:User {id: $userId})-[:HAS_NOTIFICATION]->(n:Notification)
				WHERE n.isRead = false OR n.isRead IS NULL
				WITH n
				ORDER BY n.sentAt DESC
				LIMIT $limit
				SET n.isRead = true
				RETURN n.id
			`
			return tx.Run(ctx, query, map[string]interface{}{
				"userId": userID,
				"limit":  int64(limit),
			})
		})

		c.JSON(http.StatusOK, gin.H{
			"code":      200,
			"message":   "OK",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "service": "notification-service"})
	})

	logger.Info("Notification Service HTTP Server listening on port %s", cfg.HTTPPort)
	_ = r.Run(":" + cfg.HTTPPort)
}
