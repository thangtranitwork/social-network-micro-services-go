package handler

import (
	"errors"
	"net/http"
	"time"

	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/user-service/model"
	"social-network-go/user-service/repository"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) GetUserSpace(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	currentUserID := getCurrentUser(c)
	spaceCfg, err := h.UserSvc.GetUserSpace(c.Request.Context(), username, currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetUserSpace failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetUserProfileFailed)
		return
	}

	sendSuccess(c, spaceCfg)
}

func (h *UserHandler) UpdateUserSpace(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req model.UpdateSpaceConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   "Invalid space configuration payload: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	updatedCfg, err := h.UserSvc.UpdateUserSpace(c.Request.Context(), currentUserID, req)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Warn("UpdateUserSpace validation or save failed")
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	sendSuccess(c, updatedCfg)
}

func (h *UserHandler) AddSpaceGuestbookEntry(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	targetUsername := c.Param("username")
	if targetUsername == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	var req model.CreateSpaceGuestbookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   "Invalid guestbook payload: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	entry, err := h.UserSvc.AddSpaceGuestbookEntry(c.Request.Context(), targetUsername, currentUserID, req)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Warn("AddSpaceGuestbookEntry failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		if errors.Is(err, repository.ErrGuestbookDisabled) {
			c.JSON(http.StatusForbidden, ApiResponse{
				Code:      http.StatusForbidden,
				Message:   err.Error(),
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      http.StatusBadRequest,
			Message:   err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	sendSuccess(c, entry)
}

func (h *UserHandler) DeleteSpaceGuestbookEntry(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	entryID := c.Param("entryId")
	if entryID == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.UserSvc.DeleteSpaceGuestbookEntry(c.Request.Context(), entryID, currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Warn("DeleteSpaceGuestbookEntry failed")
		if errors.Is(err, repository.ErrGuestbookEntryNotFound) {
			c.JSON(http.StatusNotFound, ApiResponse{
				Code:      http.StatusNotFound,
				Message:   err.Error(),
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}
		if errors.Is(err, repository.ErrGuestbookForbidden) {
			c.JSON(http.StatusForbidden, ApiResponse{
				Code:      http.StatusForbidden,
				Message:   err.Error(),
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}
		exception.SendError(c, exception.InvalidInput)
		return
	}

	sendSuccess(c, nil)
}
