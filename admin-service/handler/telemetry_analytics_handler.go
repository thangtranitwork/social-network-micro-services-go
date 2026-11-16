package handler

import "github.com/gin-gonic/gin"

func (h *AdminHandler) GetTelemetryAnalytics(c *gin.Context) {
	data := h.svc.GetTelemetryAndSearchAnalytics(c.Request.Context())
	sendSuccess(c, data)
}
