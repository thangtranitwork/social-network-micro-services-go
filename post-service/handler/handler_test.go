package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"social-network-go/post-service/config"
	"social-network-go/post-service/model"
	"social-network-go/post-service/service"

	"github.com/gin-gonic/gin"
)

type MockPostRepository struct {
	Post       *model.Post
	Posts      []*model.Post
	CommentObj *model.Comment
	Comments   []*model.Comment
	FileIDs    []string
	Err        error
}

func (m *MockPostRepository) CreatePost(ctx context.Context, postID, authorID, content, privacy string, fileIDs []string) error {
	return m.Err
}

func (m *MockPostRepository) SharePost(ctx context.Context, authorID, originalPostID, content, privacy string, postID string) error {
	return m.Err
}

func (m *MockPostRepository) GetPost(ctx context.Context, postID string, currentUserID string) (*model.Post, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Post != nil {
		return m.Post, nil
	}
	return &model.Post{ID: postID, AuthorID: "author-1", Privacy: "PUBLIC"}, nil
}

func (m *MockPostRepository) GetPostsOfUser(ctx context.Context, authorUsername string, currentUserID string, skip, limit int64) ([]*model.Post, error) {
	return m.Posts, m.Err
}

func (m *MockPostRepository) GetAllPosts(ctx context.Context, skip, limit int64) ([]*model.Post, error) {
	return m.Posts, m.Err
}

func (m *MockPostRepository) GetSuggestedPosts(ctx context.Context, currentUserID string, pageType string, skip, limit int64) ([]*model.Post, error) {
	return m.Posts, m.Err
}

func (m *MockPostRepository) UpdatePrivacy(ctx context.Context, currentUserID, postID, privacy string) error {
	return m.Err
}

func (m *MockPostRepository) UpdateContent(ctx context.Context, currentUserID, postID string, content *string, newFileIDs []string, deleteOldFileIDs []string, maxPostAttachFiles int) ([]string, string, error) {
	var c string
	if content != nil {
		c = *content
	}
	return m.FileIDs, c, m.Err
}

func (m *MockPostRepository) LikePost(ctx context.Context, userID, postID string) error {
	return m.Err
}

func (m *MockPostRepository) UnlikePost(ctx context.Context, userID, postID string) error {
	return m.Err
}

func (m *MockPostRepository) DeletePost(ctx context.Context, postID, currentUserID string, isAdmin bool) (string, []string, error) {
	return "author-1", m.FileIDs, m.Err
}

func (m *MockPostRepository) ValidateBlockByIDs(ctx context.Context, userID, targetID string) error {
	return m.Err
}

func (m *MockPostRepository) ValidateBlockByUsername(ctx context.Context, userID, targetUsername string) error {
	return m.Err
}

func (m *MockPostRepository) IsFriendByIDs(ctx context.Context, userID, targetID string) (bool, error) {
	return true, m.Err
}

func (m *MockPostRepository) Comment(ctx context.Context, commentID, authorID, postID, content string, fileID *string) error {
	return m.Err
}

func (m *MockPostRepository) ReplyComment(ctx context.Context, commentID, authorID, originalCommentID, postID, content string, fileID *string) error {
	return m.Err
}

func (m *MockPostRepository) LikeComment(ctx context.Context, likerID, commentID string) error {
	return m.Err
}

func (m *MockPostRepository) UnlikeComment(ctx context.Context, likerID, commentID string) error {
	return m.Err
}

func (m *MockPostRepository) UpdateCommentContent(ctx context.Context, currentUserID, commentID, content string) error {
	return m.Err
}

func (m *MockPostRepository) DeleteComment(ctx context.Context, currentUserID, commentID string, isAdmin bool, postAuthorID string) ([]string, error) {
	return m.FileIDs, m.Err
}

func (m *MockPostRepository) GetComments(ctx context.Context, postID, currentUserID string, pageType string, skip, limit int64) ([]*model.Comment, error) {
	return m.Comments, m.Err
}

func (m *MockPostRepository) GetRepliedComments(ctx context.Context, originalCommentID, currentUserID string, skip, limit int64) ([]*model.Comment, error) {
	return m.Comments, m.Err
}

func (m *MockPostRepository) GetCommentByID(ctx context.Context, commentID string, currentUserID string) (*model.Comment, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.CommentObj != nil {
		return m.CommentObj, nil
	}
	return &model.Comment{ID: commentID, AuthorID: "author-1", PostID: "post-1"}, nil
}

