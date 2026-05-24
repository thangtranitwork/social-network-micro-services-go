package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) Block(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Block(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Block failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.BlockFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) Unblock(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Unblock(c.Request.Context(), currentUserID, username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Unblock failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnblockFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) GetBlockedUsers(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	blocked, err := h.UserSvc.GetBlockedUsers(c.Request.Context(), currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetBlockedUsers failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetBlockedUsersFailed)
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(blocked))
}
