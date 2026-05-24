package handler

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/chat-service/service"

	"github.com/golang-jwt/jwt/v5"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type ChatHandler struct {
	ChatSvc  *service.ChatService
	Upgrader websocket.Upgrader
}

func NewChatHandler(chatSvc *service.ChatService) *ChatHandler {
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

func sendError(c *gin.Context, httpStatus int, code int, msg string) {
	c.JSON(httpStatus, ApiResponse{
		Code:      code,
		Message:   msg,
		Timestamp: time.Now().Format(time.RFC3339),
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
		sendError(c, http.StatusBadRequest, 400, "USER_ID_REQUIRED")
		return
	}

	// Upgrade the connection to WebSocket
	conn, err := h.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to upgrade connection"})
		return
	}

	h.ChatSvc.RegisterClient(userID, conn)
	go h.ChatSvc.HandleIncomingMessages(userID)
}

func (h *ChatHandler) GetChatHistory(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	partnerID := c.Param("partnerId")
	if partnerID == "" {
		sendError(c, http.StatusBadRequest, 400, "PARTNER_ID_REQUIRED")
		return
	}

	history := h.ChatSvc.GetChatHistory(userID, partnerID)
	sendSuccess(c, history)
}

func (h *ChatHandler) GetChatList(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
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
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	chatID := c.Param("chatId")
	if chatID == "" {
		sendError(c, http.StatusBadRequest, 400, "CHAT_ID_REQUIRED")
		return
	}

	if !h.ChatSvc.IsMemberOfChat(userID, chatID) {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
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
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
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
		sendError(c, http.StatusInternalServerError, 500, "FAILED_TO_GENERATE_TOKEN")
		return
	}

	c.JSON(http.StatusOK, gin.H{"access_token": tokenString})
}

func (h *ChatHandler) SearchChats(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}
	query := c.Query("query")
	rooms := h.ChatSvc.SearchChats(userID, query)
	sendSuccess(c, rooms)
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		Text     string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	resp, err := h.ChatSvc.SendMessage(userID, req.Username, req.Text)
	if err != nil {
		if err.Error() == "BLOCKED" {
			sendError(c, http.StatusForbidden, 403, "BLOCKED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendGif(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		URL      string `json:"url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	resp, err := h.ChatSvc.SendGif(userID, req.Username, req.URL)
	if err != nil {
		if err.Error() == "BLOCKED" {
			sendError(c, http.StatusForbidden, 403, "BLOCKED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendFile(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		FileId   string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	resp, err := h.ChatSvc.SendFile(userID, req.Username, req.FileId)
	if err != nil {
		if err.Error() == "BLOCKED" {
			sendError(c, http.StatusForbidden, 403, "BLOCKED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) SendVoice(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		Username string `json:"username" binding:"required"`
		FileId   string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	resp, err := h.ChatSvc.SendVoice(userID, req.Username, req.FileId)
	if err != nil {
		if err.Error() == "BLOCKED" {
			sendError(c, http.StatusForbidden, 403, "BLOCKED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, resp)
}

func (h *ChatHandler) EditMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		MessageID string `json:"messagesId" binding:"required"`
		Text      string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	err := h.ChatSvc.EditMessage(req.MessageID, req.Text, userID)
	if err != nil {
		if err.Error() == "UNAUTHORIZED" {
			sendError(c, http.StatusForbidden, 403, "UNAUTHORIZED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, nil)
}

func (h *ChatHandler) DeleteMessage(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	messageID := c.Param("messageId")
	if messageID == "" {
		sendError(c, http.StatusBadRequest, 400, "MESSAGE_ID_REQUIRED")
		return
	}

	err := h.ChatSvc.DeleteMessage(messageID, userID)
	if err != nil {
		if err.Error() == "UNAUTHORIZED" {
			sendError(c, http.StatusForbidden, 403, "UNAUTHORIZED")
			return
		}
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}
	sendSuccess(c, nil)
}
