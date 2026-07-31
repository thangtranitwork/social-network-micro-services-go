package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"social-network-go/recommendation-service/model"
	"social-network-go/recommendation-service/service"
)

type RecommendationHandler struct {
	svc *service.RecommendationService
}

func NewRecommendationHandler(svc *service.RecommendationService) *RecommendationHandler {
	return &RecommendationHandler{svc: svc}
}

func (h *RecommendationHandler) GetSuggestedFriends(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("userId")
	}

	candidates, err := h.svc.GetSuggestedFriends(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Response{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Success: true,
		Data:    candidates,
	})
}

func (h *RecommendationHandler) GetSuggestedPosts(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("userId")
	}

	candidates, err := h.svc.GetSuggestedPosts(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.Response{
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Success: true,
		Data:    candidates,
	})
}
