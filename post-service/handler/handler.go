package handler

import (
	"fmt"
	"net/http"
	"time"

	"social-network-go/post-service/service"

	"github.com/gin-gonic/gin"
)

type PostHandler struct {
	PostSvc *service.PostService
}

func NewPostHandler(postSvc *service.PostService) *PostHandler {
	return &PostHandler{PostSvc: postSvc}
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
		userID = c.Query("current_user_id")
	}
	return userID
}

func getPageable(c *gin.Context) service.Pageable {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	pageType := c.DefaultQuery("type", "RELEVANT")

	var skip, limit int
	fmt.Sscanf(skipStr, "%d", &skip)
	fmt.Sscanf(limitStr, "%d", &limit)

	return service.Pageable{
		Skip:  int64(skip),
		Limit: int64(limit),
		Type:  pageType,
	}
}
