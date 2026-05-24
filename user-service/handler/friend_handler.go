package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) GetFriends(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	currentUserID := getCurrentUser(c)
	friends, err := h.UserSvc.GetFriends(c.Request.Context(), username, currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetFriends failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetFriendsFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(friends))
}

func (h *UserHandler) GetSuggestedFriends(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	suggestions, err := h.UserSvc.GetSuggestedFriends(c.Request.Context(), currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetSuggestedFriends failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetFriendsFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(suggestions))
}

func (h *UserHandler) GetMutualFriends(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	mutual, err := h.UserSvc.GetMutualFriends(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetMutualFriends failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetFriendsFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(mutual))
}

func (h *UserHandler) Unfriend(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Unfriend(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Unfriend failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnfriendFailed)
		return
	}

	sendSuccess(c, nil)
}
