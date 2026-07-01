package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"social-network-go/exception"
)

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	url := h.AuthSvc.GetGoogleAuthURL()
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	_, refreshToken, _, _, err := h.AuthSvc.GoogleCallback(c.Request.Context(), code)
	if err != nil {
		exception.SendError(c, exception.UnknownError)
		return
	}

	setRefreshCookie(c, "token", refreshToken, int(h.AuthSvc.GetRefreshTokenDuration().Seconds()))

	frontendURL := h.AuthSvc.GetFrontendURL()
	redirectURL := fmt.Sprintf("%s/register?oauth=success", frontendURL)
	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}
