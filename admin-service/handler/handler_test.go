package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"social-network-go/admin-service/model"
	"social-network-go/admin-service/service"

	"github.com/gin-gonic/gin"
)

type MockAdminRepository struct {
	UsersStats *model.UserStatisticsResponse
	PostsStats *model.PostStatisticsResponse
	Posts      []model.PostResponse
	Users      []model.UserDetailResponse
	WeekStats  map[string]int
	MonthStats map[string]int
	YearStats  map[string]int
	Err        error
}

func (m *MockAdminRepository) GetUsersStatistics(ctx context.Context) (*model.UserStatisticsResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.UsersStats, nil
}

func (m *MockAdminRepository) GetPostsStatistics(ctx context.Context) (*model.PostStatisticsResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.PostsStats, nil
}

func (m *MockAdminRepository) GetPostsList(ctx context.Context, skip, limit int) ([]model.PostResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Posts, nil
}

func (m *MockAdminRepository) GetUsersList(ctx context.Context, skip, limit int) ([]model.UserDetailResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Users, nil
}

func (m *MockAdminRepository) QueryWeekUserStats(ctx context.Context, week, year int) (map[string]int, error) {
	return m.WeekStats, m.Err
}

func (m *MockAdminRepository) QueryMonthUserStats(ctx context.Context, month, year int) (map[string]int, error) {
	return m.MonthStats, m.Err
}

func (m *MockAdminRepository) QueryYearUserStats(ctx context.Context, year int) (map[string]int, error) {
	return m.YearStats, m.Err
}

func (m *MockAdminRepository) QueryWeekPostStats(ctx context.Context, week, year int) (map[string]int, error) {
	return m.WeekStats, m.Err
}

func (m *MockAdminRepository) QueryMonthPostStats(ctx context.Context, month, year int) (map[string]int, error) {
	return m.MonthStats, m.Err
}

func (m *MockAdminRepository) QueryYearPostStats(ctx context.Context, year int) (map[string]int, error) {
	return m.YearStats, m.Err
}

func (m *MockAdminRepository) DeletePost(ctx context.Context, postID string) error {
	return m.Err
}

func (m *MockAdminRepository) SuspendUser(ctx context.Context, userID string, duration time.Duration) error {
	return m.Err
}

func (m *MockAdminRepository) UnsuspendUser(ctx context.Context, userID string) error {
	return m.Err
}

func setupTestRouter(repo *MockAdminRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := service.NewAdminService(repo)
	h := NewAdminHandler(svc)
	h.RegisterRoutes(r)
	return r
}

func TestHealth(t *testing.T) {
	r := setupTestRouter(&MockAdminRepository{})
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetUsersStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		UsersStats: &model.UserStatisticsResponse{TotalUsers: 100},
		WeekStats:  map[string]int{"MONDAY": 5},
		MonthStats: map[string]int{"1": 10},
		YearStats:  map[string]int{"JANUARY": 50},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/users", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse body: %v", err)
	}
	body := resp["body"].(map[string]interface{})
	if int(body["totalUsers"].(float64)) != 100 {
		t.Errorf("Expected 100 users, got %v", body["totalUsers"])
	}
}

func TestGetUsersWeekStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		WeekStats: map[string]int{"MONDAY": 15},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/users/week?week=2026-W21", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetUsersMonthStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		MonthStats: map[string]int{"1": 25},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/users/month?month=2026-05", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetUsersYearStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		YearStats: map[string]int{"JANUARY": 35},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/users/year?year=2026", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetUsersOnlineStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/users/online", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		PostsStats: &model.PostStatisticsResponse{TotalPosts: 50},
		WeekStats:  map[string]int{"MONDAY": 5},
		MonthStats: map[string]int{"1": 10},
		YearStats:  map[string]int{"JANUARY": 50},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/posts", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsWeekStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		WeekStats: map[string]int{"TUESDAY": 12},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/posts/week?week=2026-W21", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsMonthStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		MonthStats: map[string]int{"10": 42},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/posts/month?month=2026-05", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsYearStatistics(t *testing.T) {
	mockRepo := &MockAdminRepository{
		YearStats: map[string]int{"DECEMBER": 99},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v2/statistics/posts/year?year=2026", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsList(t *testing.T) {
	mockRepo := &MockAdminRepository{
		Posts: []model.PostResponse{
			{
				ID:      "post-1",
				Content: "Test content",
				Author: model.CreatorInfo{
					ID: "author-1",
				},
			},
		},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetUsersList(t *testing.T) {
	mockRepo := &MockAdminRepository{
		Users: []model.UserDetailResponse{
			{
				ID:       "user-1",
				Username: "testuser",
			},
		},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/users?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}
