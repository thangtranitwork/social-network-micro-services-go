package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
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
}

func NewChatService(cfg *config.Config) *ChatService {
	var userClient pb.UserServiceClient
	conn, err := grpc.Dial(cfg.UserGrpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Printf("Warning: Failed to connect to User gRPC at %s: %v.", cfg.UserGrpcAddr, err)
	} else {
		userClient = pb.NewUserServiceClient(conn)
		log.Printf("Chat Service connected to User gRPC Service at %s", cfg.UserGrpcAddr)
	}

	return &ChatService{
		connections:  make(map[string]*websocket.Conn),
		writeMutexes: make(map[string]*sync.Mutex),
		activeChats:  make(map[string]string),
		UserClient:   userClient,
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
	log.Printf("User %s connected to WebSocket. Active connections: %d", userID, count)

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
		log.Printf("User %s disconnected. Active connections: %d", userID, count)

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
		log.Printf("User %s is now ONLINE (Redis updated)", userID)
		s.broadcastOnlineStatus(userID, true)
	} else if err != nil {
		log.Printf("Failed to increment online counter in Redis: %v", err)
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
		log.Printf("Failed to decrement online counter in Redis: %v", err)
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
		log.Printf("User %s is now OFFLINE (Redis updated)", userID)
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
			if conn, isOnline := s.connections[room.Target.ID]; isOnline {
				if mu := s.writeMutexes[room.Target.ID]; mu != nil {
					targets = append(targets, targetConn{conn: conn, mu: mu})
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
			log.Printf("Error reading WebSocket message from %s: %v", userID, err)
			break
		}

		// Parse the message using fields from the frontend STOMP payload (chatId, username, text)
		var req struct {
			Command  string `json:"command"`
			ChatID   string `json:"chatId"`
			Username string `json:"username"`
			Text     string `json:"text"`
		}
		if err := json.Unmarshal(messageBytes, &req); err != nil {
			log.Printf("Error unmarshalling chat message: %v", err)
			continue
		}

		if req.Command == "SUBSCRIBE" {
			if req.ChatID != "" {
				s.mu.Lock()
				s.activeChats[userID] = req.ChatID
				s.mu.Unlock()
				log.Printf("User %s subscribed to chat %s", userID, req.ChatID)

				// Mark messages as read on subscribe!
				s.MarkMessagesAsRead(req.ChatID, userID)
			}
			continue
		}

		if req.Command == "UNSUBSCRIBE" {
			s.mu.Lock()
			if currentChat, ok := s.activeChats[userID]; ok && currentChat == req.ChatID {
				delete(s.activeChats, userID)
				log.Printf("User %s unsubscribed from chat %s", userID, req.ChatID)
			}
			s.mu.Unlock()
			continue
		}

		if req.Command == "TYPING" || req.Command == "STOP_TYPING" {
			if req.ChatID != "" {
				s.BroadcastToChat(req.ChatID, map[string]interface{}{
					"command": req.Command,
					"id":      userID,
				})
			}
			continue
		}

		// Resolve recipient ID
		var recipientID string
		if req.Username != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if s.UserClient != nil {
				resp, err := s.UserClient.GetCommonUserInfo(ctx, &pb.UserRequest{Username: req.Username})
				if err == nil && resp != nil {
					recipientID = resp.UserId
				} else {
					log.Printf("Failed to resolve username %s: %v", req.Username, err)
				}
			}
			cancel()
		}

		// Fallback to resolve from ChatID members if username not provided
		if recipientID == "" && req.ChatID != "" {
			members := s.GetChatMembers(req.ChatID)
			for _, m := range members {
				if m != userID {
					recipientID = m
					break
				}
			}
		}

		if recipientID == "" {
			log.Printf("Could not resolve recipient for message from %s, skipping message", userID)
			continue
		}

		// 1. Validate empty text content
		trimmed := strings.TrimSpace(req.Text)
		if trimmed == "" {
			log.Printf("Validation error: text content is required for user %s", userID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.TextMessageContentRequired.Marshal())
			continue
		}

		// 2. Validate content length
		if len([]rune(trimmed)) > 10000 {
			log.Printf("Validation error: message content length exceeds limit for user %s", userID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.InvalidMessageContentLength.Marshal())
			continue
		}

		// 3. Validate block relationship
		blockStatus := s.CheckBlockStatus(userID, recipientID)
		if blockStatus == "BLOCKED" {
			log.Printf("Validation error: user %s has blocked recipient %s", userID, recipientID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.HasBlocked.Marshal())
			continue
		} else if blockStatus == "HAS_BEEN_BLOCKED" {
			log.Printf("Validation error: user %s has been blocked by recipient %s", userID, recipientID)
			_ = conn.WriteMessage(websocket.TextMessage, exception.HasBeenBlocked.Marshal())
			continue
		}

		// Get or create chatID if not provided
		chatID := req.ChatID
		if chatID == "" {
			var err error
			chatID, err = s.GetOrCreateDirectChat(userID, recipientID)
			if err != nil {
				log.Printf("Failed to get or create chat for %s -> %s: %v", userID, recipientID, err)
				continue
			}
		}

		// Determine status based on active chat subscription and online status
		s.mu.RLock()
		activeChat, inChat := s.activeChats[recipientID]
		_, online := s.connections[recipientID]
		s.mu.RUnlock()

		status := "SENT"
		if inChat && activeChat == chatID {
			status = "READ"
		}

		msg := &model.Message{
			ID:          uuid.New().String(),
			ChatID:      chatID,
			SenderID:    userID,
			RecipientID: recipientID,
			Content:     strings.TrimSpace(req.Text),
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
		s.BroadcastToChat(chatID, enriched)

		if !online {
			log.Printf("Recipient %s is offline. Message cached for delivery on login.", recipientID)
		}
	}
}

func (s *ChatService) SaveMessage(msg *model.Message) {
	if db.MsgCollection == nil {
		log.Println("[WARN] MsgCollection is nil, skipping SaveMessage")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := db.MsgCollection.InsertOne(ctx, msg)
	if err != nil {
		log.Printf("Failed to save message to MongoDB: %v", err)
		return
	}
	log.Printf("[DB SAVE] Message %s from %s -> %s saved", msg.ID, msg.SenderID, msg.RecipientID)
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
		log.Printf("Failed to update message status in MongoDB: %v", err)
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
		log.Printf("Failed to fetch chat history: %v", err)
		return []*model.Message{}
	}
	defer cursor.Close(ctx)

	var history []*model.Message
	if err := cursor.All(ctx, &history); err != nil {
		log.Printf("Failed to decode chat history: %v", err)
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
	if db.Neo4jDriver == nil {
		log.Println("[WARN] Neo4j driver is nil, returning empty chat list")
		return []ChatRoom{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (currentUser:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)<-[:IS_MEMBER_OF]-(target:User)
		WHERE target.id <> $userId
		RETURN chat.id, target.id, target.username, target.givenName, target.familyName, target.profilePictureId
	`

	type Neo4jChat struct {
		ChatID           string
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
			tid, _ := vals[1].(string)
			tname, _ := vals[2].(string)
			gname, _ := vals[3].(string)
			fname, _ := vals[4].(string)
			pimg, _ := vals[5].(string)
			chats = append(chats, Neo4jChat{
				ChatID:           cid,
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
		log.Printf("Failed to fetch chat list from Neo4j: %v", err)
		return []ChatRoom{}
	}

	neoChats := neoChatsVal.([]Neo4jChat)
	var rooms []ChatRoom

	for _, nc := range neoChats {
		// 1. Get latest message from MongoDB
		var lastMsg *model.Message
		if db.MsgCollection != nil {
			opts := options.FindOne().SetSort(bson.M{"timestamp": -1})
			var m model.Message
			err := db.MsgCollection.FindOne(ctx, bson.M{"chat_id": nc.ChatID}, opts).Decode(&m)
			if err == nil {
				lastMsg = &m
			}
		}

		// 2. Count unread messages from MongoDB
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

		// 3. Online status
		s.mu.RLock()
		_, isOnline := s.connections[nc.TargetID]
		s.mu.RUnlock()

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
				Type:    lastMsg.Type,
				IsRead:  lastMsg.Status == "READ",
				Deleted: lastMsg.Content == "deleted",
			}
		}

		fullName := nc.GivenName + " " + nc.FamilyName
		if fullName == " " {
			fullName = nc.TargetUser
		}

		rooms = append(rooms, ChatRoom{
			ChatID: nc.ChatID,
			Name:   fullName,
			Target: &ChatUser{
				ID:                nc.TargetID,
				Username:          nc.TargetUser,
				GivenName:         nc.GivenName,
				FamilyName:        nc.FamilyName,
				ProfilePictureUrl: nc.ProfilePictureId,
				IsOnline:          isOnline,
			},
			LatestMessage:       latestMsgResp,
			NotReadMessageCount: unreadCount,
			UpdatedAt:           updatedAt,
		})
	}

	sort.Slice(rooms, func(i, j int) bool {
		return rooms[i].UpdatedAt.After(rooms[j].UpdatedAt)
	})

	s.enrichChatRoomsWithPresignedURLs(ctx, rooms)

	return rooms
}

func (s *ChatService) IsMemberOfChat(userID, chatID string) bool {
	if db.Neo4jDriver == nil {
		log.Println("[WARN] Neo4j driver is nil, allowing IsMemberOfChat by default")
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
		log.Printf("Failed to verify chat membership in Neo4j: %v", err)
		return false
	}
	return res.(bool)
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
		log.Printf("Failed to fetch messages for chat %s: %v", chatID, err)
		return []*MessageResponse{}
	}
	defer cursor.Close(ctx)

	var msgs []*model.Message
	if err := cursor.All(ctx, &msgs); err != nil {
		log.Printf("Failed to decode messages for chat %s: %v", chatID, err)
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
		})
	}

	s.enrichMessageResponsesWithPresignedURLs(ctx, response)

	return response
}

func (s *ChatService) MarkMessagesAsRead(chatID, userID string) {
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
		log.Printf("Failed to mark messages as read for chat %s, user %s: %v", chatID, userID, err)
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
			log.Printf("Failed to mark messages as read in Neo4j: %v", err)
		}
	}

	// Broadcast READING command to notify chat members
	s.BroadcastToChat(chatID, map[string]interface{}{
		"command": "READING",
		"id":      userID,
	})
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
		log.Printf("Failed to get chat members from Neo4j: %v", err)
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
		return nil, fmt.Errorf("failed to create chat: %w", err)
	}

	s.mu.RLock()
	activeChat, inChat := s.activeChats[receiverResp.UserId]
	s.mu.RUnlock()

	status := "SENT"
	if inChat && activeChat == chatID {
		status = "READ"
	}
	log.Printf("saving message to DB with content %s", text)
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
		log.Println("[WARN] Neo4j driver is nil, returning empty chat list")
		return []ChatRoom{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	session := db.Neo4jDriver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	query := `
		MATCH (currentUser:User {id: $userId})-[:IS_MEMBER_OF]->(chat:Chat)<-[:IS_MEMBER_OF]-(target:User)
		WHERE target.id <> $userId
		AND (
			toLower(target.givenName + ' ' + target.familyName) CONTAINS toLower($searchQuery) OR
			toLower(target.username) CONTAINS toLower($searchQuery)
		)
		RETURN chat.id, target.id, target.username, target.givenName, target.familyName, target.profilePictureId
	`

	type Neo4jChat struct {
		ChatID           string
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
			tid, _ := vals[1].(string)
			tname, _ := vals[2].(string)
			gname, _ := vals[3].(string)
			fname, _ := vals[4].(string)
			pimg, _ := vals[5].(string)
			chats = append(chats, Neo4jChat{
				ChatID:           cid,
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
		log.Printf("Failed to search chat list from Neo4j: %v", err)
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

		s.mu.RLock()
		_, isOnline := s.connections[nc.TargetID]
		s.mu.RUnlock()

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

		fullName := nc.GivenName + " " + nc.FamilyName
		if fullName == " " {
			fullName = nc.TargetUser
		}

		rooms = append(rooms, ChatRoom{
			ChatID: nc.ChatID,
			Name:   fullName,
			Target: &ChatUser{
				ID:                nc.TargetID,
				Username:          nc.TargetUser,
				GivenName:         nc.GivenName,
				FamilyName:        nc.FamilyName,
				ProfilePictureUrl: nc.ProfilePictureId,
				IsOnline:          isOnline,
			},
			LatestMessage:       latestMsgResp,
			NotReadMessageCount: unreadCount,
			UpdatedAt:           updatedAt,
		})
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
		return "NORMAL"
	}
	return val.(string)
}

func (s *ChatService) enrichMessageResponsesWithPresignedURLs(ctx context.Context, msgs []*MessageResponse) {
	if s.FileClient == nil || len(msgs) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)

	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, m := range msgs {
		if m.Type == "FILE" || m.Type == "VOICE" {
			addFileID(m.Content)
		}
		if m.Sender != nil {
			addFileID(m.Sender.ProfilePictureUrl)
		}
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for messages: %v", err)
		return
	}

	for _, m := range msgs {
		if m.Type == "FILE" || m.Type == "VOICE" {
			if url, ok := urls[m.Content]; ok {
				m.Attachment = url
				m.Content = url
			}
		}
		if m.Sender != nil {
			if url, ok := urls[m.Sender.ProfilePictureUrl]; ok {
				m.Sender.ProfilePictureUrl = url
			}
		}
	}
}

func (s *ChatService) enrichChatRoomsWithPresignedURLs(ctx context.Context, rooms []ChatRoom) {
	if s.FileClient == nil || len(rooms) == 0 {
		return
	}
	fileIDs := make([]string, 0)
	fileIDSet := make(map[string]bool)

	addFileID := func(id string) {
		if id != "" && !fileIDSet[id] {
			fileIDs = append(fileIDs, id)
			fileIDSet[id] = true
		}
	}

	for _, r := range rooms {
		addFileID(r.Target.ProfilePictureUrl)
		if r.LatestMessage != nil {
			if r.LatestMessage.Type == "FILE" || r.LatestMessage.Type == "VOICE" {
				addFileID(r.LatestMessage.Content)
			}
		}
	}

	if len(fileIDs) == 0 {
		return
	}

	urls, err := s.FileClient.GetPresignedURLs(ctx, fileIDs)
	if err != nil {
		log.Printf("Error getting presigned URLs for chat rooms: %v", err)
		return
	}

	for i := range rooms {
		if url, ok := urls[rooms[i].Target.ProfilePictureUrl]; ok {
			rooms[i].Target.ProfilePictureUrl = url
		}
		if rooms[i].LatestMessage != nil {
			if rooms[i].LatestMessage.Type == "FILE" || rooms[i].LatestMessage.Type == "VOICE" {
				if url, ok := urls[rooms[i].LatestMessage.Content]; ok {
					rooms[i].LatestMessage.Attachment = url
					rooms[i].LatestMessage.Content = url
				}
			}
		}
	}
}
