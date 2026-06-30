package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/chat-service/db"
	"social-network-go/chat-service/model"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/pb"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FileClient interface {
	DeleteFiles(ctx context.Context, fileIDs []string) error
	GetPresignedURL(ctx context.Context, fileID string) (string, error)
	GetPresignedURLs(ctx context.Context, fileIDs []string) (map[string]string, error)
	GetPresignedUploadURL(ctx context.Context, filename, contentType string) (string, string, error)
	Upload(ctx context.Context, file io.Reader, filename, contentType string) (string, error)
}

type ChatService struct {
	// Active connections: userID -> Connection
	connections  map[string]*websocket.Conn
	writeMutexes map[string]*sync.Mutex
	activeChats  map[string]string // userID -> chatID
	mu           sync.RWMutex
	UserClient   pb.UserServiceClient
	FileClient   FileClient
	cfg          *config.Config
}

func NewChatService(cfg *config.Config) *ChatService {
	var userClient pb.UserServiceClient
	conn, err := grpc.NewClient(cfg.UserGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Err(err).Warn("Failed to connect to User gRPC at %s", cfg.UserGrpcAddr)
	} else {
		userClient = pb.NewUserServiceClient(conn)
		logger.Info("Chat Service connected to User gRPC Service at %s", cfg.UserGrpcAddr)
	}

	return &ChatService{
		connections:  make(map[string]*websocket.Conn),
		writeMutexes: make(map[string]*sync.Mutex),
		activeChats:  make(map[string]string),
		UserClient:   userClient,
		cfg:          cfg,
	}
}

func (s *ChatService) WithIntegrations(fileClient FileClient) *ChatService {
	s.FileClient = fileClient
	return s
}

func (s *ChatService) RegisterClient(userID string, conn *websocket.Conn) {
	s.mu.Lock()
	s.connections[userID] = conn
	s.writeMutexes[userID] = &sync.Mutex{}
	count := len(s.connections)
	s.mu.Unlock()
	logger.Info("User %s connected to WebSocket. Active connections: %d", userID, count)

	s.handleRedisOnline(userID)
}

func (s *ChatService) UnregisterClient(userID string) {
	s.mu.Lock()
	if conn, ok := s.connections[userID]; ok {
		conn.Close()
		delete(s.connections, userID)
		delete(s.writeMutexes, userID)
		delete(s.activeChats, userID)
		count := len(s.connections)
		s.mu.Unlock()
		logger.Info("User %s disconnected. Active connections: %d", userID, count)

		s.handleRedisOffline(userID)
		return
	}
	s.mu.Unlock()
}

func (s *ChatService) handleRedisOnline(userID string) {
	if db.RedisClient == nil {
		s.broadcastOnlineStatus(userID, true)
		return
	}
	ctx := context.Background()
	userKey := userID
	counterKey := "user_online_counter:" + userKey
	onlineCountKey := "online_user_count"

	count, err := db.RedisClient.Incr(ctx, counterKey).Result()
	if err == nil && count == 1 {
		db.RedisClient.Incr(ctx, onlineCountKey)
		logger.Info("User %s is now ONLINE (Redis updated)", userID)
		s.broadcastOnlineStatus(userID, true)
	} else if err != nil {
		logger.Err(err).Error("Failed to increment online counter in Redis")
		s.broadcastOnlineStatus(userID, true)
	}
}

func (s *ChatService) handleRedisOffline(userID string) {
	if db.RedisClient == nil {
		s.broadcastOnlineStatus(userID, false)
		return
	}
	ctx := context.Background()
	userKey := userID
	counterKey := "user_online_counter:" + userKey
	onlineCountKey := "online_user_count"
	lastOnlineKey := "last_online:" + userKey

	count, err := db.RedisClient.Decr(ctx, counterKey).Result()
	if err != nil {
		logger.Err(err).Error("Failed to decrement online counter in Redis")
		s.broadcastOnlineStatus(userID, false)
		return
	}

	if count < 0 {
		db.RedisClient.Del(ctx, counterKey)
		s.broadcastOnlineStatus(userID, false)
		return
	}

	if count == 0 {
		db.RedisClient.Del(ctx, counterKey)
		db.RedisClient.Decr(ctx, onlineCountKey)

		nowStr := time.Now().Format(time.RFC3339Nano)
		db.RedisClient.Set(ctx, lastOnlineKey, nowStr, 0)
		logger.Info("User %s is now OFFLINE (Redis updated)", userID)
		s.broadcastOnlineStatus(userID, false)
	}
}

func (s *ChatService) broadcastOnlineStatus(userID string, online bool) {
	go func() {
		rooms := s.GetChatList(userID)

		payloadBytes, _ := json.Marshal(map[string]interface{}{
			"command":    "ONLINE_STATUS",
			"userId":     userID,
			"online":     online,
			"isOnline":   online,
			"lastOnline": time.Now().Format(time.RFC3339),
		})

		s.mu.RLock()
		type targetConn struct {
			conn *websocket.Conn
			mu   *sync.Mutex
		}
		var targets []targetConn
		for _, room := range rooms {
			members := s.GetChatMembers(room.ChatID)
			for _, memberID := range members {
				if memberID == userID {
					continue
				}
				if conn, isOnline := s.connections[memberID]; isOnline {
					if mu := s.writeMutexes[memberID]; mu != nil {
						targets = append(targets, targetConn{conn: conn, mu: mu})
					}
				}
			}
		}
		s.mu.RUnlock()

		for _, t := range targets {
			t.mu.Lock()
			_ = t.conn.WriteMessage(websocket.TextMessage, payloadBytes)
			t.mu.Unlock()
		}
	}()
}

