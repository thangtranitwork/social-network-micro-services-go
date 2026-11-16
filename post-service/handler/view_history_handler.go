package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/post-service/dto"
)

// GetViewHistory handles GET /v1/posts/view-history
func (h *PostHandler) GetViewHistory(c *gin.Context) {
	ctx := c.Request.Context()

	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthenticated)
		return
	}

	if h.PostSvc.HistorySvc == nil {
		sendSuccess(c, dto.ViewHistoryResponse{
			Items:   []*dto.ViewHistoryItem{},
			HasMore: false,
		})
		return
	}

	cursor := c.Query("cursor")
	limitStr := c.DefaultQuery("limit", "20")
	var limit int
	fmt.Sscanf(limitStr, "%d", &limit)

	resp, err := h.PostSvc.HistorySvc.GetViewHistory(ctx, currentUserID, cursor, limit)
	if err != nil {
		logger.WithContext(ctx).Err(err).Warn("failed to fetch view history for user %s: %v", currentUserID, err)
		exception.SendError(c, exception.InvalidInput)
		return
	}

	sendSuccess(c, resp)
}
