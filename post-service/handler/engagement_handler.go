package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/post-service/model"
	"social-network-go/post-service/service"
)

func (h *PostHandler) RecordMediaWatch(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED", Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	var req model.MediaWatchEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{Code: 400, Message: "INVALID_MEDIA_WATCH_EVENT", Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	if err := h.PostSvc.RecordMediaWatch(c.Request.Context(), userID, req); err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error(), Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	c.JSON(http.StatusAccepted, ApiResponse{Code: 202, Message: "ACCEPTED", Timestamp: time.Now().Format(time.RFC3339)})
}

func (h *PostHandler) RecordNegativeSignal(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED", Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	var req model.NegativeSignalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{Code: 400, Message: "INVALID_NEGATIVE_SIGNAL", Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	if err := h.PostSvc.RecordNegativeSignal(c.Request.Context(), userID, req); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, service.ErrInvalidSignalType) {
			status = http.StatusBadRequest
		}
		c.JSON(status, ApiResponse{Code: status, Message: err.Error(), Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	sendSuccess(c, gin.H{"recorded": true})
}

func (h *PostHandler) UnmuteAuthor(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED", Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	authorID := c.Param("authorId")
	if err := h.PostSvc.UnmuteAuthor(c.Request.Context(), userID, authorID); err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error(), Timestamp: time.Now().Format(time.RFC3339)})
		return
	}
	sendSuccess(c, gin.H{"unmuted": true})
}