func (s *ChatService) HandleIncomingMessages(userID string) {
	s.mu.RLock()
	conn, ok := s.connections[userID]
	s.mu.RUnlock()
	if !ok {
		return
	}

	defer func() {
		s.UnregisterClient(userID)
	}()

	for {
		_, messageBytes, err := conn.ReadMessage()
		if err != nil {
			logger.Err(err).Error("Error reading WebSocket message from %s", userID)
			break
		}

		// Parse raw map for flexible command detection
		var raw map[string]interface{}
		if err := json.Unmarshal(messageBytes, &raw); err != nil {
			logger.Err(err).Error("Error unmarshalling raw message")
			continue
		}

		cmd, _ := raw["command"].(string)
		chatID, _ := raw["chatId"].(string)

		// DEBUG LOG
		logger.Info("Received WS message from %s: Raw='%s', command='%s', chatId='%s'", userID, string(messageBytes), cmd, chatID)

		// ──────────────────────────────────────────────────────────────────
		// WebRTC Signaling — forward CALL_* packets without touching MongoDB
		// ──────────────────────────────────────────────────────────────────
		if strings.HasPrefix(cmd, "CALL_") {
			if chatID == "" {
				logger.Warn("WebRTC signal %s from %s has no chatId, skipping", cmd, userID)
				continue
			}

			// Background context for state updates
			bgCtx := context.Background()
			isGroup := s.IsGroupChat(chatID)

			// Logic to persist call history/status
			if cmd == "CALL_OFFER" {
				callType, _ := raw["callType"].(string)
				isVideo := callType == "VIDEO"
				callID := chatID

				if isGroup {
					callerInfo, _ := s.UserClient.GetCommonUserInfo(bgCtx, &pb.UserRequest{UserId: userID})
					if callerInfo != nil {
						// Logic for group call history
						_ = s.StartGroupCall(bgCtx, callID, callerInfo.Username, chatID, isVideo)
					}
				} else {
					// 1-1 Call
					members := s.GetChatMembers(chatID)
					var recipientID string
					for _, m := range members {
						if m != userID {
							recipientID = m
							break
						}
					}
					if recipientID != "" {
						callerInfo, _ := s.UserClient.GetCommonUserInfo(bgCtx, &pb.UserRequest{UserId: userID})
						recipientInfo, _ := s.UserClient.GetCommonUserInfo(bgCtx, &pb.UserRequest{UserId: recipientID})

						if callerInfo != nil && recipientInfo != nil {
							s.PrepareCall(bgCtx, callerInfo.Username, recipientInfo.Username)
							_ = s.StartCall(bgCtx, callID, callerInfo.Username, recipientInfo.Username, isVideo)
						}
					}
				}
			} else if cmd == "CALL_ANSWER" {
				if isGroup {
					_ = s.AnswerGroupCall(bgCtx, chatID, userID)
				}
				_ = s.AnswerCall(bgCtx, chatID)
			} else if cmd == "CALL_HANGUP" {
				_ = s.EndCall(bgCtx, chatID)
			}

			members := s.GetChatMembers(chatID)
			// Add senderId to payload
			raw["senderId"] = userID

			targetId, _ := raw["targetId"].(string)
			if targetId != "" {
				// Direct routing
				s.SendToUser(targetId, raw)
			} else {
				// Broadcast to all other members
				for _, memberID := range members {
					if memberID == userID {
						continue // don't echo back to sender
					}
					s.SendToUser(memberID, raw)
				}
			}
			continue
		}

		if cmd == "SUBSCRIBE" {
			if chatID != "" {
				s.mu.Lock()
				s.activeChats[userID] = chatID
				s.mu.Unlock()
				logger.Info("User %s subscribed to chat %s", userID, chatID)

				// Mark messages as read on subscribe!
				s.MarkMessagesAsRead(chatID, userID)
			}
			continue
		}

		if cmd == "UNSUBSCRIBE" {
			s.mu.Lock()
			if currentChat, ok := s.activeChats[userID]; ok && currentChat == chatID {
				delete(s.activeChats, userID)
				logger.Info("User %s unsubscribed from chat %s", userID, chatID)
			}
			s.mu.Unlock()
			continue
		}

		if cmd == "TYPING" || cmd == "STOP_TYPING" {
			if chatID != "" {
				s.BroadcastToChat(chatID, map[string]interface{}{
					"command": cmd,
					"id":      userID,
				})
			}
			continue
		}

		// If it has a command but wasn't handled above, it shouldn't be a text message
		if cmd != "" {
			logger.Warn("Unrecognized command from %s: %s", userID, cmd)
			continue
		}

		// Proceed to parse as regular chat message
		var req struct {
			Username string `json:"username"`
			Text     string `json:"text"`
		}
		_ = json.Unmarshal(messageBytes, &req)
		effectiveChatID := chatID

		// Determine if it is group chat
		isGroup := false
		if effectiveChatID != "" {
			if !s.IsMemberOfChat(userID, effectiveChatID) {
				logger.Warn("User %s is not member of chat %s, skipping message", userID, effectiveChatID)
				continue
			}
			isGroup = s.IsGroupChat(effectiveChatID)
		}

		var recipientID string
		var blockStatus string
		var online bool = true

		if isGroup {
			recipientID = ""
		} else {
			// Resolve recipient ID
			if req.Username != "" {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				if s.UserClient != nil {
					resp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: req.Username})
					if err == nil && resp != nil {
						recipientID = resp.UserId
					} else {
						logger.Err(err).Error("Failed to resolve username %s", req.Username)
					}
				}
				cancel()
			}

			// Fallback to resolve from effectiveChatID members if username not provided
			if recipientID == "" && effectiveChatID != "" {
				members := s.GetChatMembers(effectiveChatID)
				for _, m := range members {
					if m != userID {
						recipientID = m
						break
					}
				}
			}

			if recipientID == "" {
				logger.Warn("Could not resolve recipient for message from %s, skipping message", userID)
				continue
			}

			// Validate block relationship
			blockStatus = s.CheckBlockStatus(userID, recipientID)
			if blockStatus == "BLOCKED" {
				logger.Warn("Validation error: user %s has blocked recipient %s", userID, recipientID)
				_ = conn.WriteMessage(websocket.TextMessage, exception.HasBlocked.Marshal())
				continue
			} else if blockStatus == "HAS_BEEN_BLOCKED" {
				logger.Warn("Validation error: user %s has been blocked by recipient %s", userID, recipientID)
				_ = conn.WriteMessage(websocket.TextMessage, exception.HasBeenBlocked.Marshal())
				continue
			}
		}

		// 1. Validate empty text content
		trimmed := strings.TrimSpace(req.Text)
		if trimmed == "" {
			logger.Warn("Validation error: text content is required for user %s", userID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.TextMessageContentRequired.Marshal())
			continue
		}

		// 2. Validate content length
		if len([]rune(trimmed)) > 10000 {
			logger.Warn("Validation error: message content length exceeds limit for user %s", userID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.InvalidMessageContentLength.Marshal())
			continue
		}

		// Get or create effectiveChatID if not provided
		if effectiveChatID == "" {
			var err error
			effectiveChatID, err = s.GetOrCreateDirectChat(userID, recipientID)
			if err != nil {
				logger.Err(err).Error("Failed to get or create chat for %s -> %s", userID, recipientID)
				continue
			}
		}

		// Determine status based on active chat subscription and online status
		status := "SENT"
		if !isGroup {
			s.mu.RLock()
			activeChat, inChat := s.activeChats[recipientID]
			_, online = s.connections[recipientID]
			s.mu.RUnlock()

			if inChat && activeChat == effectiveChatID {
				status = "READ"
			}
		}

		msg := &model.Message{
			ID:          uuid.New().String(),
			ChatID:      effectiveChatID,
			SenderID:    userID,
			RecipientID: recipientID,
			Content:     trimmed,
			Timestamp:   time.Now(),
			Type:        "TEXT",
			Status:      status,
		}

		// Save message
		s.SaveMessage(msg)

		enriched := s.EnrichMessage(msg)

		// Enrich with presigned URLs
		enrichCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		s.enrichMessageResponsesWithPresignedURLs(enrichCtx, []*MessageResponse{enriched})
		cancel()

		// Broadcast to all chat members (including sender)
		s.BroadcastToChat(effectiveChatID, enriched)

		if !isGroup && !online {
			logger.Info("Recipient %s is offline. Message cached for delivery on login.", recipientID)
		}
	}
}

func (s *ChatService) SaveMessage(msg *model.Message) {
	if db.MsgCollection == nil {
		logger.Warn("MsgCollection is nil, skipping SaveMessage")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.MsgCollection.InsertOne(ctx, msg)
	if err != nil {
		logger.Err(err).Error("Failed to save message to MongoDB")
		return
	}
	logger.Info("Message %s from %s -> %s saved", msg.ID, msg.SenderID, msg.RecipientID)

	s.InvalidateChatListCache(msg.ChatID)
}

func (s *ChatService) InvalidateChatListCache(chatID string) {
	if db.RedisClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	members := s.GetChatMembers(chatID)
	for _, m := range members {
		if m != "" {
			db.RedisClient.Del(ctx, "chat:list:"+m)
		}
	}
}

func (s *ChatService) UpdateMessageStatus(msgID, status string) {
	if db.MsgCollection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.MsgCollection.UpdateOne(
		ctx,
		bson.M{"_id": msgID},
		bson.M{"$set": bson.M{"status": status}},
	)
	if err != nil {
		logger.Err(err).Error("Failed to update message status in MongoDB")
	}
}

func (s *ChatService) GetChatHistory(userID, partnerID string) []*model.Message {
	if db.MsgCollection == nil {
		return []*model.Message{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"$or": []bson.M{
			{"sender_id": userID, "recipient_id": partnerID},
			{"sender_id": partnerID, "recipient_id": userID},
		},
	}
	opts := options.Find().SetSort(bson.M{"timestamp": 1})
	cursor, err := db.MsgCollection.Find(ctx, filter, opts)
	if err != nil {
		logger.Err(err).Error("Failed to fetch chat history")
		return []*model.Message{}
	}
	defer cursor.Close(ctx)

	var history []*model.Message
	if err := cursor.All(ctx, &history); err != nil {
		logger.Err(err).Error("Failed to decode chat history")
		return []*model.Message{}
	}
	if history == nil {
		history = []*model.Message{}
	}
	return history
}

type MessageResponse struct {
	ID             string                  `json:"id"`
	ChatID         string                  `json:"chatId"`
	Content        string                  `json:"content"`
	SentAt         time.Time               `json:"sentAt"`
	Sender         *UserCommonInfoResponse `json:"sender"`
	Attachment     string                  `json:"attachment,omitempty"`
	AttachmentName string                  `json:"attachmentName,omitempty"`
	Deleted        bool                    `json:"deleted"`
	Updated        bool                    `json:"updated"`
	Type           string                  `json:"type"` // TEXT, FILE, GIF, VOICE, CALL
	CallID         string                  `json:"callId,omitempty"`
	CallAt         *time.Time              `json:"callAt,omitempty"`
	AnswerAt       *time.Time              `json:"answerAt,omitempty"`
	EndAt          *time.Time              `json:"endAt,omitempty"`
	IsAnswered     *bool                   `json:"isAnswered,omitempty"`
	IsVideoCall    *bool                   `json:"isVideoCall,omitempty"`
	IsRead         bool                    `json:"isRead"`
}

type UserCommonInfoResponse struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
	Email             string `json:"email"`
}

// ChatRoom represents a conversation summary returned to the UI
type ChatRoom struct {
	ChatID              string           `json:"chatId"`
	Name                string           `json:"name"`
	IsGroup             bool             `json:"isGroup"`
	Avatar              string           `json:"avatar"`
	AdminID             string           `json:"adminId,omitempty"`
	Target              *ChatUser        `json:"target"`
	LatestMessage       *MessageResponse `json:"latestMessage,omitempty"`
	NotReadMessageCount int              `json:"notReadMessageCount"`
	UpdatedAt           time.Time        `json:"updatedAt"`
}

type ChatUser struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
	IsOnline          bool   `json:"isOnline"`
}

