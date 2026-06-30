package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"
	"social-network-go/user-service/model"

	"github.com/gin-gonic/gin"
)

func (h *UserHandler) GetUserProfile(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	currentUserID := getCurrentUser(c)
	email := c.GetHeader("X-User-Email")
	if currentUserID != "" && email != "" {
		_, _ = h.UserSvc.EnsureProfile(c.Request.Context(), currentUserID, email, "", "", "")
	}

	profile, err := h.UserSvc.GetUserProfile(c.Request.Context(), username, currentUserID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetUserProfile failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetUserProfileFailed)
		return
	}

	sendSuccess(c, mapUserToProfileResponse(profile))
}

func (h *UserHandler) UpdateBio(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req model.UpdateBioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.UserSvc.UpdateBio(c.Request.Context(), currentUserID, req.Bio)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateBio failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdateBioFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateBirthdate(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req model.UpdateBirthdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.BirthdateRequired)
		return
	}

	err := h.UserSvc.UpdateBirthdate(c.Request.Context(), currentUserID, req.Birthdate)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateBirthdate failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdateBirthdateFailed)
		return
	}

	sendSuccess(c, req.Birthdate)
}

func (h *UserHandler) UpdateName(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req model.UpdateNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.UserSvc.UpdateName(c.Request.Context(), currentUserID, req.FamilyName, req.GivenName)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateName failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdateNameFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateUsername(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req model.UpdateUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	err := h.UserSvc.UpdateUsername(c.Request.Context(), currentUserID, req.Username)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateUsername failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdateUsernameFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateProfilePicture(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req struct {
		FileId string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.FileRequired)
		return
	}

	path, err := h.UserSvc.UpdateProfilePicture(c.Request.Context(), currentUserID, req.FileId)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateProfilePicture failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdateProfilePictureFailed)
		return
	}

	sendSuccess(c, path)
}

func (h *UserHandler) UpdateNotificationPreferences(c *gin.Context) {
	currentUserID := c.GetString("user_id")
	var req model.UpdateNotificationPreferencesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"code": "INVALID_REQUEST", "message": "Invalid request format or missing required fields"})
		return
	}

	err := h.UserSvc.UpdateNotificationPreferences(c.Request.Context(), currentUserID, req)
	if err != nil {
		c.JSON(500, gin.H{"code": "INTERNAL_SERVER_ERROR", "message": err.Error()})
		return
	}

	c.JSON(200, gin.H{"message": "Notification preferences updated successfully"})
}
