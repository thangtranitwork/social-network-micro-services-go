package handler

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/chat-service/model"
	"social-network-go/chat-service/service"
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ChatServiceInterface interface {
	RegisterClient(userID string, conn *websocket.Conn)
	HandleIncomingMessages(userID string)
	GetChatHistory(userID, partnerID string) []*model.Message
	GetChatList(userID string) []service.ChatRoom
	IsMemberOfChat(userID, chatID string) bool
	GetChatMessages(chatID string, skip, limit int64) []*service.MessageResponse
	MarkMessagesAsRead(chatID, userID string)
	SearchChats(userID, searchQuery string) []service.ChatRoom
	SendMessage(senderID, receiverUsername, text string) (*service.MessageResponse, error)
	SendGif(senderID, receiverUsername, gifURL string) (*service.MessageResponse, error)
	SendFile(senderID, receiverUsername, fileID string) (*service.MessageResponse, error)
	SendVoice(senderID, receiverUsername, fileID string) (*service.MessageResponse, error)
	DeleteMessage(messageID, userID string) error
	EditMessage(messageID, newContent, userID string) error
	UpdateGroupChat(ctx context.Context, adminID string, chatID string, name string, avatar string) error
	CreateGroupChat(ctx context.Context, adminID string, name string, memberIDs []string) (string, error)
	AddMembersToGroup(ctx context.Context, adminID string, chatID string, memberIDs []string) error
	RemoveMemberFromGroup(ctx context.Context, adminID string, chatID string, userID string) error
	GetChatMembersDetails(chatID string) []*service.ChatUser
}

type ChatHandler struct {
	ChatSvc  ChatServiceInterface
	Upgrader websocket.Upgrader
}

func NewChatHandler(chatSvc ChatServiceInterface) *ChatHandler {
	return &ChatHandler{
		ChatSvc: chatSvc,
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
					// Fallback to allow localhost and 127.0.0.1 by default
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
	}
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func sendSuccess(c *gin.Context, body interface{}) {
	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      body,
	})
}

func getCurrentUser(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		appEnv := strings.ToLower(os.Getenv("APP_ENV"))
		if appEnv != "production" && appEnv != "prod" && appEnv != "staging" {
			userID = c.Query("userId")
		}
	}
	return userID
}

func (h *ChatHandler) HandleWebSocket(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	// Upgrade the connection to WebSocket
	conn, err := h.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Failed to upgrade websocket connection")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return
	}

	h.ChatSvc.RegisterClient(userID, conn)
	go h.ChatSvc.HandleIncomingMessages(userID)
}

func (h *ChatHandler) GetChatHistory(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	partnerID := c.Param("partnerId")
	if partnerID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	history := h.ChatSvc.GetChatHistory(userID, partnerID)
	sendSuccess(c, history)
}

func (h *ChatHandler) GetChatList(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}
	rooms := h.ChatSvc.GetChatList(userID)
	if rooms == nil {
		rooms = []service.ChatRoom{}
	}
	sendSuccess(c, rooms)
}

func (h *ChatHandler) GetChatMessages(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatID := c.Param("chatId")
	if chatID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if !h.ChatSvc.IsMemberOfChat(userID, chatID) {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")

	skip, _ := strconv.ParseInt(skipStr, 10, 64)
	limit, _ := strconv.ParseInt(limitStr, 10, 64)

	messages := h.ChatSvc.GetChatMessages(chatID, skip, limit)

	if skip <= limit {
		h.ChatSvc.MarkMessagesAsRead(chatID, userID)
	}

	sendSuccess(c, messages)
}

// GetICEServers returns STUN/TURN ICE server configuration for WebRTC clients.
func (h *ChatHandler) GetICEServers(c *gin.Context) {
	cfg := config.LoadConfig()

	iceServers := []gin.H{
		// Public Google STUN (always included)
		{"urls": []string{"stun:stun.l.google.com:19302", "stun:stun1.l.google.com:19302"}},
	}

	// Append TURN server only when configured
	if cfg.TurnURL != "" {
		iceServers = append(iceServers, gin.H{
			"urls":       []string{cfg.TurnURL},
			"username":   cfg.TurnUser,
			"credential": cfg.TurnPass,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"iceServers": iceServers,
	})
}

func (h *ChatHandler) SearchChats(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}
	query := c.Query("query")
	rooms := h.ChatSvc.SearchChats(userID, query)
	sendSuccess(c, rooms)
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Text     string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	resp, err := h.ChatSvc.SendMessage(userID, req.Username, req.Text)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SendMessage failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SendMessageFailed)
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendGif(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		URL      string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	resp, err := h.ChatSvc.SendGif(userID, req.Username, req.URL)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SendGif failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SendGifFailed)
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendFile(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		FileId   string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	resp, err := h.ChatSvc.SendFile(userID, req.Username, req.FileId)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SendFile failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SendFileFailed)
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendVoice(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		FileId   string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	resp, err := h.ChatSvc.SendVoice(userID, req.Username, req.FileId)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SendVoice failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SendVoiceFailed)
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) EditMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		MessageID string `json:"messagesId" binding:"required"`
		Text      string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.ChatSvc.EditMessage(req.MessageID, req.Text, userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("EditMessage failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.EditMessageFailed)
		return
	}
	sendSuccess(c, nil)
}

func (h *ChatHandler) DeleteMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	messageID := c.Param("messageId")
	if messageID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.ChatSvc.DeleteMessage(messageID, userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("DeleteMessage failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.DeleteMessageFailed)
		return
	}
	sendSuccess(c, nil)
}

func (h *ChatHandler) CreateGroupChat(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = getCurrentUser(c)
	}
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		Name      string   `json:"name"`
		MemberIDs []string `json:"memberIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if req.Name == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatID, err := h.ChatSvc.CreateGroupChat(c.Request.Context(), userID, req.Name, req.MemberIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"chatId": chatID})
}

func (h *ChatHandler) AddMembersToGroup(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = getCurrentUser(c)
	}
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatID := c.Param("chatId")
	if chatID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	var req struct {
		MemberIDs []string `json:"memberIds"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.ChatSvc.AddMembersToGroup(c.Request.Context(), userID, chatID, req.MemberIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) RemoveMemberFromGroup(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = getCurrentUser(c)
	}
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatID := c.Param("chatId")
	targetUserID := c.Param("userId")
	if chatID == "" || targetUserID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.ChatSvc.RemoveMemberFromGroup(c.Request.Context(), userID, chatID, targetUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) UpdateGroupChat(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = getCurrentUser(c)
	}
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatID := c.Param("chatId")
	if chatID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	var req struct {
		Name   string `json:"name"`
		Avatar string `json:"avatar"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.ChatSvc.UpdateGroupChat(c.Request.Context(), userID, chatID, req.Name, req.Avatar)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) GetGroupMembers(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = getCurrentUser(c)
	}
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatID := c.Param("chatId")
	if chatID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if !h.ChatSvc.IsMemberOfChat(userID, chatID) {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	members := h.ChatSvc.GetChatMembersDetails(chatID)
	sendSuccess(c, members)
}