// GetChatList returns all unique conversations for the given user, queried from Neo4j and enriched via MongoDB
func (s *ChatService) GetChatList(userID string) []ChatRoom {
	if db.RedisClient != nil {
		ctx := context.Background()
		cacheKey := "chat:list:" + userID
		cached, err := db.RedisClient.Get(ctx, cacheKey).Result()
		if err == nil && cached != "" {
			var rooms []ChatRoom
			if json.Unmarshal([]byte(cached), &rooms) == nil {
				return rooms
			}
		}
	}

	if db.Neo4jDriver == nil {
		logger.Warn("Neo4j driver is nil, returning empty chat list")
		return []ChatRoom{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (currentUser:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)
		OPTIONAL MATCH (chat)<-[:IS_MEMBER_OF]-(target:User)
		WHERE (chat.isGroup IS NULL OR chat.isGroup = false) AND target.id <> $userId
		RETURN chat.id, 
		       coalesce(chat.isGroup, false) AS isGroup, 
		       coalesce(chat.name, "") AS name, 
		       coalesce(chat.avatar, "") AS avatar, 
		       coalesce(chat.adminId, "") AS adminId,
		       target.id AS targetId, 
		       target.username AS targetUsername, 
		       target.givenName AS targetGivenName, 
		       target.familyName AS targetFamilyName, 
		       target.profilePictureId AS targetProfilePictureId
	`

	type Neo4jChat struct {
		ChatID           string
		IsGroup          bool
		Name             string
		Avatar           string
		AdminID          string
		TargetID         string
		TargetUser       string
		GivenName        string
		FamilyName       string
		ProfilePictureId string
	}

	neoChatsVal, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{"userId": userID})
		if err != nil {
			return nil, err
		}
		var chats []Neo4jChat
		for res.Next(ctx) {
			vals := res.Record().Values
			cid, _ := vals[0].(string)
			isGrp, _ := vals[1].(bool)
			name, _ := vals[2].(string)
			avatar, _ := vals[3].(string)
			adminId, _ := vals[4].(string)

			var tid, tname, gname, fname, pimg string
			if vals[5] != nil {
				tid, _ = vals[5].(string)
			}
			if vals[6] != nil {
				tname, _ = vals[6].(string)
			}
			if vals[7] != nil {
				gname, _ = vals[7].(string)
			}
			if vals[8] != nil {
				fname, _ = vals[8].(string)
			}
			if vals[9] != nil {
				pimg, _ = vals[9].(string)
			}
			chats = append(chats, Neo4jChat{
				ChatID:           cid,
				IsGroup:          isGrp,
				Name:             name,
				Avatar:           avatar,
				AdminID:          adminId,
				TargetID:         tid,
				TargetUser:       tname,
				GivenName:        gname,
				FamilyName:       fname,
				ProfilePictureId: pimg,
			})
		}
		return chats, nil
	})

	if err != nil {
		logger.Err(err).Error("Failed to fetch chat list from Neo4j")
		return []ChatRoom{}
	}

	neoChats := neoChatsVal.([]Neo4jChat)
	var rooms []ChatRoom

	chatIDs := make([]string, 0, len(neoChats))
	for _, nc := range neoChats {
		chatIDs = append(chatIDs, nc.ChatID)
	}

	latestMsgMap := make(map[string]*model.Message)
	unreadCountMap := make(map[string]int)

	if len(chatIDs) > 0 && db.MsgCollection != nil {
		// Aggregation for latest messages
		pipelineLatest := []bson.M{
			{"$match": bson.M{"chat_id": bson.M{"$in": chatIDs}}},
			{"$sort": bson.M{"timestamp": -1}},
			{"$group": bson.M{
				"_id":       "$chat_id",
				"latestMsg": bson.M{"$first": "$$ROOT"},
			}},
		}
		cursorLatest, err := db.MsgCollection.Aggregate(ctx, pipelineLatest)
		if err == nil {
			defer cursorLatest.Close(ctx)
			type aggResult struct {
				ChatID    string        `bson:"_id"`
				LatestMsg model.Message `bson:"latestMsg"`
			}
			var results []aggResult
			if err := cursorLatest.All(ctx, &results); err == nil {
				for _, res := range results {
					msgCopy := res.LatestMsg
					latestMsgMap[res.ChatID] = &msgCopy
				}
			} else {
				logger.Err(err).Error("Failed to decode aggregation results for latest messages")
			}
		} else {
			logger.Err(err).Error("Failed to aggregate latest messages")
		}

		// Aggregation for unread counts
		pipelineUnread := []bson.M{
			{"$match": bson.M{
				"chat_id":   bson.M{"$in": chatIDs},
				"sender_id": bson.M{"$ne": userID},
				"status":    bson.M{"$ne": "READ"},
			}},
			{"$group": bson.M{
				"_id":   "$chat_id",
				"count": bson.M{"$sum": 1},
			}},
		}
		cursorUnread, err := db.MsgCollection.Aggregate(ctx, pipelineUnread)
		if err == nil {
			defer cursorUnread.Close(ctx)
			type aggCountResult struct {
				ChatID string `bson:"_id"`
				Count  int    `bson:"count"`
			}
			var results []aggCountResult
			if err := cursorUnread.All(ctx, &results); err == nil {
				for _, res := range results {
					unreadCountMap[res.ChatID] = res.Count
				}
			} else {
				logger.Err(err).Error("Failed to decode unread counts aggregation results")
			}
		} else {
			logger.Err(err).Error("Failed to aggregate unread counts")
		}
	}

	for _, nc := range neoChats {
		lastMsg := latestMsgMap[nc.ChatID]
		unreadCount := unreadCountMap[nc.ChatID]

		// 3. Online status (only for direct 1-1 chats)
		isOnline := false
		if !nc.IsGroup && nc.TargetID != "" {
			s.mu.RLock()
			_, isOnline = s.connections[nc.TargetID]
			s.mu.RUnlock()
		}

		updatedAt := time.Now()
		var latestMsgResp *MessageResponse
		if lastMsg != nil {
			updatedAt = lastMsg.Timestamp
			latestMsgResp = &MessageResponse{
				ID:      lastMsg.ID,
				ChatID:  lastMsg.ChatID,
				Content: lastMsg.Content,
				SentAt:  lastMsg.Timestamp,
				Sender: &UserCommonInfoResponse{
					ID: lastMsg.SenderID,
				},
				Type:        lastMsg.Type,
				IsRead:      lastMsg.Status == "READ",
				Deleted:     lastMsg.Content == "deleted",
				CallID:      lastMsg.CallID,
				CallAt:      lastMsg.CallAt,
				AnswerAt:    lastMsg.AnswerAt,
				EndAt:       lastMsg.EndAt,
				IsAnswered:  lastMsg.IsAnswered,
				IsVideoCall: lastMsg.IsVideoCall,
			}
		}

		var target *ChatUser
		roomName := nc.Name
		roomAvatar := nc.Avatar

		if !nc.IsGroup {
			fullName := nc.GivenName + " " + nc.FamilyName
			if fullName == " " {
				fullName = nc.TargetUser
			}
			roomName = fullName
			roomAvatar = nc.ProfilePictureId

			target = &ChatUser{
				ID:                nc.TargetID,
				Username:          nc.TargetUser,
				GivenName:         nc.GivenName,
				FamilyName:        nc.FamilyName,
				ProfilePictureUrl: nc.ProfilePictureId,
				IsOnline:          isOnline,
			}
		}

		rooms = append(rooms, ChatRoom{
			ChatID:              nc.ChatID,
			Name:                roomName,
			IsGroup:             nc.IsGroup,
			Avatar:              roomAvatar,
			AdminID:             nc.AdminID,
			Target:              target,
			LatestMessage:       latestMsgResp,
			NotReadMessageCount: unreadCount,
			UpdatedAt:           updatedAt,
		})
	}

	// Enrich latest message senders
	latestMsgSenderIDs := make(map[string]bool)
	for _, room := range rooms {
		if room.LatestMessage != nil && room.LatestMessage.Sender != nil && room.LatestMessage.Sender.ID != "" {
			latestMsgSenderIDs[room.LatestMessage.Sender.ID] = true
		}
	}
	if s.UserClient != nil && len(latestMsgSenderIDs) > 0 {
		var ids []string
		for id := range latestMsgSenderIDs {
			ids = append(ids, id)
		}
		grpcCtx, grpcCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer grpcCancel()
		resp, err := s.UserClient.GetUsersByIds(grpcCtx, &pb.UsersByIdsRequest{UserIds: ids})
		if err == nil && resp != nil {
			latestMsgUserMap := make(map[string]*UserCommonInfoResponse)
			for _, u := range resp.Users {
				latestMsgUserMap[u.UserId] = &UserCommonInfoResponse{
					ID:                u.UserId,
					Username:          u.Username,
					GivenName:         u.GivenName,
					FamilyName:        u.FamilyName,
					ProfilePictureUrl: u.ProfilePictureId,
					Email:             u.Email,
				}
			}
			for i := range rooms {
				if rooms[i].LatestMessage != nil && rooms[i].LatestMessage.Sender != nil {
					senderID := rooms[i].LatestMessage.Sender.ID
					if u, exists := latestMsgUserMap[senderID]; exists {
						rooms[i].LatestMessage.Sender = u
					}
				}
			}
		}
	}

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].UpdatedAt.After(rooms[j].UpdatedAt)
	})

	s.enrichChatRoomsWithPresignedURLs(ctx, rooms)

	if db.RedisClient != nil {
		cacheKey := "chat:list:" + userID
		bytes, err := json.Marshal(rooms)
		if err == nil {
			_ = db.RedisClient.Set(ctx, cacheKey, string(bytes), 10*time.Second).Err()
		}
	}

	return rooms
}

func (s *ChatService) IsMemberOfChat(userID, chatID string) bool {
	if db.Neo4jDriver == nil {
		logger.Warn("Neo4j driver is nil, allowing IsMemberOfChat by default")
		return true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User {id: $userId})-[:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		RETURN count(c) > 0
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		result, err := tx.Run(ctx, query, map[string]interface{}{
			"userId": userID,
			"chatId": chatID,
		})
		if err != nil {
			return false, err
		}
		if result.Next(ctx) {
			return result.Record().Values[0].(bool), nil
		}
		return false, nil
	})
	if err != nil {
		logger.Err(err).Error("Failed to verify chat membership in Neo4j")
		return false
	}
	return res.(bool)
}

func (s *ChatService) IsGroupChat(chatID string) bool {
	if db.Neo4jDriver == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (c:Chat {id: $chatId})
		RETURN coalesce(c.isGroup, false)
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		r, err := tx.Run(ctx, query, map[string]interface{}{"chatId": chatID})
		if err != nil {
			return false, err
		}
		if r.Next(ctx) {
			if isGroup, ok := r.Record().Values[0].(bool); ok {
				return isGroup, nil
			}
		}
		return false, nil
	})
	if err != nil {
		return false
	}
	return res.(bool)
}

func (s *ChatService) GetChatMembersDetails(chatID string) []*ChatUser {
	if db.Neo4jDriver == nil {
		return []*ChatUser{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User)-[:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		RETURN u.id, u.username, u.givenName, u.familyName, u.profilePictureId
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		r, err := tx.Run(ctx, query, map[string]interface{}{"chatId": chatID})
		if err != nil {
			return nil, err
		}
		var members []*ChatUser
		for r.Next(ctx) {
			vals := r.Record().Values
			id, _ := vals[0].(string)
			username, _ := vals[1].(string)
			givenName, _ := vals[2].(string)
			familyName, _ := vals[3].(string)
			profilePictureId, _ := vals[4].(string)

			isOnline := false
			s.mu.RLock()
			_, isOnline = s.connections[id]
			s.mu.RUnlock()

			members = append(members, &ChatUser{
				ID:                id,
				Username:          username,
				GivenName:         givenName,
				FamilyName:        familyName,
				ProfilePictureUrl: profilePictureId,
				IsOnline:          isOnline,
			})
		}
		return members, nil
	})
	if err != nil {
		logger.Err(err).Error("Failed to get chat members details from Neo4j")
		return []*ChatUser{}
	}

	members := res.([]*ChatUser)
	for _, m := range members {
		if m.ProfilePictureUrl != "" && s.FileClient != nil {
			if presigned, err := s.FileClient.GetPresignedURL(ctx, m.ProfilePictureUrl); err == nil {
				m.ProfilePictureUrl = presigned
			}
		}
	}
	return members
}

func (s *ChatService) CreateGroupChat(ctx context.Context, adminID string, name string, memberIDs []string) (string, error) {
	if db.Neo4jDriver == nil {
		return "", fmt.Errorf("Neo4j driver is nil")
	}

	chatID := uuid.New().String()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	uniqueMembers := make(map[string]bool)
	uniqueMembers[adminID] = true
	for _, m := range memberIDs {
		if m != "" {
			uniqueMembers[m] = true
		}
	}

	query := `
		CREATE (c:Chat {
			id: $chatId,
			isGroup: true,
			name: $name,
			adminId: $adminId,
			avatar: ""
		})
		WITH c
		UNWIND $memberIds AS memberId
		MATCH (u:User {id: memberId})
		CREATE (u)-[:IS_MEMBER_OF]->(c)
		RETURN c.id
	`

	var membersSlice []string
	for m := range uniqueMembers {
		membersSlice = append(membersSlice, m)
	}

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, query, map[string]interface{}{
			"chatId":    chatID,
			"name":      name,
			"adminId":   adminID,
			"memberIds": membersSlice,
		})
	})
	if err != nil {
		logger.Err(err).Error("Failed to create group chat in Neo4j")
		return "", err
	}

	return chatID, nil
}

func (s *ChatService) AddMembersToGroup(ctx context.Context, adminID string, chatID string, memberIDs []string) error {
	if db.Neo4jDriver == nil {
		return fmt.Errorf("Neo4j driver is nil")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	checkQuery := `
		MATCH (u:User {id: $adminId})-[:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		WHERE c.isGroup = true
		RETURN count(c) > 0
	`
	isMemberVal, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
			"adminId": adminID,
			"chatId":  chatID,
		})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			return res.Record().Values[0].(bool), nil
		}
		return false, nil
	})
	if err != nil || !isMemberVal.(bool) {
		if err != nil {
			logger.Err(err).Error("Failed to verify group membership")
			return err
		}
		return fmt.Errorf("unauthorized or group not found")
	}

	addQuery := `
		MATCH (c:Chat {id: $chatId})
		UNWIND $memberIds AS memberId
		MATCH (u:User {id: memberId})
		MERGE (u)-[:IS_MEMBER_OF]->(c)
		RETURN count(u)
	`
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, addQuery, map[string]interface{}{
			"chatId":    chatID,
			"memberIds": memberIDs,
		})
	})
	return err
}

func (s *ChatService) RemoveMemberFromGroup(ctx context.Context, adminID string, chatID string, userID string) error {
	if db.Neo4jDriver == nil {
		return fmt.Errorf("Neo4j driver is nil")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	checkQuery := `
		MATCH (c:Chat {id: $chatId})
		WHERE c.isGroup = true
		RETURN c.adminId, (exists((:User {id: $adminId})-[:IS_MEMBER_OF]->(c)))
	`
	authVal, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
			"chatId":  chatID,
			"adminId": adminID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			admin, _ := res.Record().Values[0].(string)
			isMember, _ := res.Record().Values[1].(bool)
			return map[string]interface{}{
				"adminId":  admin,
				"isMember": isMember,
			}, nil
		}
		return nil, nil
	})
	if err != nil || authVal == nil {
		if err != nil {
			logger.Err(err).Error("Failed to fetch group auth info")
			return err
		}
		return fmt.Errorf("group not found")
	}

	authMap := authVal.(map[string]interface{})
	groupAdmin := authMap["adminId"].(string)
	isMember := authMap["isMember"].(bool)

	if !isMember {
		return fmt.Errorf("unauthorized: caller is not a member of this group")
	}

	if groupAdmin != adminID && adminID != userID {
		return fmt.Errorf("unauthorized: only the group admin can remove members")
	}

	removeQuery := `
		MATCH (u:User {id: $userId})-[r:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		DELETE r
	`
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, removeQuery, map[string]interface{}{
			"chatId": chatID,
			"userId": userID,
		})
	})
	if err != nil {
		logger.Err(err).Error("Failed to remove member relationship in Neo4j")
		return err
	}

	if groupAdmin == userID {
		assignAdminQuery := `
			MATCH (c:Chat {id: $chatId})<-[:IS_MEMBER_OF]-(newAdmin:User)
			LIMIT 1
			SET c.adminId = newAdmin.id
			RETURN newAdmin.id
		`
		_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			res, err := tx.Run(ctx, assignAdminQuery, map[string]interface{}{
				"chatId": chatID,
			})
			if err != nil {
				return nil, err
			}
			if res.Next(ctx) {
				return res.Record().Values[0].(string), nil
			}
			return "", nil
		})
		if err != nil {
			logger.Err(err).Error("Failed to auto-assign new group admin")
		}
	}

	return nil
}

func (s *ChatService) UpdateGroupChat(ctx context.Context, adminID string, chatID string, name string, avatar string) error {
	if db.Neo4jDriver == nil {
		return fmt.Errorf("Neo4j driver is nil")
	}

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	checkQuery := `
		MATCH (u:User {id: $adminId})-[:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		WHERE c.isGroup = true
		RETURN count(c) > 0
	`
	isMemberVal, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
			"adminId": adminID,
			"chatId":  chatID,
		})
		if err != nil {
			return false, err
		}
		if res.Next(ctx) {
			return res.Record().Values[0].(bool), nil
		}
		return false, nil
	})
	if err != nil || !isMemberVal.(bool) {
		return fmt.Errorf("unauthorized or group not found")
	}

	updateQuery := `
		MATCH (c:Chat {id: $chatId})
		SET c.name = $name, c.avatar = $avatar
		RETURN c.id
	`
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, updateQuery, map[string]interface{}{
			"chatId": chatID,
			"name":   name,
			"avatar": avatar,
		})
	})
	return err
}

func (s *ChatService) GetChatMessages(chatID string, skip, limit int64) []*MessageResponse {
	if db.MsgCollection == nil {
		return []*MessageResponse{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"chat_id": chatID}
	opts := options.Find().
		SetSort(bson.M{"timestamp": -1}).
		SetSkip(skip).
		SetLimit(limit)

	cursor, err := db.MsgCollection.Find(ctx, filter, opts)
	if err != nil {
		logger.Err(err).Error("Failed to fetch messages for chat %s", chatID)
		return []*MessageResponse{}
	}
	defer cursor.Close(ctx)

	var msgs []*model.Message
	if err := cursor.All(ctx, &msgs); err != nil {
		logger.Err(err).Error("Failed to decode messages for chat %s", chatID)
		return []*MessageResponse{}
	}

	senderIDs := make(map[string]bool)
	for _, m := range msgs {
		senderIDs[m.SenderID] = true
	}

	userMap := make(map[string]*UserCommonInfoResponse)
	if s.UserClient != nil && len(senderIDs) > 0 {
		var ids []string
		for id := range senderIDs {
			ids = append(ids, id)
		}
		resp, err := s.UserClient.GetUsersByIds(ctx, &pb.UsersByIdsRequest{UserIds: ids})
		if err == nil && resp != nil {
			for _, u := range resp.Users {
				userMap[u.UserId] = &UserCommonInfoResponse{
					ID:                u.UserId,
					Username:          u.Username,
					GivenName:         u.GivenName,
					FamilyName:        u.FamilyName,
					ProfilePictureUrl: u.ProfilePictureId,
					Email:             u.Email,
				}
			}
		}
	}

	var response []*MessageResponse
	for _, m := range msgs {
		sender, ok := userMap[m.SenderID]
		if !ok {
			sender = &UserCommonInfoResponse{
				ID:       m.SenderID,
				Username: m.SenderID,
			}
		}

		var attachment, attachmentName string
		if m.Type == "FILE" || m.Type == "VOICE" {
			attachment = m.Content
			attachmentName = m.Content
		} else if m.Type == "GIF" {
			attachment = m.Content
		}

		response = append(response, &MessageResponse{
			ID:             m.ID,
			ChatID:         m.ChatID,
			Content:        m.Content,
			Attachment:     attachment,
			AttachmentName: attachmentName,
			SentAt:         m.Timestamp,
			Sender:         sender,
			Type:           m.Type,
			IsRead:         m.Status == "READ",
			Deleted:        m.Content == "deleted",
			CallID:         m.CallID,
			CallAt:         m.CallAt,
			AnswerAt:       m.AnswerAt,
			EndAt:          m.EndAt,
			IsAnswered:     m.IsAnswered,
			IsVideoCall:    m.IsVideoCall,
		})
	}

	s.enrichMessageResponsesWithPresignedURLs(ctx, response)

	return response
}

func (s *ChatService) MarkMessagesAsRead(chatID, userID string) {
	if s.IsGroupChat(chatID) {
		// For Group Chat, we don't update global status to READ since it is shared,
		// but we still broadcast the READING command to online members for real-time presence.
		s.BroadcastToChat(chatID, map[string]interface{}{
			"command": "READING",
			"id":      userID,
		})
		return
	}

	if db.MsgCollection == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"chat_id":   chatID,
		"sender_id": bson.M{"$ne": userID},
		"status":    bson.M{"$ne": "READ"},
	}
	update := bson.M{
		"$set": bson.M{"status": "READ"},
	}

	_, err := db.MsgCollection.UpdateMany(ctx, filter, update)
	if err != nil {
		logger.Err(err).Error("Failed to mark messages as read for chat %s, user %s", chatID, userID)
	}

	if db.Neo4jDriver != nil {
		session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
		defer session.Close(ctx)
		query := `
			MATCH (c:Chat {id: $chatId})-[:HAS_MESSAGE]->(m:Message)
			WHERE NOT (:User {id: $userId})-[:SENT]->(m) AND m.isRead = false
			SET m.isRead = true
		`
		_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
			return tx.Run(ctx, query, map[string]interface{}{
				"chatId": chatID,
				"userId": userID,
			})
		})
		if err != nil {
			logger.Err(err).Error("Failed to mark messages as read in Neo4j")
		}
	}

	// Broadcast READING command to notify chat members
	s.BroadcastToChat(chatID, map[string]interface{}{
		"command": "READING",
		"id":      userID,
	})

	s.InvalidateChatListCache(chatID)
}

func (s *ChatService) EnrichMessage(msg *model.Message) *MessageResponse {
	sender := &UserCommonInfoResponse{
		ID:       msg.SenderID,
		Username: msg.SenderID,
	}

	if s.UserClient != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: msg.SenderID})
		if err == nil && resp != nil {
			sender = &UserCommonInfoResponse{
				ID:                resp.UserId,
				Username:          resp.Username,
				GivenName:         resp.GivenName,
				FamilyName:        resp.FamilyName,
				ProfilePictureUrl: resp.ProfilePictureId,
				Email:             resp.Email,
			}
		}
	}

	var attachment, attachmentName string
	if msg.Type == "FILE" || msg.Type == "VOICE" {
		attachment = "/v1/files/" + msg.Content
		attachmentName = msg.Content
	} else if msg.Type == "GIF" {
		attachment = msg.Content
	}

	return &MessageResponse{
		ID:             msg.ID,
		ChatID:         msg.ChatID,
		Content:        msg.Content,
		Attachment:     attachment,
		AttachmentName: attachmentName,
		SentAt:         msg.Timestamp,
		Sender:         sender,
		Type:           msg.Type,
		IsRead:         msg.Status == "READ",
		Deleted:        msg.Content == "deleted",
		CallID:         msg.CallID,
		CallAt:         msg.CallAt,
		AnswerAt:       msg.AnswerAt,
		EndAt:          msg.EndAt,
		IsAnswered:     msg.IsAnswered,
		IsVideoCall:    msg.IsVideoCall,
	}
}

func (s *ChatService) GetChatMembers(chatID string) []string {
	if db.Neo4jDriver == nil {
		return []string{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u:User)-[:IS_MEMBER_OF]->(c:Chat {id: $chatId})
		RETURN u.id
	`
	res, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		r, err := tx.Run(ctx, query, map[string]interface{}{"chatId": chatID})
		if err != nil {
			return nil, err
		}
		var members []string
		for r.Next(ctx) {
			if id, ok := r.Record().Values[0].(string); ok {
				members = append(members, id)
			}
		}
		return members, nil
	})
	if err != nil {
		logger.Err(err).Error("Failed to get chat members from Neo4j")
		return []string{}
	}
	return res.([]string)
}

func (s *ChatService) BroadcastToChat(chatID string, payload interface{}) {
	s.mu.RLock()
	members := s.GetChatMembers(chatID)

	type targetConn struct {
		conn *websocket.Conn
		mu   *sync.Mutex
	}
	var targets []targetConn
	for _, memberID := range members {
		if conn, online := s.connections[memberID]; online {
			if mu := s.writeMutexes[memberID]; mu != nil {
				targets = append(targets, targetConn{conn: conn, mu: mu})
			}
		}
	}
	s.mu.RUnlock()

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Err(err).Error("Failed to marshal broadcast payload")
		return
	}

	for _, t := range targets {
		t.mu.Lock()
		_ = t.conn.WriteMessage(websocket.TextMessage, payloadBytes)
		t.mu.Unlock()
	}
}

func (s *ChatService) SendMessage(senderID, receiverUsername, text string) (*MessageResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.UserClient == nil {
		return nil, fmt.Errorf("user client not initialized")
	}

	receiverResp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: receiverUsername})
	if err != nil {
		logger.Err(err).Error("Failed to get common user info for receiver %s", receiverUsername)
		return nil, fmt.Errorf("receiver not found: %w", err)
	}

	friendship, err := s.UserClient.CheckFriendship(ctx, &pb.FriendshipRequest{
		UserIdA: senderID,
		UserIdB: receiverResp.UserId,
	})
	if err == nil && friendship != nil && friendship.HasBlocked {
		return nil, fmt.Errorf("BLOCKED")
	}

	chatID, err := s.GetOrCreateDirectChat(senderID, receiverResp.UserId)
	if err != nil {
		logger.Err(err).Error("Failed to get or create direct chat between %s and %s", senderID, receiverResp.UserId)
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	s.mu.RLock()
	activeChat, inChat := s.activeChats[receiverResp.UserId]
	s.mu.RUnlock()

	status := "SENT"
	if inChat && activeChat == chatID {
		status = "READ"
	}
	logger.Info("saving message to DB with content %s", text)
	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    senderID,
		RecipientID: receiverResp.UserId,
		Content:     strings.TrimSpace(text),
		Timestamp:   time.Now(),
		Type:        "TEXT",
		Status:      status,
	}

	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)

	// Enrich with presigned URLs before broadcasting
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	s.BroadcastToChat(chatID, enriched)

	return enriched, nil
}

