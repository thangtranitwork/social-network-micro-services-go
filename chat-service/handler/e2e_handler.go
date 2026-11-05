package handler

import (
	"net/http"

	"social-network-go/chat-service/service"
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

func (h *ChatHandler) RegisterE2EKeys(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req service.E2ERegisterKeysRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	err := chatSvc.RegisterDeviceKeys(c.Request.Context(), userID, &req)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("RegisterDeviceKeys failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) AddOneTimePreKeys(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req service.E2EAddPreKeysRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	count, err := chatSvc.AddOneTimePreKeys(c.Request.Context(), userID, &req)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("AddOneTimePreKeys failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"remainingCount": count})
}

func (h *ChatHandler) GetE2EKeyStatus(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	status, err := chatSvc.GetE2EKeyStatus(c.Request.Context(), userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetE2EKeyStatus failed")
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, status)
}

func (h *ChatHandler) GetPreKeyBundle(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	targetUserID := c.Param("targetUserId")
	if targetUserID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	bundle, err := chatSvc.GetPreKeyBundle(c.Request.Context(), targetUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Warn("GetPreKeyBundle failed for %s: %v", targetUserID, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, bundle)
}

func (h *ChatHandler) RevokeDevice(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	deviceID := c.Param("deviceId")
	if deviceID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	err := chatSvc.RevokeDevice(c.Request.Context(), userID, deviceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) SaveKeyBackup(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req service.E2EBackupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	err := chatSvc.SaveKeyBackup(c.Request.Context(), userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) GetKeyBackup(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	backup, err := chatSvc.GetKeyBackup(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, backup)
}

func (h *ChatHandler) SaveGroupSenderKeys(c *gin.Context) {
	userID := getCurrentUser(c)
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

	var req service.E2EGroupSenderKeysBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	err := chatSvc.SaveGroupSenderKeys(c.Request.Context(), userID, chatID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, gin.H{"status": "success"})
}

func (h *ChatHandler) GetGroupSenderKeys(c *gin.Context) {
	userID := getCurrentUser(c)
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

	recipientDeviceID := c.Query("deviceId")

	chatSvc, ok := h.ChatSvc.(*service.ChatService)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "ChatService internal error"})
		return
	}

	keys, err := chatSvc.GetGroupSenderKeys(c.Request.Context(), userID, recipientDeviceID, chatID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sendSuccess(c, keys)
}
