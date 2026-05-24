package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) SendFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.SendFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SendFriendRequest failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SendFriendRequestFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) AcceptFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.AcceptFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("AcceptFriendRequest failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.AcceptRequestFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) DeleteFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.DeleteFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("DeleteFriendRequest failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.DeleteRequestFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) GetSentRequests(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	requests, err := h.UserSvc.GetSentRequests(c.Request.Context(), currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetSentRequests failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetRequestsFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(requests))
}

func (h *UserHandler) GetReceivedRequests(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	requests, err := h.UserSvc.GetReceivedRequests(c.Request.Context(), currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetReceivedRequests failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetRequestsFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(requests))
}