func (s *ChatService) SendGif(senderID, receiverUsername, gifURL string) (*MessageResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.UserClient == nil {
		return nil, fmt.Errorf("user client not initialized")
	}

	receiverResp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: receiverUsername})
	if err != nil {
		logger.Err(err).Error("Failed to get common user info for receiver %s", receiverUsername)
		return nil, fmt.Errorf("receiver not found: %w", err)
	}

	friendship, err := s.UserClient.CheckFriendship(ctx, &pb.FriendshipRequest{
		UserIdA: senderID,
		UserIdB: receiverResp.UserId,
	})
	if err == nil && friendship != nil && friendship.HasBlocked {
		return nil, fmt.Errorf("BLOCKED")
	}

	chatID, err := s.GetOrCreateDirectChat(senderID, receiverResp.UserId)
	if err != nil {
		logger.Err(err).Error("Failed to get or create direct chat between %s and %s", senderID, receiverResp.UserId)
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	s.mu.RLock()
	activeChat, inChat := s.activeChats[receiverResp.UserId]
	s.mu.RUnlock()

	status := "SENT"
	if inChat && activeChat == chatID {
		status = "READ"
	}

	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    senderID,
		RecipientID: receiverResp.UserId,
		Content:     gifURL,
		Timestamp:   time.Now(),
		Type:        "GIF",
		Status:      status,
	}

	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)

	// Enrich with presigned URLs before broadcasting
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	s.BroadcastToChat(chatID, enriched)

	return enriched, nil
}

