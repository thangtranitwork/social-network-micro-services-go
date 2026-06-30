package handler

import (
	"net/http"
	"time"

	"social-network-go/logger"
	"social-network-go/search-service/service"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	SearchSvc *service.SearchService
}

func NewSearchHandler(searchSvc *service.SearchService) *SearchHandler {
	return &SearchHandler{SearchSvc: searchSvc}
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("query")
	userID := getCurrentUser(c)

	results, err := h.SearchSvc.Search(c.Request.Context(), query, userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Search failed")
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
		Body:      results,
	})
}

func getCurrentUser(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	return userID
}
