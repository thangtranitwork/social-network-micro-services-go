package handler

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"social-network-go/logger"
	"social-network-go/search-service/service"

	"github.com/gin-gonic/gin"
)

type SearchHandler struct {
	SearchSvc *service.SearchService
}

func NewSearchHandler(searchSvc *service.SearchService) *SearchHandler {
	return &SearchHandler{SearchSvc: searchSvc}
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("query")
	searchType := c.Query("type")
	userID := getCurrentUser(c)

	results, err := h.SearchSvc.Search(c.Request.Context(), query, userID, searchType)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Search failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Internal Server Error: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      results,
	})
}

// SearchByImage handles Image-to-Image visual search via multipart upload
func (h *SearchHandler) SearchByImage(c *gin.Context) {
	userID := getCurrentUser(c)
	rateKey := userID
	if rateKey == "" {
		rateKey = c.ClientIP()
	}

	// Rate limiting: 10 req/min
	if !h.SearchSvc.CheckRateLimit(c.Request.Context(), "ratelimit:visual_search:image:"+rateKey, 10, time.Minute) {
		c.JSON(http.StatusTooManyRequests, ApiResponse{
			Code:      429,
			Message:   "RATE_LIMIT_EXCEEDED: Quá giới hạn 10 lượt tìm kiếm hình ảnh mỗi phút",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Missing or invalid 'image' form file: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}
	defer file.Close()

	// Max 10MB
	if header.Size > 10<<20 {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "File size exceeds 10MB limit",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Validate content-type
	contentType := header.Header.Get("Content-Type")
	if contentType != "" && !strings.HasPrefix(contentType, "image/") {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Invalid image type. Only JPEG, PNG, and WebP are supported.",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	imageBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Failed to read image content: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	limit := getLimit(c, 20, 50)
	results, err := h.SearchSvc.SearchByImage(c.Request.Context(), imageBytes, userID, limit)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Visual SearchByImage failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Visual search failed: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      results,
	})
}

// SearchVisualText handles Text-to-Image cross-modal search
func (h *SearchHandler) SearchVisualText(c *gin.Context) {
	userID := getCurrentUser(c)
	rateKey := userID
	if rateKey == "" {
		rateKey = c.ClientIP()
	}

	// Rate limiting: 30 req/min
	if !h.SearchSvc.CheckRateLimit(c.Request.Context(), "ratelimit:visual_search:text:"+rateKey, 30, time.Minute) {
		c.JSON(http.StatusTooManyRequests, ApiResponse{
			Code:      429,
			Message:   "RATE_LIMIT_EXCEEDED: Quá giới hạn 30 lượt tìm kiếm qua mô tả ảnh mỗi phút",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Query parameter 'query' is required",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	limit := getLimit(c, 20, 50)
	results, err := h.SearchSvc.SearchImageByText(c.Request.Context(), query, userID, limit)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Visual SearchVisualText failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Visual text search failed: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      results,
	})
}

// SearchSimilar handles 1-click similar image search by existing file_id
func (h *SearchHandler) SearchSimilar(c *gin.Context) {
	userID := getCurrentUser(c)
	rateKey := userID
	if rateKey == "" {
		rateKey = c.ClientIP()
	}

	// Rate limiting: 60 req/min
	if !h.SearchSvc.CheckRateLimit(c.Request.Context(), "ratelimit:visual_search:similar:"+rateKey, 60, time.Minute) {
		c.JSON(http.StatusTooManyRequests, ApiResponse{
			Code:      429,
			Message:   "RATE_LIMIT_EXCEEDED: Quá giới hạn 60 lượt tìm ảnh tương tự mỗi phút",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	fileID := strings.TrimSpace(c.Query("file_id"))
	if fileID == "" {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Query parameter 'file_id' is required",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	limit := getLimit(c, 20, 50)
	results, err := h.SearchSvc.SearchSimilarByFileID(c.Request.Context(), fileID, userID, limit)
	if err != nil {
		if strings.Contains(err.Error(), "SOURCE_FILE_NOT_FOUND_OR_FORBIDDEN") {
			c.JSON(http.StatusForbidden, ApiResponse{
				Code:      403,
				Message:   "Không tìm thấy ảnh hoặc bạn không có quyền xem bài viết chứa ảnh này",
				Timestamp: time.Now().Format(time.RFC3339),
			})
			return
		}
		logger.WithContext(c.Request.Context()).Err(err).Error("Visual SearchSimilar failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Similar search failed: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      results,
	})
}

func (h *SearchHandler) SyncData(c *gin.Context) {
	// Restrict sync strictly to ADMIN users
	if role := c.GetHeader("X-User-Role"); role != "ADMIN" {
		c.JSON(http.StatusForbidden, ApiResponse{
			Code:      403,
			Message:   "Forbidden: Only administrators can trigger sync",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	usersCount, postsCount, err := h.SearchSvc.SyncData(c.Request.Context())
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Sync data to Elasticsearch failed")
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Sync failed: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "Sync complete",
		Timestamp: time.Now().Format(time.RFC3339),
		Body: gin.H{
			"users_synced": usersCount,
			"posts_synced": postsCount,
		},
	})
}

type ReindexRequest struct {
	Alias       string `json:"alias" binding:"required"`
	TargetIndex string `json:"target_index" binding:"required"`
}

// Reindex handles zero-downtime reindexing requests for administrators
func (h *SearchHandler) Reindex(c *gin.Context) {
	// Restrict reindex strictly to ADMIN users
	if role := c.GetHeader("X-User-Role"); role != "ADMIN" {
		c.JSON(http.StatusForbidden, ApiResponse{
			Code:      403,
			Message:   "Forbidden: Only administrators can trigger reindex",
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	var req ReindexRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ApiResponse{
			Code:      400,
			Message:   "Invalid request body: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	if err := h.SearchSvc.Reindex(c.Request.Context(), req.Alias, req.TargetIndex); err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Reindex failed for %s -> %s", req.Alias, req.TargetIndex)
		c.JSON(http.StatusInternalServerError, ApiResponse{
			Code:      500,
			Message:   "Reindex failed: " + err.Error(),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}

	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "Zero-downtime reindex completed successfully",
		Timestamp: time.Now().Format(time.RFC3339),
		Body: gin.H{
			"alias":        req.Alias,
			"target_index": req.TargetIndex,
			"status":       "MIGRATED_AND_ALIASED",
		},
	})
}

func getLimit(c *gin.Context, defaultVal, maxVal int) int {
	lStr := c.Query("limit")
	if lStr == "" {
		return defaultVal
	}
	l, err := strconv.Atoi(lStr)
	if err != nil || l <= 0 {
		return defaultVal
	}
	if l > maxVal {
		return maxVal
	}
	return l
}

func getCurrentUser(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	return userID
}