func (s *ChatService) SendFile(senderID, receiverUsername, fileID string) (*MessageResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.UserClient == nil {
		return nil, fmt.Errorf("user client not initialized")
	}

	receiverResp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: receiverUsername})
	if err != nil {
		logger.Err(err).Error("Failed to get common user info for receiver %s", receiverUsername)
		return nil, fmt.Errorf("receiver not found: %w", err)
	}

	friendship, err := s.UserClient.CheckFriendship(ctx, &pb.FriendshipRequest{
		UserIdA: senderID,
		UserIdB: receiverResp.UserId,
	})
	if err == nil && friendship != nil && friendship.HasBlocked {
		return nil, fmt.Errorf("BLOCKED")
	}

	chatID, err := s.GetOrCreateDirectChat(senderID, receiverResp.UserId)
	if err != nil {
		logger.Err(err).Error("Failed to get or create direct chat between %s and %s", senderID, receiverResp.UserId)
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	s.mu.RLock()
	activeChat, inChat := s.activeChats[receiverResp.UserId]
	s.mu.RUnlock()

	status := "SENT"
	if inChat && activeChat == chatID {
		status = "READ"
	}

	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    senderID,
		RecipientID: receiverResp.UserId,
		Content:     fileID,
		Timestamp:   time.Now(),
		Type:        "FILE",
		Status:      status,
	}

	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)

	// Enrich with presigned URLs before broadcasting
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	s.BroadcastToChat(chatID, enriched)

	return enriched, nil
}

