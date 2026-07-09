package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"social-network-go/admin-service/repository"
	"social-network-go/admin-service/service"
)

type RuntimeConfigResetRequest struct {
	Reason string `json:"reason"`
}

type RuntimeConfigSyncRequest struct {
	Reason string `json:"reason"`
}

func (h *AdminHandler) GetRuntimeConfigs(c *gin.Context) {
	items, err := h.svc.ListRuntimeConfigs(c.Request.Context(), repository.RuntimeConfigListFilter{
		Scope:    c.Query("scope"),
		Category: c.Query("category"),
		Query:    c.Query("q"),
	})
	if err != nil {
		writeRuntimeConfigError(c, err, "FAILED_TO_LIST_RUNTIME_CONFIGS")
		return
	}
	sendSuccess(c, gin.H{"items": items})
}

func (h *AdminHandler) CreateRuntimeConfig(c *gin.Context) {
	var req service.RuntimeConfigCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_RUNTIME_CONFIG_REQUEST", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	item, err := h.svc.CreateRuntimeConfig(c.Request.Context(), req, currentAdminID(c))
	if err != nil {
		writeRuntimeConfigError(c, err, "FAILED_TO_CREATE_RUNTIME_CONFIG")
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"code":      http.StatusCreated,
		"message":   "CREATED",
		"timestamp": time.Now().Format(time.RFC3339),
		"body":      item,
	})
}

func (h *AdminHandler) UpdateRuntimeConfig(c *gin.Context) {
	key := c.Param("key")
	var req service.RuntimeConfigUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "INVALID_RUNTIME_CONFIG_REQUEST", "timestamp": time.Now().Format(time.RFC3339), "error": err.Error()})
		return
	}
	item, err := h.svc.UpdateRuntimeConfig(c.Request.Context(), key, req, currentAdminID(c))
	if err != nil {
		writeRuntimeConfigError(c, err, "FAILED_TO_UPDATE_RUNTIME_CONFIG")
		return
	}
	sendSuccess(c, item)
}

func (h *AdminHandler) ResetRuntimeConfig(c *gin.Context) {
	key := c.Param("key")
	var req RuntimeConfigResetRequest
	_ = c.ShouldBindJSON(&req)
	item, err := h.svc.ResetRuntimeConfig(c.Request.Context(), key, currentAdminID(c), req.Reason)
	if err != nil {
		writeRuntimeConfigError(c, err, "FAILED_TO_RESET_RUNTIME_CONFIG")
		return
	}
	sendSuccess(c, item)
}

func (h *AdminHandler) SyncRuntimeConfigs(c *gin.Context) {
	var req RuntimeConfigSyncRequest
	_ = c.ShouldBindJSON(&req)
	result, err := h.svc.SyncRuntimeConfigCache(c.Request.Context(), currentAdminID(c), req.Reason)
	if err != nil {
		writeRuntimeConfigError(c, err, "FAILED_TO_SYNC_RUNTIME_CONFIGS")
		return
	}
	sendSuccess(c, result)
}

func writeRuntimeConfigError(c *gin.Context, err error, fallbackMessage string) {
	status := http.StatusInternalServerError
	message := fallbackMessage
	switch {
	case errors.Is(err, service.ErrInvalidRuntimeConfig):
		status = http.StatusBadRequest
		message = "INVALID_RUNTIME_CONFIG"
	case errors.Is(err, repository.ErrRuntimeConfigNotFound):
		status = http.StatusNotFound
		message = "RUNTIME_CONFIG_NOT_FOUND"
	case errors.Is(err, repository.ErrRuntimeConfigVersionConflict):
		status = http.StatusConflict
		message = "RUNTIME_CONFIG_VERSION_CONFLICT"
	case errors.Is(err, repository.ErrRuntimeConfigAlreadyExists):
		status = http.StatusConflict
		message = "RUNTIME_CONFIG_ALREADY_EXISTS"
	case errors.Is(err, repository.ErrRuntimeConfigNotEditable):
		status = http.StatusForbidden
		message = "RUNTIME_CONFIG_NOT_EDITABLE"
	case errors.Is(err, service.ErrRuntimeConfigUnavailable):
		status = http.StatusServiceUnavailable
		message = "RUNTIME_CONFIG_UNAVAILABLE"
	case errors.Is(err, service.ErrRuntimeConfigCacheUnavailable):
		status = http.StatusServiceUnavailable
		message = "RUNTIME_CONFIG_CACHE_UNAVAILABLE"
	case errors.Is(err, service.ErrRuntimeConfigSyncInProgress):
		status = http.StatusConflict
		message = "RUNTIME_CONFIG_SYNC_IN_PROGRESS"
	}
	c.JSON(status, gin.H{
		"code":      status,
		"message":   message,
		"timestamp": time.Now().Format(time.RFC3339),
		"error":     err.Error(),
	})
}
