package handler

import (
	"errors"
	"net/http"
	"strconv"

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
		Code:    200,
		Success: true,
		Data:    candidates,
		Body:    candidates,
		Message: "OK",
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
		Code:    200,
		Success: true,
		Data:    candidates,
		Body:    candidates,
		Message: "OK",
	})
}

func (h *RecommendationHandler) GetNearbyUsers(c *gin.Context) {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("userId")
	}

	lat, errLat := strconv.ParseFloat(c.Query("lat"), 64)
	lng, errLng := strconv.ParseFloat(c.Query("lng"), 64)
	if errLat != nil || errLng != nil || lat < -90 || lat > 90 || lng < -180 || lng > 180 {
		c.JSON(http.StatusBadRequest, model.Response{
			Code:    400,
			Success: false,
			Message: "INVALID_COORDINATES",
		})
		return
	}

	radiusKm, _ := strconv.ParseFloat(c.DefaultQuery("radiusKm", "5"), 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	candidates, err := h.svc.GetNearbyUsers(c.Request.Context(), userID, lat, lng, radiusKm, limit)
	if err != nil {
		if errors.Is(err, service.ErrMutualOptInRequired) {
			c.JSON(http.StatusForbidden, model.Response{
				Code:    403,
				Success: false,
				Message: err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.Response{
			Code:    500,
			Success: false,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, model.Response{
		Code:    200,
		Success: true,
		Data:    candidates,
		Body:    candidates,
		Message: "OK",
	})
}