func (s *ChatService) SendVoice(senderID, receiverUsername, fileID string) (*MessageResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if s.UserClient == nil {
		return nil, fmt.Errorf("user client not initialized")
	}

	receiverResp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: receiverUsername})
	if err != nil {
		logger.Err(err).Error("Failed to get common user info for receiver %s", receiverUsername)
		return nil, fmt.Errorf("receiver not found: %w", err)
	}

	friendship, err := s.UserClient.CheckFriendship(ctx, &pb.FriendshipRequest{
		UserIdA: senderID,
		UserIdB: receiverResp.UserId,
	})
	if err == nil && friendship != nil && friendship.HasBlocked {
		return nil, fmt.Errorf("BLOCKED")
	}

	chatID, err := s.GetOrCreateDirectChat(senderID, receiverResp.UserId)
	if err != nil {
		logger.Err(err).Error("Failed to get or create direct chat between %s and %s", senderID, receiverResp.UserId)
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	s.mu.RLock()
	activeChat, inChat := s.activeChats[receiverResp.UserId]
	s.mu.RUnlock()

	status := "SENT"
	if inChat && activeChat == chatID {
		status = "READ"
	}

	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    senderID,
		RecipientID: receiverResp.UserId,
		Content:     fileID,
		Timestamp:   time.Now(),
		Type:        "VOICE",
		Status:      status,
	}

	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)

	// Enrich with presigned URLs before broadcasting
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	s.BroadcastToChat(chatID, enriched)

	return enriched, nil
}

