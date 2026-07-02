package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/post-service/service"
)

type ReportHandler struct {
	reportSvc *service.ReportService
}

type ReportRequest struct {
	TargetType string `json:"targetType" binding:"required"`
	TargetID   string `json:"targetId" binding:"required"`
	Reason     string `json:"reason" binding:"required"`
}

func NewReportHandler(reportSvc *service.ReportService) *ReportHandler {
	return &ReportHandler{reportSvc: reportSvc}
}

func (h *ReportHandler) SubmitReport(c *gin.Context) {
	reporterID := getCurrentUser(c)
	if reporterID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req ReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	count, thresholdReached, err := h.reportSvc.SubmitReport(c.Request.Context(), req.TargetType, req.TargetID, reporterID, req.Reason)
	if err != nil {
		if err.Error() == "DUPLICATE_REPORT" {
			c.JSON(http.StatusConflict, gin.H{
				"code":      http.StatusConflict,
				"message":   "DUPLICATE_REPORT",
				"timestamp": time.Now().Format(time.RFC3339),
			})
			return
		}
		logger.WithContext(c.Request.Context()).Err(err).Error("SubmitReport failed")
		exception.SendError(c, exception.InvalidInput)
		return
	}

	sendSuccess(c, gin.H{
		"status":           "reported",
		"reportCount":      count,
		"thresholdReached": thresholdReached,
	})
}
