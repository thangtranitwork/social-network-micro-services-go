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

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, req.TwoFactorCode, false)
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
	setRefreshCookie(c, "token", refreshToken, int(h.AuthSvc.GetRefreshTokenDuration().Seconds()))

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) LoginAdmin(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, req.TwoFactorCode, true)
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
	setRefreshCookie(c, "admin_token", refreshToken, int(h.AuthSvc.GetRefreshTokenDuration().Seconds()))

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	// Retrieve from Cookie: only check token
	cookieToken, err := c.Cookie("token")
	if err != nil || cookieToken == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	newAccessToken, err := h.AuthSvc.RefreshTokenForRole(cookieToken, "USER")
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

func (h *AuthHandler) RefreshAdmin(c *gin.Context) {
	// Retrieve from Cookie: only check admin_token
	cookieToken, err := c.Cookie("admin_token")
	if err != nil || cookieToken == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	newAccessToken, err := h.AuthSvc.RefreshTokenForRole(cookieToken, "ADMIN")
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("RefreshAdmin failed")
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
	h.logoutCookie(c, "admin_token")
	h.logoutCookie(c, "token")

	sendSuccess(c, gin.H{"message": "LOGGED_OUT_SUCCESSFULLY"})
}

func (h *AuthHandler) LogoutUser(c *gin.Context) {
	h.logoutCookie(c, "token")

	sendSuccess(c, gin.H{"message": "USER_LOGGED_OUT_SUCCESSFULLY"})
}

func (h *AuthHandler) LogoutAdmin(c *gin.Context) {
	h.logoutCookie(c, "admin_token")

	sendSuccess(c, gin.H{"message": "ADMIN_LOGGED_OUT_SUCCESSFULLY"})
}

func (h *AuthHandler) logoutCookie(c *gin.Context, name string) {
	if cookieToken, err := c.Cookie(name); err == nil && cookieToken != "" {
		_ = h.AuthSvc.Logout(cookieToken)
	}
	setRefreshCookie(c, name, "", -1)
}