func (s *ChatService) DeleteMessage(messageID, userID string) error {
	if db.MsgCollection == nil {
		return fmt.Errorf("database collection not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var msg model.Message
	err := db.MsgCollection.FindOne(ctx, bson.M{"_id": messageID}).Decode(&msg)
	if err != nil {
		logger.Err(err).Error("Failed to find message %s in MongoDB", messageID)
		return fmt.Errorf("MESSAGE_NOT_FOUND")
	}

	if msg.SenderID != userID {
		return fmt.Errorf("UNAUTHORIZED")
	}

	update := bson.M{
		"$set": bson.M{
			"content": "deleted",
		},
	}
	_, err = db.MsgCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		logger.Err(err).Error("Failed to update message %s in MongoDB", messageID)
		return err
	}

	if msg.Type == "FILE" || msg.Type == "VOICE" {
		filePath := filepath.Join("/home/thang/coding/social-network/upload", msg.Content)
		_ = os.Remove(filePath)
	}

	s.BroadcastToChat(msg.ChatID, map[string]interface{}{
		"command": "DELETE",
		"id":      messageID,
	})

	return nil
}

func (s *ChatService) EditMessage(messageID, newContent, userID string) error {
	if db.MsgCollection == nil {
		return fmt.Errorf("database collection not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var msg model.Message
	err := db.MsgCollection.FindOne(ctx, bson.M{"_id": messageID}).Decode(&msg)
	if err != nil {
		logger.Err(err).Error("Failed to find message %s in MongoDB for edit", messageID)
		return fmt.Errorf("MESSAGE_NOT_FOUND")
	}

	if msg.SenderID != userID {
		return fmt.Errorf("UNAUTHORIZED")
	}

	if msg.Type != "TEXT" && msg.Type != "" {
		return fmt.Errorf("CAN_NOT_EDIT_FILE_OR_CALL")
	}

	trimmed := strings.TrimSpace(newContent)
	if trimmed == "" {
		return fmt.Errorf("TEXT_MESSAGE_CONTENT_REQUIRED")
	}

	update := bson.M{
		"$set": bson.M{
			"content": trimmed,
		},
	}
	_, err = db.MsgCollection.UpdateOne(ctx, bson.M{"_id": messageID}, update)
	if err != nil {
		logger.Err(err).Error("Failed to edit message %s in MongoDB", messageID)
		return err
	}

	s.BroadcastToChat(msg.ChatID, map[string]interface{}{
		"command":  "EDIT",
		"id":       messageID,
		"message":  trimmed,
		"editedAt": time.Now().Format(time.RFC3339),
	})

	return nil
}

func (s *ChatService) SearchChats(userID, searchQuery string) []ChatRoom {
	if db.Neo4jDriver == nil {
		logger.Warn("Neo4j driver is nil, returning empty chat list")
		return []ChatRoom{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (currentUser:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)
		OPTIONAL MATCH (chat)<-[:IS_MEMBER_OF]-(target:User)
		WHERE (chat.isGroup IS NULL OR chat.isGroup = false) AND target.id <> $userId
		WITH chat, target
		WHERE (
			(chat.isGroup = true AND toLower(chat.name) CONTAINS toLower($searchQuery)) OR
			((chat.isGroup IS NULL OR chat.isGroup = false) AND (
				toLower(target.givenName + ' ' + target.familyName) CONTAINS toLower($searchQuery) OR
				toLower(target.username) CONTAINS toLower($searchQuery)
			))
		)
		RETURN chat.id, 
		       coalesce(chat.isGroup, false) AS isGroup, 
		       coalesce(chat.name, "") AS name, 
		       coalesce(chat.avatar, "") AS avatar, 
		       coalesce(chat.adminId, "") AS adminId,
		       target.id AS targetId, 
		       target.username AS targetUsername, 
		       target.givenName AS targetGivenName, 
		       target.familyName AS targetFamilyName, 
		       target.profilePictureId AS targetProfilePictureId
	`

	type Neo4jChat struct {
		ChatID           string
		IsGroup          bool
		Name             string
		Avatar           string
		AdminID          string
		TargetID         string
		TargetUser       string
		GivenName        string
		FamilyName       string
		ProfilePictureId string
	}

	neoChatsVal, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{"userId": userID, "searchQuery": searchQuery})
		if err != nil {
			return nil, err
		}
		var chats []Neo4jChat
		for res.Next(ctx) {
			vals := res.Record().Values
			cid, _ := vals[0].(string)
			isGrp, _ := vals[1].(bool)
			name, _ := vals[2].(string)
			avatar, _ := vals[3].(string)
			adminId, _ := vals[4].(string)

			var tid, tname, gname, fname, pimg string
			if vals[5] != nil {
				tid, _ = vals[5].(string)
			}
			if vals[6] != nil {
				tname, _ = vals[6].(string)
			}
			if vals[7] != nil {
				gname, _ = vals[7].(string)
			}
			if vals[8] != nil {
				fname, _ = vals[8].(string)
			}
			if vals[9] != nil {
				pimg, _ = vals[9].(string)
			}
			chats = append(chats, Neo4jChat{
				ChatID:           cid,
				IsGroup:          isGrp,
				Name:             name,
				Avatar:           avatar,
				AdminID:          adminId,
				TargetID:         tid,
				TargetUser:       tname,
				GivenName:        gname,
				FamilyName:       fname,
				ProfilePictureId: pimg,
			})
		}
		return chats, nil
	})

	if err != nil {
		logger.Err(err).Error("Failed to search chat list from Neo4j")
		return []ChatRoom{}
	}

	neoChats := neoChatsVal.([]Neo4jChat)
	var rooms []ChatRoom

	for _, nc := range neoChats {
		var lastMsg *model.Message
		if db.MsgCollection != nil {
			opts := options.FindOne().SetSort(bson.M{"timestamp": -1})
			var m model.Message
			err := db.MsgCollection.FindOne(ctx, bson.M{"chat_id": nc.ChatID}, opts).Decode(&m)
			if err == nil {
				lastMsg = &m
			}
		}

		unreadCount := 0
		if db.MsgCollection != nil {
			count, err := db.MsgCollection.CountDocuments(ctx, bson.M{
				"chat_id":   nc.ChatID,
				"sender_id": bson.M{"$ne": userID},
				"status":    bson.M{"$ne": "READ"},
			})
			if err == nil {
				unreadCount = int(count)
			}
		}

		isOnline := false
		if !nc.IsGroup && nc.TargetID != "" {
			s.mu.RLock()
			_, isOnline = s.connections[nc.TargetID]
			s.mu.RUnlock()
		}

		updatedAt := time.Now()
		var latestMsgResp *MessageResponse
		if lastMsg != nil {
			updatedAt = lastMsg.Timestamp

			var attachment, attachmentName string
			if lastMsg.Type == "FILE" || lastMsg.Type == "VOICE" {
				attachment = "/v1/files/" + lastMsg.Content
				attachmentName = lastMsg.Content
			}

			latestMsgResp = &MessageResponse{
				ID:             lastMsg.ID,
				ChatID:         lastMsg.ChatID,
				Content:        lastMsg.Content,
				Attachment:     attachment,
				AttachmentName: attachmentName,
				SentAt:         lastMsg.Timestamp,
				Sender: &UserCommonInfoResponse{
					ID: lastMsg.SenderID,
				},
				Type:    lastMsg.Type,
				IsRead:  lastMsg.Status == "READ",
				Deleted: lastMsg.Content == "deleted",
			}
		}

		var target *ChatUser
		roomName := nc.Name
		roomAvatar := nc.Avatar

		if !nc.IsGroup {
			fullName := nc.GivenName + " " + nc.FamilyName
			if fullName == " " {
				fullName = nc.TargetUser
			}
			roomName = fullName
			roomAvatar = nc.ProfilePictureId

			target = &ChatUser{
				ID:                nc.TargetID,
				Username:          nc.TargetUser,
				GivenName:         nc.GivenName,
				FamilyName:        nc.FamilyName,
				ProfilePictureUrl: nc.ProfilePictureId,
				IsOnline:          isOnline,
			}
		}

		rooms = append(rooms, ChatRoom{
			ChatID:              nc.ChatID,
			Name:                roomName,
			IsGroup:             nc.IsGroup,
			Avatar:              roomAvatar,
			AdminID:             nc.AdminID,
			Target:              target,
			LatestMessage:       latestMsgResp,
			NotReadMessageCount: unreadCount,
			UpdatedAt:           updatedAt,
		})
	}

	// Enrich latest message senders
	latestMsgSenderIDs := make(map[string]bool)
	for _, room := range rooms {
		if room.LatestMessage != nil && room.LatestMessage.Sender != nil && room.LatestMessage.Sender.ID != "" {
			latestMsgSenderIDs[room.LatestMessage.Sender.ID] = true
		}
	}
	if s.UserClient != nil && len(latestMsgSenderIDs) > 0 {
		var ids []string
		for id := range latestMsgSenderIDs {
			ids = append(ids, id)
		}
		grpcCtx, grpcCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer grpcCancel()
		resp, err := s.UserClient.GetUsersByIds(grpcCtx, &pb.UsersByIdsRequest{UserIds: ids})
		if err == nil && resp != nil {
			latestMsgUserMap := make(map[string]*UserCommonInfoResponse)
			for _, u := range resp.Users {
				latestMsgUserMap[u.UserId] = &UserCommonInfoResponse{
					ID:                u.UserId,
					Username:          u.Username,
					GivenName:         u.GivenName,
					FamilyName:        u.FamilyName,
					ProfilePictureUrl: u.ProfilePictureId,
					Email:             u.Email,
				}
			}
			for i := range rooms {
				if rooms[i].LatestMessage != nil && rooms[i].LatestMessage.Sender != nil {
					senderID := rooms[i].LatestMessage.Sender.ID
					if u, exists := latestMsgUserMap[senderID]; exists {
						rooms[i].LatestMessage.Sender = u
					}
				}
			}
		}
	}

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].UpdatedAt.After(rooms[j].UpdatedAt)
	})

	s.enrichChatRoomsWithPresignedURLs(ctx, rooms)

	return rooms
}

func (s *ChatService) GetOrCreateDirectChat(userID, partnerID string) (string, error) {
	if db.Neo4jDriver == nil {
		return "", fmt.Errorf("Neo4j driver is nil")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	// 1. Check if chat already exists
	checkQuery := `
		MATCH (u1:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)<-[:IS_MEMBER_OF]-(u2:User {id: $partnerId})
		RETURN chat.id
	`
	existingChatVal, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, checkQuery, map[string]interface{}{
			"userId":    userID,
			"partnerId": partnerID,
		})
		if err != nil {
			return nil, err
		}
		if res.Next(ctx) {
			if id, ok := res.Record().Values[0].(string); ok {
				return id, nil
			}
		}
		return "", nil
	})
	if err != nil {
		logger.Err(err).Error("Failed to query existing direct chat in Neo4j")
		return "", err
	}

	existingChatID := existingChatVal.(string)
	if existingChatID != "" {
		return existingChatID, nil
	}

	// 2. Chat doesn't exist, create it
	newChatID := uuid.New().String()
	createQuery := `
		MATCH (u1:User {id: $userId})
		MATCH (u2:User {id: $partnerId})
		CREATE (chat:Chat {id: $chatId})
		CREATE (u1)-[:IS_MEMBER_OF]->(chat)
		CREATE (u2)-[:IS_MEMBER_OF]->(chat)
		RETURN chat.id
	`
	_, err = session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		return tx.Run(ctx, createQuery, map[string]interface{}{
			"userId":    userID,
			"partnerId": partnerID,
			"chatId":    newChatID,
		})
	})
	if err != nil {
		logger.Err(err).Error("Failed to create new direct chat relation in Neo4j")
		return "", err
	}

	return newChatID, nil
}

func (s *ChatService) CheckBlockStatus(userID, partnerID string) string {
	if db.Neo4jDriver == nil {
		return "NORMAL"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (u1:User {id: $userId})
		MATCH (u2:User {id: $partnerId})
		OPTIONAL MATCH (u1)-[blockOut:BLOCK]->(u2)
		OPTIONAL MATCH (u2)-[blockIn:BLOCK]->(u1)
		RETURN
			CASE
				WHEN blockOut IS NOT NULL THEN 'BLOCKED'
				WHEN blockIn IS NOT NULL THEN 'HAS_BEEN_BLOCKED'
				ELSE 'NORMAL'
			END AS blockStatus
	`

	val, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (interface{}, error) {
		res, err := tx.Run(ctx, query, map[string]interface{}{
			"userId":    userID,
			"partnerId": partnerID,
		})
		if err != nil {
			return "NORMAL", err
		}
		if res.Next(ctx) {
			if status, ok := res.Record().Values[0].(string); ok {
				return status, nil
			}
		}
		return "NORMAL", nil
	})
	if err != nil {
		logger.Err(err).Error("Failed to check block status in Neo4j")
		return "NORMAL"
	}
	return val.(string)
}

func (s *ChatService) enrichMessageResponsesWithPresignedURLs(ctx context.Context, msgs []*MessageResponse) {
	if len(msgs) == 0 {
		return
	}
	for _, m := range msgs {
		// FILE/VOICE: build full URL from Content (raw fileId), skip if already a full URL
		if (m.Type == "FILE" || m.Type == "VOICE") && m.Content != "" && !strings.HasPrefix(m.Content, "http://") && !strings.HasPrefix(m.Content, "https://") {
			url := fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, m.Content)
			m.Attachment = url
			m.Content = url
		}
		// GIF: attachment is already set to the external GIF URL (e.g. giphy.com), no processing needed
		if m.Sender != nil && m.Sender.ProfilePictureUrl != "" && !strings.HasPrefix(m.Sender.ProfilePictureUrl, "http://") && !strings.HasPrefix(m.Sender.ProfilePictureUrl, "https://") {
			m.Sender.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, m.Sender.ProfilePictureUrl)
		}
	}
}

func (s *ChatService) enrichChatRoomsWithPresignedURLs(ctx context.Context, rooms []ChatRoom) {
	if len(rooms) == 0 {
		return
	}
	for i := range rooms {
		if rooms[i].Target != nil && rooms[i].Target.ProfilePictureUrl != "" && !strings.HasPrefix(rooms[i].Target.ProfilePictureUrl, "http://") && !strings.HasPrefix(rooms[i].Target.ProfilePictureUrl, "https://") {
			rooms[i].Target.ProfilePictureUrl = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, rooms[i].Target.ProfilePictureUrl)
		}
		if rooms[i].IsGroup && rooms[i].Avatar != "" && !strings.HasPrefix(rooms[i].Avatar, "http://") && !strings.HasPrefix(rooms[i].Avatar, "https://") {
			rooms[i].Avatar = fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, rooms[i].Avatar)
		}
		if rooms[i].LatestMessage != nil {
			m := rooms[i].LatestMessage
			if (m.Type == "FILE" || m.Type == "VOICE") && m.Content != "" && !strings.HasPrefix(m.Content, "http://") && !strings.HasPrefix(m.Content, "https://") {
				url := fmt.Sprintf("%s/%s", s.cfg.FileServiceURL, m.Content)
				m.Attachment = url
				m.Content = url
			}
		}
	}
}

