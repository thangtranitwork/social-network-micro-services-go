package handler

import (
	"net/http"
	"strings"
	"time"
	"unicode"
	"social-network-go/logger"
	"social-network-go/auth-service/service"
	"social-network-go/exception"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AuthHandler struct {
	AuthSvc *service.AuthService
}

func NewAuthHandler(authSvc *service.AuthService) *AuthHandler {
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

func sendError(c *gin.Context, httpStatus int, code int, msg string) {
	// Look up mapped ErrorCode in exception package
	var mapped exception.ErrorCode
	var found bool

	// Match by msg
	switch msg {
	case "ACCOUNT_NOT_FOUND":
		mapped = exception.AccountNotFound
		found = true
	case "ACCOUNT_NOT_VERIFIED":
		mapped = exception.AccountNotVerified
		found = true
	case "ACCOUNT_LOCKED":
		mapped = exception.AccountLocked
		found = true
	case "AUTHENTICATION_FAILED":
		mapped = exception.AuthenticationFailed
		found = true
	case "INVALID_PASSWORD", "WEAK_PASSWORD_MUST_BE_8_CHARS_UPPERCASE_NUMBER", "PASSWORD_NOT_STRONG_ENOUGH":
		mapped = exception.InvalidPassword
		found = true
	case "INVALID_EMAIL":
		mapped = exception.InvalidEmail
		found = true
	case "EMAIL_REQUIRED":
		mapped = exception.EmailRequired
		found = true
	case "PASSWORD_REQUIRED":
		mapped = exception.PasswordRequired
		found = true
	case "VERIFICATION_CODE_NOT_FOUND":
		mapped = exception.VerificationCodeNotFound
		found = true
	case "VERIFICATION_CODE_NOT_MATCHED_OR_EXPIRED":
		mapped = exception.VerificationCodeNotMatchedOrExpired
		found = true
	case "REFRESH_TOKEN_REQUIRED":
		mapped = exception.RefreshTokenRequired
		found = true
	case "INVALID_OR_EXPIRED_REFRESH_TOKEN":
		mapped = exception.InvalidOrExpiredRefreshToken
		found = true
	case "ACCOUNT_ALREADY_EXISTS":
		mapped = exception.AccountAlreadyExists
		found = true
	case "INVALID_TOKEN":
		mapped = exception.InvalidToken
		found = true
	case "EXPIRED_TOKEN":
		mapped = exception.ExpiredToken
		found = true
	case "VERIFICATION_CODE_REQUIRED":
		mapped = exception.VerificationCodeRequired
		found = true
	case "ACCOUNT_VERIFIED", "ACCOUNT_ALREADY_VERIFIED":
		mapped = exception.AccountVerified
		found = true
	case "EMAIL_NOT_VERIFIED":
		mapped = exception.EmailNotVerified
		found = true
	case "UNAUTHORIZED":
		mapped = exception.Unauthorized
		found = true
	case "INVALID_REQUEST_PAYLOAD", "INVALID_BIRTHDATE_FORMAT_MUST_BE_YYYY_MM_DD", "INVALID_CODE_FORMAT":
		mapped = exception.InvalidInput
		found = true
	}

	// Support auth LOCK / ATTEMPT prefixed strings
	if !found {
		if strings.HasPrefix(msg, "AUTHENTICATION_FAILED") {
			mapped = exception.AuthenticationFailed
			found = true
		} else if strings.HasPrefix(msg, "ACCOUNT_LOCKED") {
			mapped = exception.AccountLocked
			found = true
		}
	}

	if found {
		httpStatus = mapped.Status
		code = mapped.Code
		msg = mapped.Message
	}

	c.JSON(httpStatus, ApiResponse{
		Code:      code,
		Message:   msg,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func sendAppError(c *gin.Context, err error) {
	if appErr, ok := err.(exception.AppException); ok {
		c.JSON(appErr.ErrCode.Status, ApiResponse{
			Code:      appErr.ErrCode.Code,
			Message:   appErr.ErrCode.Message,
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}
	// Fallback to sendError mapping
	sendError(c, http.StatusBadRequest, 400, err.Error())
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

func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, false)
	if err != nil {
		sendAppError(c, err)
		return
	}

	// Set HTTP-Only Cookie with Refresh Token
	c.SetCookie("token", refreshToken, int(h.AuthSvc.Cfg.RefreshTokenDuration.Seconds()), "/", "", false, true)

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) LoginAdmin(c *gin.Context) {
	var req LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	accessToken, refreshToken, err := h.AuthSvc.Login(req.Email, req.Password, true)
	if err != nil {
		sendAppError(c, err)
		return
	}

	// Set HTTP-Only Cookie with Refresh Token
	c.SetCookie("token", refreshToken, int(h.AuthSvc.Cfg.RefreshTokenDuration.Seconds()), "/", "", false, true)

	sendSuccess(c, TokenResp{Token: accessToken})
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	if !isValidPassword(req.Password) {
		sendError(c, http.StatusBadRequest, 400, "WEAK_PASSWORD_MUST_BE_8_CHARS_UPPERCASE_NUMBER")
		return
	}

	if !isValidBirthdate(req.Birthdate) {
		sendError(c, http.StatusBadRequest, 400, "INVALID_BIRTHDATE_FORMAT_MUST_BE_YYYY_MM_DD")
		return
	}

	// Perform registration
	verifyCode, err := h.AuthSvc.Register(req.Email, req.Password, req.GivenName, req.FamilyName, req.Birthdate)
	if err != nil {
		sendAppError(c, err)
		return
	}

	// Return Success (Verify code returned for convenience in development, in production it would only go via email)
	sendSuccess(c, gin.H{
		"message":      "REGISTRATION_SUCCESSFUL_PLEASE_VERIFY",
		"verify_code":  verifyCode.Code, // Optional but useful for debugging
		"email":        req.Email,
	})
}

func (h *AuthHandler) Verify(c *gin.Context) {
	var req VerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		logger.Err(err).Error("Invalid request")
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	codeUUID, err := uuid.Parse(req.Code)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_CODE_FORMAT")
		return
	}

	if err := h.AuthSvc.Verify(req.Email, codeUUID); err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, gin.H{"message": "ACCOUNT_VERIFIED_SUCCESSFULLY"})
}

func (h *AuthHandler) ResendEmail(c *gin.Context) {
	email := c.Query("email")
	if email == "" {
		sendError(c, http.StatusBadRequest, 400, "EMAIL_REQUIRED")
		return
	}

	_, err := h.AuthSvc.ResendEmail(email)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, gin.H{"message": "VERIFICATION_EMAIL_SENT"})
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	// Retrieve from Cookie
	cookieToken, err := c.Cookie("token")
	if err != nil || cookieToken == "" {
		sendError(c, http.StatusUnauthorized, 401, "REFRESH_TOKEN_REQUIRED")
		return
	}

	newAccessToken, err := h.AuthSvc.RefreshToken(cookieToken)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, TokenResp{Token: newAccessToken})
}

func (h *AuthHandler) Logout(c *gin.Context) {
	cookieToken, err := c.Cookie("token")
	if err == nil && cookieToken != "" {
		_ = h.AuthSvc.Logout(cookieToken)
	}

	// Delete cookie
	c.SetCookie("token", "", -1, "/", "", false, true)

	sendSuccess(c, gin.H{"message": "LOGGED_OUT_SUCCESSFULLY"})
}

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req ForgotPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	if err := h.AuthSvc.ForgotPassword(req.Email); err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_RESET_EMAIL_SENT"})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	if !isValidPassword(req.NewPassword) {
		sendError(c, http.StatusBadRequest, 400, "PASSWORD_NOT_STRONG_ENOUGH")
		return
	}

	if err := h.AuthSvc.ResetPassword(req.Code, req.NewPassword); err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_RESET_SUCCESSFULLY"})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	// Extract user ID from header (passed by API Gateway Auth Middleware)
	userIDStr := c.GetHeader("X-User-ID")
	if userIDStr == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}
	
	accountID, err := uuid.Parse(userIDStr)
	if err != nil {
		sendError(c, http.StatusUnauthorized, 401, "INVALID_USER_ID")
		return
	}

	var req ChangePasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	if !isValidPassword(req.NewPassword) {
		sendError(c, http.StatusBadRequest, 400, "PASSWORD_NOT_STRONG_ENOUGH")
		return
	}

	if err := h.AuthSvc.ChangePassword(accountID, req.OldPassword, req.NewPassword); err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, gin.H{"message": "PASSWORD_CHANGED_SUCCESSFULLY"})
}
