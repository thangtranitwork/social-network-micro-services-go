package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"social-network-go/search-service/model"
	"social-network-go/search-service/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestSearchHandler_EmptyQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.GET("/v1/search", h.Search)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?query=", nil)
	req.Header.Set("X-User-ID", "user-123")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.Code)
	assert.Equal(t, "OK", resp.Message)
}

func TestSearchHandler_ShortQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.GET("/v1/search", h.Search)

	req := httptest.NewRequest(http.MethodGet, "/v1/search?query=a&type=USER", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Code int                  `json:"code"`
		Body *model.SearchResults `json:"body"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.Code)
	assert.NotNil(t, resp.Body)
	assert.Empty(t, resp.Body.USER)
}

func TestGetCurrentUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Case 1: From Header
	c1, _ := gin.CreateTestContext(httptest.NewRecorder())
	c1.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	c1.Request.Header.Set("X-User-ID", "header-user-id")
	assert.Equal(t, "header-user-id", getCurrentUser(c1))

	// Case 2: From Query
	c2, _ := gin.CreateTestContext(httptest.NewRecorder())
	c2.Request = httptest.NewRequest(http.MethodGet, "/?current_user_id=query-user-id", nil)
	assert.Equal(t, "query-user-id", getCurrentUser(c2))

	// Case 3: Empty
	c3, _ := gin.CreateTestContext(httptest.NewRecorder())
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	assert.Equal(t, "", getCurrentUser(c3))
}

func TestSyncData_ForbiddenForRegularUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.POST("/v1/search/sync", h.SyncData)

	req := httptest.NewRequest(http.MethodPost, "/v1/search/sync", nil)
	req.Header.Set("X-User-Role", "USER")
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.Code)
}

func TestSyncData_ForbiddenWithoutAdminHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.POST("/v1/search/sync", h.SyncData)

	req := httptest.NewRequest(http.MethodPost, "/v1/search/sync", nil)
	// No X-User-Role header
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 403, resp.Code)
}

func TestSearchHandler_SearchByImage_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.POST("/v1/search/image", h.SearchByImage)

	// Case 1: Missing form file
	req := httptest.NewRequest(http.MethodPost, "/v1/search/image", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestSearchHandler_SearchVisualText_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.GET("/v1/search/visual", h.SearchVisualText)

	// Case 1: Empty query
	req := httptest.NewRequest(http.MethodGet, "/v1/search/visual?query=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.Code)
}

func TestSearchHandler_SearchSimilar_Validation(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.GET("/v1/search/similar", h.SearchSimilar)

	// Case 1: Empty file_id
	req := httptest.NewRequest(http.MethodGet, "/v1/search/similar?file_id=", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp ApiResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(t, err)
	assert.Equal(t, 400, resp.Code)
}

func TestReindex_ForbiddenForRegularUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.POST("/v1/search/reindex", h.Reindex)

	req := httptest.NewRequest(http.MethodPost, "/v1/search/reindex", strings.NewReader(`{"alias":"posts","target_index":"posts_v2"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "USER")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestReindex_InvalidBody(t *testing.T) {
	gin.SetMode(gin.TestMode)

	svc := &service.SearchService{}
	h := NewSearchHandler(svc)

	r := gin.New()
	r.POST("/v1/search/reindex", h.Reindex)

	req := httptest.NewRequest(http.MethodPost, "/v1/search/reindex", strings.NewReader(`{"alias":""}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "ADMIN")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}


