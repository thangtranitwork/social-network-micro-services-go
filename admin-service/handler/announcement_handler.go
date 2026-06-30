package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AnnouncementRequest struct {
	Text   string `json:"text"`
	Active bool   `json:"active"`
}

func (h *AdminHandler) GetAnnouncement(c *gin.Context) {
	announcement, err := h.svc.GetAnnouncement(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_GET_ANNOUNCEMENT",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, announcement)
}

func (h *AdminHandler) SetAnnouncement(c *gin.Context) {
	var req AnnouncementRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "INVALID_REQUEST_BODY",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	err := h.svc.SetAnnouncement(c.Request.Context(), req.Text, req.Active)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_SET_ANNOUNCEMENT",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{"status": "saved"})
}

func (h *AdminHandler) DeleteAnnouncement(c *gin.Context) {
	err := h.svc.DeleteAnnouncement(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "FAILED_TO_DELETE_ANNOUNCEMENT",
			"timestamp": time.Now().Format(time.RFC3339),
			"error":     err.Error(),
		})
		return
	}

	sendSuccess(c, gin.H{"status": "deleted"})
}