func (s *ChatService) SendToUser(userID string, payload interface{}) {
	s.mu.RLock()
	conn, online := s.connections[userID]
	mu := s.writeMutexes[userID]
	s.mu.RUnlock()

	if !online || conn == nil || mu == nil {
		return
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		logger.Err(err).Error("Failed to marshal single user payload")
		return
	}

	mu.Lock()
	_ = conn.WriteMessage(websocket.TextMessage, payloadBytes)
	mu.Unlock()
}

func (s *ChatService) IsInCall(ctx context.Context, username string) bool {
	if db.RedisClient == nil {
		return false
	}
	exists, err := db.RedisClient.Exists(ctx, "incall:"+username).Result()
	return err == nil && exists > 0
}

func (s *ChatService) IsPreparedForCall(ctx context.Context, caller, callee string) bool {
	if db.RedisClient == nil {
		return false
	}
	exists, err := db.RedisClient.Exists(ctx, "prepared_for_call:"+caller+":"+callee).Result()
	return err == nil && exists > 0
}

func (s *ChatService) PrepareCall(ctx context.Context, caller, callee string) {
	if db.RedisClient == nil {
		return
	}
	db.RedisClient.Set(ctx, "prepared_for_call:"+caller+":"+callee, "true", 2*time.Minute)
}

func (s *ChatService) InitCall(ctx context.Context, callerID string, calleeUsername string) error {
	if s.UserClient == nil {
		return fmt.Errorf("user client not initialized")
	}

	// Resolve caller username
	callerResp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: callerID})
	if err != nil || callerResp == nil {
		return exception.NewAppException(exception.UserNotFound)
	}
	callerUsername := callerResp.Username

	// Check caller call status
	if s.IsInCall(ctx, callerUsername) {
		return exception.NewAppException(exception.AlreadyInCall)
	}

	// Check callee call status
	if s.IsInCall(ctx, calleeUsername) {
		return exception.NewAppException(exception.TargetAlreadyInCall)
	}

	// Prepare call in Redis
	s.PrepareCall(ctx, callerUsername, calleeUsername)
	return nil
}

func (s *ChatService) StartGroupCall(ctx context.Context, callID string, callerUsername string, chatID string, isVideoCall bool) error {
	if s.UserClient == nil {
		return fmt.Errorf("user client not initialized")
	}

	if s.IsInCall(ctx, callerUsername) {
		return exception.NewAppException(exception.AlreadyInCall)
	}

	caller, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: callerUsername})
	if err != nil || caller == nil {
		return exception.NewAppException(exception.UserNotFound)
	}

	now := time.Now()
	content := "Cuộc gọi nhóm"
	if isVideoCall {
		content = "Cuộc gọi video nhóm"
	}

	falseVal := false
	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    caller.UserId,
		RecipientID: "", // Group message
		Content:     content,
		Timestamp:   now,
		Type:        "CALL",
		Status:      "SENT",
		CallID:      callID,
		CallAt:      &now,
		IsVideoCall: &isVideoCall,
		IsAnswered:  &falseVal,
	}
	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	// Broadcast call to all group members
	s.BroadcastToChat(chatID, enriched)

	// Update call state in Redis for the caller
	if db.RedisClient != nil {
		db.RedisClient.Set(ctx, "incall:"+callerUsername, callID, 0)
		db.RedisClient.SAdd(ctx, "call:"+callID, callerUsername)
		db.RedisClient.SAdd(ctx, "call_uuid:"+callID, caller.UserId)
	}

	return nil
}

func (s *ChatService) StartCall(ctx context.Context, callID string, callerUsername, calleeUsername string, isVideoCall bool) error {
	if s.UserClient == nil {
		return fmt.Errorf("user client not initialized")
	}

	anyInCall := s.IsInCall(ctx, callerUsername) || s.IsInCall(ctx, calleeUsername)
	preparedForCall := s.IsPreparedForCall(ctx, callerUsername, calleeUsername)
	if anyInCall || !preparedForCall {
		return exception.NewAppException(exception.NotReadyForCall)
	}

	// Resolve caller info
	caller, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: callerUsername})
	if err != nil || caller == nil {
		return exception.NewAppException(exception.UserNotFound)
	}

	// Resolve callee info
	callee, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: calleeUsername})
	if err != nil || callee == nil {
		return exception.NewAppException(exception.UserNotFound)
	}

	// Get or create direct chat
	chatID, err := s.GetOrCreateDirectChat(caller.UserId, callee.UserId)
	if err != nil {
		return err
	}

	// Create and save Call message to MongoDB
	now := time.Now()
	content := "Cuộc gọi thoại"
	if isVideoCall {
		content = "Cuộc gọi video"
	}

	falseVal := false
	msg := &model.Message{
		ID:          uuid.New().String(),
		ChatID:      chatID,
		SenderID:    caller.UserId,
		RecipientID: callee.UserId,
		Content:     content,
		Timestamp:   now,
		Type:        "CALL",
		Status:      "SENT",
		CallID:      callID,
		CallAt:      &now,
		IsVideoCall: &isVideoCall,
		IsAnswered:  &falseVal,
	}
	s.SaveMessage(msg)

	enriched := s.EnrichMessage(msg)

	// Enrich with presigned URLs (consistent with other messages)
	s.enrichMessageResponsesWithPresignedURLs(ctx, []*MessageResponse{enriched})

	// Broadcast call notification to chat and callee
	s.BroadcastToChat(chatID, enriched)
	s.SendToUser(callee.UserId, enriched)

	// Update call state in Redis
	if db.RedisClient != nil {
		db.RedisClient.Set(ctx, "incall:"+callerUsername, callID, 0)
		db.RedisClient.Set(ctx, "incall:"+calleeUsername, callID, 0)
		db.RedisClient.SAdd(ctx, "call:"+callID, calleeUsername, callerUsername)
		db.RedisClient.SAdd(ctx, "call_uuid:"+callID, caller.UserId, callee.UserId)
	}

	return nil
}

func (s *ChatService) AnswerCall(ctx context.Context, callID string) error {
	if db.MsgCollection == nil {
		return fmt.Errorf("message collection not initialized")
	}

	now := time.Now()
	answered := true
	_, err := db.MsgCollection.UpdateOne(ctx, bson.M{"call_id": callID}, bson.M{
		"$set": bson.M{
			"is_answered": &answered,
			"answer_at":   &now,
		},
	})
	return err
}

func (s *ChatService) AnswerGroupCall(ctx context.Context, callID string, userID string) error {
	if db.RedisClient == nil {
		return nil
	}

	// Resolve username
	user, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{UserId: userID})
	if err != nil || user == nil {
		return nil
	}

	db.RedisClient.Set(ctx, "incall:"+user.Username, callID, 0)
	db.RedisClient.SAdd(ctx, "call:"+callID, user.Username)
	db.RedisClient.SAdd(ctx, "call_uuid:"+callID, userID)
	return nil
}

func (s *ChatService) RejectCall(ctx context.Context, callID string, userID string) error {
	if db.MsgCollection == nil {
		return fmt.Errorf("message collection not initialized")
	}

	now := time.Now()
	rejected := true
	_, err := db.MsgCollection.UpdateOne(ctx, bson.M{"call_id": callID}, bson.M{
		"$set": bson.M{
			"is_rejected": &rejected,
			"end_at":      &now,
		},
	})
	if err != nil {
		return err
	}

	return s.EndCall(ctx, callID)
}

func (s *ChatService) EndCall(ctx context.Context, callID string) error {
	if db.MsgCollection == nil {
		return fmt.Errorf("message collection not initialized")
	}

	// Update endAt in MongoDB
	now := time.Now()
	_, err := db.MsgCollection.UpdateOne(ctx, bson.M{"call_id": callID}, bson.M{
		"$set": bson.M{
			"end_at": &now,
		},
	})
	if err != nil {
		logger.Err(err).Error("Failed to update end_at in MongoDB for call %s", callID)
	}

	if db.RedisClient == nil {
		return nil
	}

	// Retrieve call members and user IDs from Redis
	usernames, _ := db.RedisClient.SMembers(ctx, "call:"+callID).Result()
	userIDs, _ := db.RedisClient.SMembers(ctx, "call_uuid:"+callID).Result()

	if len(usernames) > 0 {
		for _, uname := range usernames {
			db.RedisClient.Del(ctx, "incall:"+uname)
		}

		// Also clean up prepared flags (only relevant for 1-1 but safe to attempt)
		for _, u1 := range usernames {
			for _, u2 := range usernames {
				if u1 != u2 {
					db.RedisClient.Del(ctx, "prepared_for_call:"+u1+":"+u2)
				}
			}
		}

		db.RedisClient.Del(ctx, "call:"+callID)
		db.RedisClient.Del(ctx, "call_uuid:"+callID)

		// Broadcast END_CALL command to all members
		payload := map[string]interface{}{
			"command": "END_CALL",
			"id":      callID,
		}
		for _, uid := range userIDs {
			s.SendToUser(uid, payload)
		}
	} else {
		logger.Warn("No members to end call %s in Redis", callID)
	}

	return nil
}

func (s *ChatService) EndCallByMemberUsername(ctx context.Context, username string) error {
	if db.RedisClient == nil {
		return nil
	}
	callID, err := db.RedisClient.Get(ctx, "incall:"+username).Result()
	if err != nil || callID == "" {
		return nil
	}
	return s.EndCall(ctx, callID)
}