func (m *MockPostRepository) GetFilesInPostsOfUser(ctx context.Context, username string, skip, limit int64) ([]string, error) {
	return m.FileIDs, m.Err
}

func setupTestRouter(repo *MockPostRepository) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfg := &config.Config{
		UserGrpcAddr: "localhost:10052",
		RedisAddr:    "localhost:6379",
	}
	svc := service.NewPostService(cfg, repo)
	h := NewPostHandler(svc)

	// Register Routes
	r.GET("/v1/posts/newsfeed", h.GetNewsfeed)
	r.GET("/v1/posts/of-user/:username", h.GetPostsOfUser)
	r.GET("/v1/posts/:id", h.GetPost)
	r.POST("/v1/posts/post", h.CreatePost)
	r.POST("/v1/posts/share", h.SharePost)
	r.POST("/v1/posts/like/:postId", h.LikePost)
	r.PATCH("/v1/posts/update-privacy/:postId", h.UpdatePrivacy)
	r.PATCH("/v1/posts/update-content/:postId", h.UpdateContent)
	r.DELETE("/v1/posts/unlike/:postId", h.UnlikePost)
	r.DELETE("/v1/posts/:postId", h.DeletePost)
	r.GET("/v1/posts/files/:username", h.GetFilesInPostsOfUser)

	r.POST("/v1/posts/comment", h.Comment)
	r.POST("/v1/posts/reply-comment", h.ReplyComment)
	r.POST("/v1/posts/like-comment/:commentId", h.LikeComment)
	r.DELETE("/v1/posts/unlike-comment/:commentId", h.UnlikeComment)
	r.GET("/v1/posts/comments/:postId", h.GetComments)
	r.GET("/v1/posts/replied-comments/:commentId", h.GetRepliedComments)
	r.DELETE("/v1/posts/comment/:commentId", h.DeleteComment)

	return r
}

func TestGetNewsfeed(t *testing.T) {
	mockRepo := &MockPostRepository{
		Posts: []*model.Post{
			{ID: "post-1", Content: "Hello", AuthorID: "author-1", CreatedAt: time.Now()},
		},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/newsfeed?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPostsOfUser(t *testing.T) {
	mockRepo := &MockPostRepository{
		Posts: []*model.Post{
			{ID: "post-1", Content: "Hello", AuthorID: "author-1", CreatedAt: time.Now()},
		},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/of-user/testuser?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetPost(t *testing.T) {
	mockRepo := &MockPostRepository{
		Post: &model.Post{ID: "post-1", Content: "Hello", AuthorID: "author-1", Privacy: "PUBLIC", CreatedAt: time.Now()},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/post-1", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestCreatePost(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"content": "new post",
		"privacy": "PUBLIC",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/posts/post", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestSharePost(t *testing.T) {
	mockRepo := &MockPostRepository{
		Post: &model.Post{ID: "post-1", Content: "Hello", AuthorID: "author-1", Privacy: "PUBLIC", CreatedAt: time.Now()},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"originalPostId": "post-1",
		"content":        "shared!",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/posts/share", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestLikePost(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/posts/like/post-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdatePrivacy(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PATCH", "/v1/posts/update-privacy/post-1?privacy=FRIEND", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUpdateContent(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	content := "updated content"
	payload := map[string]interface{}{
		"content": &content,
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("PATCH", "/v1/posts/update-content/post-1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUnlikePost(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/posts/unlike/post-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestDeletePost(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/posts/post-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetFilesInPostsOfUser(t *testing.T) {
	mockRepo := &MockPostRepository{
		FileIDs: []string{"file-1"},
	}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/files/testuser?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestComment(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"postId":  "post-1",
		"content": "nice comment",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/posts/comment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestReplyComment(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	payload := map[string]interface{}{
		"originalCommentId": "comment-1",
		"content":           "reply content",
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/posts/reply-comment", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestLikeComment(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/v1/posts/like-comment/comment-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUnlikeComment(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/posts/unlike-comment/comment-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetComments(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/comments/post-1?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestGetRepliedComments(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/v1/posts/replied-comments/comment-1?skip=0&limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestDeleteComment(t *testing.T) {
	mockRepo := &MockPostRepository{}
	r := setupTestRouter(mockRepo)
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/v1/posts/comment/comment-1", nil)
	req.Header.Set("X-User-ID", "author-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}
