package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/admin-service/service"
)

type SuspendRequest struct {
	DurationSeconds int64  `json:"duration_seconds"`
	Reason          string `json:"reason"`
}

type ModerationActionRequest struct {
	Reason string `json:"reason"`
}

func currentAdminID(c *gin.Context) string {
	if id := c.GetHeader("X-User-ID"); id != "" {
		return id
	}
	return "admin"
}

func (h *AdminHandler) GetModerationQueue(c *gin.Context) {
	items := h.svc.ListModerationQueue(c.Request.Context(), service.ModerationQueueFilter{
		Status:   c.Query("status"),
		Category: c.Query("category"),
	})
	sendSuccess(c, items)
}

func (h *AdminHandler) ApproveModerationItem(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_MODERATION_ID", "timestamp": time.Now().Format(time.RFC3339)})
		return
	}
	var req ModerationActionRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.ApproveModerationItem(c.Request.Context(), id, currentAdminID(c), req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "FAILED_TO_APPROVE_MODERATION_ITEM", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	sendSuccess(c, gin.H{"status": "approved"})
}

func (h *AdminHandler) HideModerationItem(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_MODERATION_ID", "timestamp": time.Now().Format(time.RFC3339)})
		return
	}
	var req ModerationActionRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.HideModerationItem(c.Request.Context(), id, currentAdminID(c), req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "FAILED_TO_HIDE_MODERATION_ITEM", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	sendSuccess(c, gin.H{"status": "hidden"})
}

func (h *AdminHandler) DeleteModerationItem(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_MODERATION_ID", "timestamp": time.Now().Format(time.RFC3339)})
		return
	}
	var req ModerationActionRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.DeleteModerationItem(c.Request.Context(), id, currentAdminID(c), req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "FAILED_TO_DELETE_MODERATION_ITEM", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	sendSuccess(c, gin.H{"status": "deleted"})
}

func (h *AdminHandler) SuspendModerationAuthor(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_MODERATION_ID", "timestamp": time.Now().Format(time.RFC3339)})
		return
	}

	var req SuspendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.DurationSeconds = 86400
	}
	if req.DurationSeconds <= 0 {
		req.DurationSeconds = 86400
	}
	duration := time.Duration(req.DurationSeconds) * time.Second
	if err := h.svc.SuspendModerationAuthor(c.Request.Context(), id, currentAdminID(c), req.Reason, duration); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "FAILED_TO_SUSPEND_MODERATION_AUTHOR", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	sendSuccess(c, gin.H{"status": "author_suspended", "duration_second": req.DurationSeconds})
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
