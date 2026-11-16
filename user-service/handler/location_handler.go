package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"social-network-go/user-service/model"
)

func (h *UserHandler) GetLocationSettings(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED"})
		return
	}
	settings, err := h.UserSvc.GetLocationSettings(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error()})
		return
	}
	sendSuccess(c, settings)
}

func (h *UserHandler) UpdateLocationSettings(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED"})
		return
	}
	var req model.LocationSettingsUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{Code: 400, Message: "INVALID_REQUEST"})
		return
	}
	settings, err := h.UserSvc.UpdateLocationSettings(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error()})
		return
	}
	sendSuccess(c, settings)
}

func (h *UserHandler) UpdateLocation(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED"})
		return
	}
	var req model.LocationUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{Code: 400, Message: "INVALID_LOCATION_PAYLOAD"})
		return
	}
	meta := model.ClientTelemetryMeta{
		IPAddress:   c.GetHeader("X-Client-IP"),
		DeviceID:    c.GetHeader("X-Device-Id"),
		Platform:    c.GetHeader("X-Platform"),
		DeviceModel: c.GetHeader("X-Device-Model"),
		OSVersion:   c.GetHeader("X-OS-Version"),
		AppVersion:  c.GetHeader("X-App-Version"),
		UserAgent:   c.GetHeader("User-Agent"),
	}
	if meta.IPAddress == "" {
		meta.IPAddress = c.ClientIP()
	}

	res, err := h.UserSvc.UpdateUserLocation(c.Request.Context(), userID, req, meta)
	if err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{Code: 400, Message: err.Error()})
		return
	}
	sendSuccess(c, res)
}

func (h *UserHandler) ClearLocation(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED"})
		return
	}
	if err := h.UserSvc.ClearAllUserLocation(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error()})
		return
	}
	sendSuccess(c, gin.H{"cleared": true})
}

func (h *UserHandler) GetDeviceSessions(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		c.JSON(http.StatusUnauthorized, ApiResponse{Code: 401, Message: "UNAUTHORIZED"})
		return
	}
	sessions, err := h.UserSvc.GetDeviceSessions(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{Code: 500, Message: err.Error()})
		return
	}
	sendSuccess(c, sessions)
}
