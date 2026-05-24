package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"social-network-go/user-service/model"
	"social-network-go/user-service/service"

	"github.com/gin-gonic/gin"
)

type MockUserRepository struct {
	User  *model.User
	Users []*model.User
	Err   error
}

func (m *MockUserRepository) EnsureProfile(ctx context.Context, id, email, givenName, familyName, birthdate string) (*model.User, error) {
	return m.User, m.Err
}

func (m *MockUserRepository) GetUserProfile(ctx context.Context, usernameOrID string, currentUserID string) (*model.User, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.User != nil {
		return m.User, nil
	}
	return &model.User{
		ID:        "user-123",
		Username:  "testuser",
		Email:     "test@test.com",
		GivenName: "Test",
	}, nil
}

func (m *MockUserRepository) GetFriends(ctx context.Context, username string, currentUserID string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) GetSuggestedFriends(ctx context.Context, currentUserID string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) GetMutualFriends(ctx context.Context, currentUserID string, targetUsername string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) Unfriend(ctx context.Context, currentUserID string, targetUsername string) error {
	return m.Err
}

func (m *MockUserRepository) Block(ctx context.Context, currentUserID string, targetUsername string) error {
	return m.Err
}

func (m *MockUserRepository) Unblock(ctx context.Context, currentUserID string, targetUsername string) error {
	return m.Err
}

func (m *MockUserRepository) GetBlockedUsers(ctx context.Context, currentUserID string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) SendFriendRequest(ctx context.Context, currentUserID string, targetID string, requestReceivedCount int) error {
	return m.Err
}

func (m *MockUserRepository) AcceptFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	return m.Err
}

func (m *MockUserRepository) DeleteFriendRequest(ctx context.Context, currentUserID string, targetUsername string) error {
	return m.Err
}

func (m *MockUserRepository) GetSentRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) GetReceivedRequests(ctx context.Context, currentUserID string) ([]*model.User, error) {
	return m.Users, m.Err
}

func (m *MockUserRepository) UpdateBio(ctx context.Context, currentUserID string, bio string) error {
	return m.Err
}

func (m *MockUserRepository) UpdateBirthdate(ctx context.Context, currentUserID string, birthdate string, nextDate string) error {
	return m.Err
}

func (m *MockUserRepository) UpdateName(ctx context.Context, currentUserID string, familyName, givenName string, nextDate string) error {
	return m.Err
}

func (m *MockUserRepository) UpdateUsername(ctx context.Context, currentUserID string, username string, nextDate string) error {
	return m.Err
}

func (m *MockUserRepository) UpdateProfilePicture(ctx context.Context, currentUserID string, fileID string) error {
	return m.Err
}

func setupTestRouter(repo *MockUserRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := &service.UserService{
		UserRepo: repo,
	}
	h := NewUserHandler(svc)

	r.GET("/v1/users/:username", h.GetUserProfile)
	r.PATCH("/v1/users/update-bio", h.UpdateBio)
	r.PATCH("/v1/users/update-birthdate", h.UpdateBirthdate)
	r.PATCH("/v1/users/update-name", h.UpdateName)
	r.PATCH("/v1/users/update-username", h.UpdateUsername)
	r.PATCH("/v1/users/update-profile-picture", h.UpdateProfilePicture)

	r.GET("/v1/friends/suggested", h.GetSuggestedFriends)
	r.GET("/v1/friends/mutual-friends/:username", h.GetMutualFriends)
	r.GET("/v1/friends/:username", h.GetFriends)
	r.DELETE("/v1/friends/:username", h.Unfriend)

	r.GET("/v1/blocks", h.GetBlockedUsers)
	r.POST("/v1/blocks/:username", h.Block)
	r.DELETE("/v1/blocks/:username", h.Unblock)

	r.GET("/v1/friend-request/sent-requests", h.GetSentRequests)
	r.GET("/v1/friend-request/received-requests", h.GetReceivedRequests)
	r.POST("/v1/friend-request/send/:username", h.SendFriendRequest)
	r.POST("/v1/friend-request/accept/:username", h.AcceptFriendRequest)
	r.DELETE("/v1/friend-request/delete/:username", h.DeleteFriendRequest)

	return r
}

func TestGetUserProfile(t *testing.T) {
	mockRepo := &MockUserRepository{
		User: &model.User{ID: "user-1", Username: "testuser", CreatedAt: time.Now()},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/users/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdateBio(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"bio": "my new bio",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/users/update-bio", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdateBirthdate(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"birthdate": "2000-01-01",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/users/update-birthdate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateName(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"familyName": "Doe",
		"givenName":  "John",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/users/update-name", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdateUsername(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"username": "newusername",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/users/update-username", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdateProfilePicture(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"fileId": "image-uuid-1",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/users/update-profile-picture", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGetSuggestedFriends(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/friends/suggested", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetMutualFriends(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/friends/mutual-friends/otheruser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetFriends(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/friends/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUnfriend(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/friends/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetBlockedUsers(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/blocks", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestBlock(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/blocks/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUnblock(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/blocks/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetSentRequests(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/friend-request/sent-requests", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetReceivedRequests(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/friend-request/received-requests", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestSendFriendRequest(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/friend-request/send/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestAcceptFriendRequest(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/friend-request/accept/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestDeleteFriendRequest(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/friend-request/delete/testuser", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}
