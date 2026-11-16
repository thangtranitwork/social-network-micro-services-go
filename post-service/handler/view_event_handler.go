package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/post-service/dto"
	"social-network-go/post-service/service"
)

// RecordViewEvents handles POST /v1/posts/view-events
func (h *PostHandler) RecordViewEvents(c *gin.Context) {
	ctx := c.Request.Context()

	// 1. Check payload size against hard cap (64 KiB)
	if c.Request.ContentLength > dto.MaxPayloadBytes {
		logger.WithContext(ctx).Warn("payload size %d exceeds limit %d", c.Request.ContentLength, dto.MaxPayloadBytes)
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   "PAYLOAD_TOO_LARGE: maximum payload size is 64 KiB",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// 2. Extract authenticated user ID
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthenticated)
		return
	}

	// 3. Parse JSON batch
	var req dto.ViewEventsBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.WithContext(ctx).Err(err).Warn("failed to parse view events request body")
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   "INVALID_REQUEST_BODY: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// 4. Process and publish to Kafka with broker ACK
	resp, err := h.PostSvc.ProcessViewEvents(ctx, currentUserID, &req)
	if err != nil {
		if errors.Is(err, service.ErrUnauthorized) {
			exception.SendError(c, exception.Unauthenticated)
			return
		}
		if errors.Is(err, service.ErrBadRequest) {
			c.JSON(http.StatusBadRequest, ApiResponse{
				Code:      http.StatusBadRequest,
				Message:   err.Error(),
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}
		if errors.Is(err, service.ErrServiceUnavailable) {
			c.JSON(http.StatusServiceUnavailable, ApiResponse{
				Code:      http.StatusServiceUnavailable,
				Message:   "SERVICE_UNAVAILABLE: failed to deliver view events to broker",
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}

		logger.WithContext(ctx).Err(err).Error("unexpected error processing view events")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      http.StatusInternalServerError,
			Message:   "INTERNAL_SERVER_ERROR",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// 5. Return HTTP 202 Accepted only after broker confirmation
	c.JSON(http.StatusAccepted, ApiResponse{
		Code:      http.StatusAccepted,
		Message:   "Accepted",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      resp,
	})
}
