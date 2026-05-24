package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/chat-service/model"
	"social-network-go/chat-service/service"
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/golang-jwt/jwt/v5"

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
				// Allow all origins for the websocket upgrade
				return true
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
		userID = c.Query("userId")
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

func (h *ChatHandler) CreateStringeeToken(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	cfg := config.LoadConfig()
	now := time.Now()
	exp := now.Add(24 * time.Hour).Unix()

	claims := jwt.MapClaims{
		"jti":       fmt.Sprintf("%s-%d", cfg.StringeeSid, now.Unix()),
		"iss":       cfg.StringeeSid,
		"exp":       exp,
		"userId":    userID,
		"rest_api":  true,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(cfg.StringeeSecret))
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Failed to generate Stringee token")
		exception.SendError(c, exception.AuthenticationFailed)
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": tokenString})
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
