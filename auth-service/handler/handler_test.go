package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"social-network-go/auth-service/model"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type MockAuthService struct {
	MockAccessToken  string
	MockRefreshToken string
	VerifyCode       *model.VerifyCode
	Err              error
}

func (m *MockAuthService) Login(email, password, twoFactorCode string, isAdmin bool) (string, string, error) {
	return m.MockAccessToken, m.MockRefreshToken, m.Err
}

func (m *MockAuthService) RefreshToken(tokenStr string) (string, error) {
	return m.MockAccessToken, m.Err
}

func (m *MockAuthService) Logout(refreshToken string) error {
	return m.Err
}

func (m *MockAuthService) Register(email, password, givenName, familyName, birthdate, clientIP string) (*model.VerifyCode, error) {
	return m.VerifyCode, m.Err
}

func (m *MockAuthService) Verify(email string, code uuid.UUID) error {
	return m.Err
}

func (m *MockAuthService) ResendEmail(email, clientIP string) (*model.VerifyCode, error) {
	return m.VerifyCode, m.Err
}

func (m *MockAuthService) ForgotPassword(email, clientIP string) error {
	return m.Err
}

func (m *MockAuthService) ResetPassword(code, newPassword string) error {
	return m.Err
}

func (m *MockAuthService) ChangePassword(userID uuid.UUID, oldPassword, newPassword string) error {
	return m.Err
}

func (m *MockAuthService) GetRefreshTokenDuration() time.Duration {
	return 24 * time.Hour
}

func (m *MockAuthService) GetGoogleAuthURL() string {
	return ""
}

func (m *MockAuthService) GetFrontendURL() string {
	return ""
}

func (m *MockAuthService) GoogleCallback(ctx context.Context, code string) (string, string, string, string, error) {
	return "", "", "", "", nil
}

func (m *MockAuthService) Generate2FA(userID uuid.UUID, email string) (string, string, error) {
	return "", "", nil
}

func (m *MockAuthService) Verify2FA(userID uuid.UUID, code string) error {
	return nil
}

func (m *MockAuthService) Disable2FA(userID uuid.UUID, code string) error {
	return nil
}

func (m *MockAuthService) Get2FAStatus(userID uuid.UUID) (bool, error) {
	return false, nil
}

func setupTestRouter(svc *MockAuthService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAuthHandler(svc)

	authRoutes := r.Group("/v1/auth")
	{
		authRoutes.POST("/login", h.Login)
		authRoutes.POST("/login-admin", h.LoginAdmin)
		authRoutes.POST("/refresh", h.Refresh)
		authRoutes.DELETE("/logout", h.Logout)
		authRoutes.POST("/forgot-password", h.ForgotPassword)
		authRoutes.POST("/reset-password", h.ResetPassword)
		authRoutes.POST("/change-password", h.ChangePassword)
	}

	registerRoutes := r.Group("/v1/register")
	{
		registerRoutes.POST("", h.Register)
		registerRoutes.POST("/resend-email", h.ResendEmail)
		registerRoutes.PATCH("/verify", h.Verify)
	}

	return r
}

func TestLogin(t *testing.T) {
	mockSvc := &MockAuthService{
		MockAccessToken:  "access-token-123",
		MockRefreshToken: "refresh-token-123",
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := LoginReq{
		Email:    "test@example.com",
		Password: "Password123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/auth/login", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestLoginAdmin(t *testing.T) {
	mockSvc := &MockAuthService{
		MockAccessToken:  "access-token-123",
		MockRefreshToken: "refresh-token-123",
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := LoginReq{
		Email:    "test@example.com",
		Password: "Password123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/auth/login-admin", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestRefresh(t *testing.T) {
	mockSvc := &MockAuthService{
		MockAccessToken: "new-access-token",
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "refresh-token-123"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestLogout(t *testing.T) {
	mockSvc := &MockAuthService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: "refresh-token-123"})
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestForgotPassword(t *testing.T) {
	mockSvc := &MockAuthService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := ForgotPasswordReq{
		Email: "test@example.com",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/auth/forgot-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestResetPassword(t *testing.T) {
	mockSvc := &MockAuthService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := ResetPasswordReq{
		Code:        uuid.New().String(),
		NewPassword: "NewPassword123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/auth/reset-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestChangePassword(t *testing.T) {
	mockSvc := &MockAuthService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := ChangePasswordReq{
		OldPassword: "OldPassword123",
		NewPassword: "NewPassword123",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/auth/change-password", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", uuid.New().String())
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestRegister(t *testing.T) {
	mockSvc := &MockAuthService{
		VerifyCode: &model.VerifyCode{
			Code: uuid.New(),
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := RegisterReq{
		Email:      "test@example.com",
		Password:   "Password123",
		GivenName:  "John",
		FamilyName: "Doe",
		Birthdate:  "2000-01-01",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/register", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestResendEmail(t *testing.T) {
	mockSvc := &MockAuthService{
		VerifyCode: &model.VerifyCode{
			Code: uuid.New(),
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/register/resend-email?email=test@example.com", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestVerify(t *testing.T) {
	mockSvc := &MockAuthService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()
	payload := VerifyReq{
		Email: "test@example.com",
		Code:  uuid.New().String(),
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/register/verify", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}
