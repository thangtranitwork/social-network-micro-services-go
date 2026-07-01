package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"social-network-go/file-service/model"

	"github.com/gin-gonic/gin"
)

type MockFileService struct {
	File      *model.File
	FileResp  *model.FileResponse
	UploadID  string
	UploadURL string
	Url       string
	Err       error
}

func (m *MockFileService) Upload(ctx context.Context, file io.Reader, filename string, contentType string, uploaderID string) (*model.File, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.File, nil
}

func (m *MockFileService) GetPresignedUploadURL(ctx context.Context, filename string, contentType string) (string, string, error) {
	if m.Err != nil {
		return "", "", m.Err
	}
	return m.UploadID, m.UploadURL, nil
}

func (m *MockFileService) Load(ctx context.Context, id string) (io.ReadCloser, string, string, int64, error) {
	if m.Err != nil {
		return nil, "", "", 0, m.Err
	}
	content := []byte("dummy file content")
	reader := io.NopCloser(bytes.NewReader(content))
	return reader, "test.jpg", "image/jpeg", int64(len(content)), nil
}

func (m *MockFileService) GetPresignedURL(ctx context.Context, id string) (string, error) {
	if m.Err != nil {
		return "", m.Err
	}
	return m.Url, nil
}

func (m *MockFileService) DeleteFile(ctx context.Context, id string) error {
	return m.Err
}

func (m *MockFileService) DeleteFiles(ctx context.Context, ids []string) error {
	return m.Err
}

func (m *MockFileService) DeleteFileForUser(ctx context.Context, id, userID string, isAdmin bool) error {
	return m.Err
}

func (m *MockFileService) DeleteFilesForUser(ctx context.Context, ids []string, userID string, isAdmin bool) error {
	return m.Err
}

func setupTestRouter(svc *MockFileService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewFileHandler(svc)

	v1 := r.Group("/v1/files")
	{
		v1.POST("/upload", h.Upload)
		v1.POST("/upload-multiple", h.UploadMultiple)
		v1.GET("/upload/presigned", h.GetPresignedUploadURL)
		v1.GET("/:id", h.Load)
		v1.GET("/:id/presigned", h.GetPresignedURL)
		v1.DELETE("/:id", h.Delete)
		v1.POST("/delete-multiple", h.DeleteMultiple)
	}

	return r
}

func TestUpload(t *testing.T) {
	mockSvc := &MockFileService{
		File: &model.File{
			ID:          "file-123.jpg",
			Name:        "test.jpg",
			ContentType: "image/jpeg",
			UploaderID:  "user-1",
			CreatedAt:   time.Now(),
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("file", "test.jpg")
	part.Write([]byte("dummy content"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/v1/files/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "user-1")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestUploadMultiple(t *testing.T) {
	mockSvc := &MockFileService{
		File: &model.File{
			ID:          "file-123.jpg",
			Name:        "test.jpg",
			ContentType: "image/jpeg",
			UploaderID:  "user-1",
			CreatedAt:   time.Now(),
		},
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part1, _ := writer.CreateFormFile("files", "test1.jpg")
	part1.Write([]byte("dummy content 1"))
	part2, _ := writer.CreateFormFile("files", "test2.jpg")
	part2.Write([]byte("dummy content 2"))
	writer.Close()

	req, _ := http.NewRequest("POST", "/v1/files/upload-multiple", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-User-ID", "user-1")

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestGetPresignedUploadURL(t *testing.T) {
	mockSvc := &MockFileService{
		UploadID:  "file-123",
		UploadURL: "http://minio/upload",
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/files/upload/presigned?filename=test.jpg&contentType=image/jpeg", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestLoad(t *testing.T) {
	mockSvc := &MockFileService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/files/file-123.jpg", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
	if w.Header().Get("Content-Type") != "image/jpeg" {
		t.Errorf("Expected Content-Type image/jpeg, got %s", w.Header().Get("Content-Type"))
	}
	if w.Body.String() != "dummy file content" {
		t.Errorf("Expected dummy file content, got %s", w.Body.String())
	}
}

func TestGetPresignedURL(t *testing.T) {
	mockSvc := &MockFileService{
		Url: "http://minio/file-123.jpg?token=abc",
	}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("GET", "/v1/files/file-123.jpg/presigned", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}
}

func TestDelete(t *testing.T) {
	mockSvc := &MockFileService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("DELETE", "/v1/files/file-123.jpg", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", w.Code)
	}
}

func TestDeleteRequiresAuthenticatedUser(t *testing.T) {
	mockSvc := &MockFileService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("DELETE", "/v1/files/file-123.jpg", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestDeleteForbidden(t *testing.T) {
	mockSvc := &MockFileService{Err: errors.New("forbidden file delete")}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	req, _ := http.NewRequest("DELETE", "/v1/files/file-123.jpg", nil)
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

func TestDeleteMultiple(t *testing.T) {
	mockSvc := &MockFileService{}
	r := setupTestRouter(mockSvc)
	w := httptest.NewRecorder()

	payload := struct {
		IDs []string `json:"ids"`
	}{
		IDs: []string{"file1.jpg", "file2.jpg"},
	}
	body, _ := json.Marshal(payload)
	req, _ := http.NewRequest("POST", "/v1/files/delete-multiple", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "user-1")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204, got %d", w.Code)
	}
}
