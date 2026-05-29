package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if !isValidPassword(req.Password) {
		exception.SendError(c, exception.InvalidPassword)
		return
	}

	if !isValidBirthdate(req.Birthdate) {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	// Perform registration
	verifyCode, err := h.AuthSvc.Register(req.Email, req.Password, req.GivenName, req.FamilyName, req.Birthdate, c.ClientIP())
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Register failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.RegisterFailed)
		return
	}

	// Return Success (Verify code returned for convenience in development)
	sendSuccess(c, gin.H{
		"message":     "REGISTRATION_SUCCESSFUL_PLEASE_VERIFY",
		"verify_code": verifyCode.Code,
		"email":       req.Email,
	})
}

func (h *AuthHandler) Verify(c *gin.Context) {
	var req VerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Error("Invalid request")
		exception.SendError(c, exception.InvalidInput)
		return
	}

	codeUUID, err := uuid.Parse(req.Code)
	if err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	if err := h.AuthSvc.Verify(req.Email, codeUUID); err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Verify failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.VerifyFailed)
		return
	}

	sendSuccess(c, gin.H{"message": "ACCOUNT_VERIFIED_SUCCESSFULLY"})
}

func (h *AuthHandler) ResendEmail(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	_, err := h.AuthSvc.ResendEmail(email, c.ClientIP())
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("ResendEmail failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.ResendEmailFailed)
		return
	}

	sendSuccess(c, gin.H{"message": "VERIFICATION_EMAIL_SENT"})
}
