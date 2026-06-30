package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"net/http"
	"social-network-go/exception"
)

type TwoFactorCodeReq struct {
	Code string `json:"code" binding:"required"`
}

func (h *AuthHandler) Generate2FA(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	email := c.GetHeader("X-User-Email")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	secret, url, err := h.AuthSvc.Generate2FA(userID, email)
	if err != nil {
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnknownError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"body": gin.H{
			"secret": secret,
			"url":    url,
		},
	})
}

func (h *AuthHandler) Verify2FA(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req TwoFactorCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err = h.AuthSvc.Verify2FA(userID, req.Code)
	if err != nil {
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		c.JSON(400, gin.H{"code": "INVALID_2FA_CODE", "message": "Invalid 2FA code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "2FA enabled successfully",
	})
}

func (h *AuthHandler) Disable2FA(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req TwoFactorCodeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err = h.AuthSvc.Disable2FA(userID, req.Code)
	if err != nil {
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		c.JSON(400, gin.H{"code": "INVALID_2FA_CODE", "message": "Invalid 2FA code"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "2FA disabled successfully",
	})
}

func (h *AuthHandler) Get2FAStatus(c *gin.Context) {
	userIDStr := c.GetHeader("X-User-ID")
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	enabled, err := h.AuthSvc.Get2FAStatus(userID)
	if err != nil {
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnknownError)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"body": gin.H{
			"isTwoFactorEnabled": enabled,
		},
	})
}
