package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if err := h.AuthSvc.ForgotPassword(req.Email, c.ClientIP()); err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("ForgotPassword failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.ForgotPasswordFailed)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_RESET_EMAIL_SENT"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if !isValidPassword(req.NewPassword) {
		exception.SendError(c, exception.InvalidPassword)
		return
	}

	if err := h.AuthSvc.ResetPassword(req.Code, req.NewPassword); err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("ResetPassword failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.ResetPasswordFailed)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_RESET_SUCCESSFULLY"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	// Extract user ID from header (passed by API Gateway Auth Middleware)
	userIDStr := c.GetHeader("X-User-ID")
	if userIDStr == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	accountID, err := uuid.Parse(userIDStr)
	if err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	var req ChangePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if !isValidPassword(req.NewPassword) {
		exception.SendError(c, exception.InvalidPassword)
		return
	}

	if err := h.AuthSvc.ChangePassword(accountID, req.OldPassword, req.NewPassword); err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("ChangePassword failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.ChangePasswordFailed)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_CHANGED_SUCCESSFULLY"})
}
