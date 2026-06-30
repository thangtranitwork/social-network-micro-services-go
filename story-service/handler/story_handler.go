package handler

import (
	"net/http"
	"time"

	"social-network-go/logger"
	"social-network-go/story-service/service"

	"github.com/gin-gonic/gin"
)

type StoryHandler struct {
	StorySvc *service.StoryService
}

func NewStoryHandler(storySvc *service.StoryService) *StoryHandler {
	return &StoryHandler{StorySvc: storySvc}
}

type CreateStoryRequest struct {
	MediaUrl  string `json:"mediaUrl" binding:"required"`
	MediaType string `json:"mediaType"` // IMAGE or VIDEO
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func (h *StoryHandler) CreateStory(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{
			Code:      401,
			Message:   "Unauthorized",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	var req CreateStoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Invalid input data: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	story, err := h.StorySvc.CreateStory(c.Request.Context(), userID, req.MediaUrl, req.MediaType)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("CreateStory failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Internal Server Error: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      story,
	})
}

func (h *StoryHandler) GetStoryFeed(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{
			Code:      401,
			Message:   "Unauthorized",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	feed, err := h.StorySvc.GetStoryFeed(c.Request.Context(), userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetStoryFeed failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Internal Server Error: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      feed,
	})
}

func (h *StoryHandler) DeleteStory(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{
			Code:      401,
			Message:   "Unauthorized",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	storyID := c.Param("id")
	err := h.StorySvc.DeleteStory(c.Request.Context(), userID, storyID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("DeleteStory failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Internal Server Error: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func getCurrentUser(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	return userID
}
