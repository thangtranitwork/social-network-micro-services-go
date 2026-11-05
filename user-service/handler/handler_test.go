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

func (m *MockUserRepository) SendFriendRequest(ctx context.Context, currentUserID string, targetID string, requestReceivedCount int, maxReceivedRequestCount int) error {
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

func (m *MockUserRepository) RecordProfileView(ctx context.Context, viewerID, targetID string) error {
	return m.Err
}

func (m *MockUserRepository) UpdateNotificationPreferences(ctx context.Context, currentUserID string, email bool, push bool, digest string) error {
	return m.Err
}

func (m *MockUserRepository) GetUserSpaceConfig(ctx context.Context, targetUserID string) (*model.UserSpaceConfig, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return &model.UserSpaceConfig{
		UserID:          targetUserID,
		IsConfigured:    false,
		Theme:           "COSMIC_VOID",
		DefaultViewMode: "SPACE",
		CustomAccent:    "#6366F1",
		AllowGuestbook:  true,
		Nodes: []model.SpatialNode{
			{
				ID:         "node_identity_core",
				Type:       "IDENTITY_CORE",
				X:          0,
				Y:          0,
				Scale:      1.0,
				Rotation:   0,
				ZIndex:     10,
				FrameStyle: "GLASS",
				Content:    map[string]interface{}{"subtitle": "Hello"},
			},
		},
		GuestbookEntries: []model.SpaceGuestbookEntry{},
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *MockUserRepository) SaveUserSpaceConfig(ctx context.Context, userID string, req model.UpdateSpaceConfigRequest) (*model.UserSpaceConfig, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	allow := true
	if req.AllowGuestbook != nil {
		allow = *req.AllowGuestbook
	}
	return &model.UserSpaceConfig{
		UserID:           userID,
		IsConfigured:     true,
		Theme:            req.Theme,
		DefaultViewMode:  req.DefaultViewMode,
		CustomAccent:     req.CustomAccent,
		AllowGuestbook:   allow,
		Nodes:            req.Nodes,
		GuestbookEntries: []model.SpaceGuestbookEntry{},
		UpdatedAt:        time.Now(),
	}, nil
}

func (m *MockUserRepository) AddSpaceGuestbookEntry(ctx context.Context, entry *model.SpaceGuestbookEntry) (*model.SpaceGuestbookEntry, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	entry.AuthorUsername = "visitor"
	return entry, nil
}

func (m *MockUserRepository) DeleteSpaceGuestbookEntry(ctx context.Context, entryID string, actorUserID string) error {
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
	r.GET("/v1/users/:username/space", h.GetUserSpace)
	r.PUT("/v1/users/space", h.UpdateUserSpace)
	r.POST("/v1/users/:username/space/guestbook", h.AddSpaceGuestbookEntry)
	r.DELETE("/v1/users/space/guestbook/:entryId", h.DeleteSpaceGuestbookEntry)
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

func TestGetUserSpace(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/users/testuser/space", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestUpdateUserSpace_ValidAndInvalid(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)

	// 1. Valid payload
	validPayload := map[string]interface{}{
		"theme":           "COSMIC_VOID",
		"defaultViewMode": "SPACE",
		"customAccent":    "#6366F1",
		"allowGuestbook":  true,
		"nodes": []map[string]interface{}{
			{
				"id":         "node_identity_core",
				"type":       "IDENTITY_CORE",
				"x":          0,
				"y":          0,
				"scale":      1.0,
				"rotation":   0,
				"zIndex":     10,
				"frameStyle": "GLASS",
				"content":    map[string]interface{}{"subtitle": "Curator"},
			},
			{
				"id":         "node_quote_1",
				"type":       "TEXT_NOTE",
				"x":          320,
				"y":          -140,
				"scale":      1.1,
				"rotation":   -3,
				"zIndex":     12,
				"frameStyle": "SERIF_EDITORIAL",
				"content":    map[string]interface{}{"text": "Stay curious"},
			},
		},
	}
	body, _ := json.Marshal(validPayload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/v1/users/space", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 for valid space config, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Invalid payload (missing IDENTITY_CORE node - strict validation fail-fast)
	invalidPayload := map[string]interface{}{
		"theme":           "COSMIC_VOID",
		"defaultViewMode": "SPACE",
		"customAccent":    "#6366F1",
		"allowGuestbook":  true,
		"nodes":           []map[string]interface{}{},
	}
	bodyInvalid, _ := json.Marshal(invalidPayload)
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest("PUT", "/v1/users/space", bytes.NewBuffer(bodyInvalid))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for missing IDENTITY_CORE, got %d", w2.Code)
	}
}

func TestSpaceGuestbookLifecycle(t *testing.T) {
	mockRepo := &MockUserRepository{}
	r := setupTestRouter(mockRepo)

	payload := map[string]interface{}{
		"message":   "Không gian trưng bày tuyệt vời quá!",
		"cardColor": "AMBER",
		"posX":      150,
		"posY":      200,
	}
	body, _ := json.Marshal(payload)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/users/testuser/space/guestbook", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-2")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200 on guestbook add, got %d. Body: %s", w.Code, w.Body.String())
	}

	wDel := httptest.NewRecorder()
	reqDel, _ := http.NewRequest("DELETE", "/v1/users/space/guestbook/entry-123", nil)
	reqDel.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(wDel, reqDel)

	if wDel.Code != http.StatusOK {
		t.Errorf("Expected 200 on guestbook delete, got %d", wDel.Code)
	}
}
