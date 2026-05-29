package handler

import (
	"net/http"
	"time"
	"unicode"

	"social-network-go/auth-service/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthServiceInterface interface {
	Login(email, password string, isAdmin bool) (string, string, error)
	RefreshToken(tokenStr string) (string, error)
	Logout(refreshToken string) error
	Register(email, password, givenName, familyName, birthdate, clientIP string) (*model.VerifyCode, error)
	Verify(email string, code uuid.UUID) error
	ResendEmail(email, clientIP string) (*model.VerifyCode, error)
	ForgotPassword(email, clientIP string) error
	ResetPassword(code, newPassword string) error
	ChangePassword(userID uuid.UUID, oldPassword, newPassword string) error
	GetRefreshTokenDuration() time.Duration
}

type AuthHandler struct {
	AuthSvc AuthServiceInterface
}

func NewAuthHandler(authSvc AuthServiceInterface) *AuthHandler {
	return &AuthHandler{AuthSvc: authSvc}
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func sendSuccess(c *gin.Context, body interface{}) {
	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      body,
	})
}

type LoginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type RegisterReq struct {
	Email      string `json:"email" binding:"required,email"`
	Password   string `json:"password" binding:"required"`
	GivenName  string `json:"givenName" binding:"required"`
	FamilyName string `json:"familyName" binding:"required"`
	Birthdate  string `json:"birthdate" binding:"required"` // format: YYYY-MM-DD
}

type VerifyReq struct {
	Email string `json:"email" binding:"required,email"`
	Code  string `json:"code" binding:"required"`
}

type ForgotPasswordReq struct {
	Email string `json:"email" binding:"required,email"`
}

type ResetPasswordReq struct {
	Code        string `json:"code" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

type ChangePasswordReq struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required"`
}

type TokenResp struct {
	Token string `json:"token"`
}

func isValidPassword(s string) bool {
	if len(s) < 8 {
		return false
	}
	var hasUpper, hasNumber bool
	for _, c := range s {
		if unicode.IsUpper(c) {
			hasUpper = true
		}
		if unicode.IsDigit(c) {
			hasNumber = true
		}
	}
	return hasUpper && hasNumber
}

func isValidBirthdate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
