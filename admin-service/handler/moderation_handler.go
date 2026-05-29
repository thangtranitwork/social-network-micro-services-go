package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type SuspendRequest struct {
	DurationSeconds int64 `json:"duration_seconds"`
}

func (h *AdminHandler) DeletePost(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_POST_ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	err := h.svc.DeletePost(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_DELETE_POST",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{"status": "deleted"})
}

func (h *AdminHandler) SuspendUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_USER_ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	var req SuspendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// fallback to 24h default if body is not supplied/valid
		req.DurationSeconds = 86400
	}
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 86400
	}

	duration := time.Duration(req.DurationSeconds) * time.Second

	err := h.svc.SuspendUser(c.Request.Context(), id, duration)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_SUSPEND_USER",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{
		"status":          "suspended",
		"duration_second": req.DurationSeconds,
		"suspended_until": time.Now().Add(duration).Format(time.RFC3339),
	})
}

func (h *AdminHandler) UnsuspendUser(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_USER_ID",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	err := h.svc.UnsuspendUser(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_UNSUSPEND_USER",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{
		"status": "active",
	})
}
