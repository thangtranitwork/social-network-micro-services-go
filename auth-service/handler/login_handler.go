package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, false)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Login failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.LoginFailed)
		return
	}

	// Set HTTP-Only Cookie with Refresh Token
	c.SetCookie("token", refreshToken, int(h.AuthSvc.GetRefreshTokenDuration().Seconds()), "/", "", false, true)

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) LoginAdmin(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, true)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("LoginAdmin failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.LoginFailed)
		return
	}

	// Set HTTP-Only Cookie with Admin Refresh Token
	c.SetCookie("admin_token", refreshToken, int(h.AuthSvc.GetRefreshTokenDuration().Seconds()), "/", "", false, true)

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	// Retrieve from Cookie: try admin_token first, then token
	cookieToken, err := c.Cookie("admin_token")
	if err != nil || cookieToken == "" {
		cookieToken, err = c.Cookie("token")
	}
	if err != nil || cookieToken == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	newAccessToken, err := h.AuthSvc.RefreshToken(cookieToken)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Refresh failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.RefreshFailed)
		return
	}

	sendSuccess(c, TokenResp{Token: newAccessToken})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	if cookieToken, err := c.Cookie("admin_token"); err == nil && cookieToken != "" {
		_ = h.AuthSvc.Logout(cookieToken)
	}
	if cookieToken, err := c.Cookie("token"); err == nil && cookieToken != "" {
		_ = h.AuthSvc.Logout(cookieToken)
	}

	// Delete cookies
	c.SetCookie("admin_token", "", -1, "/", "", false, true)
	c.SetCookie("token", "", -1, "/", "", false, true)

	sendSuccess(c, gin.H{"message": "LOGGED_OUT_SUCCESSFULLY"})
}
