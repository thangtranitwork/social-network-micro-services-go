package handler

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"social-network-go/file-service/model"
	"strings"

	"github.com/gin-gonic/gin"
)

const MaxUploadSize = 50 * 1024 * 1024 // 50 MB

var allowedExtensions = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".webp": true,
	".mp4":  true,
	".mov":  true,
	".webm": true,
	".avi":  true,
	".mp3":  true,
	".wav":  true,
	".ogg":  true,
	".m4a":  true,
	".pdf":  true,
	".txt":  true,
	".docx": true,
	".xlsx": true,
	".pptx": true,
}

func isSafeInlineContentType(contentType, ext string) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "html") || strings.Contains(ct, "javascript") || strings.Contains(ct, "svg") || strings.Contains(ct, "xml") {
		return false
	}
	switch strings.ToLower(ext) {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".mp4", ".webm", ".mp3", ".wav", ".ogg":
		return true
	default:
		return false
	}
}

type FileServiceInterface interface {
	Upload(ctx context.Context, file io.Reader, filename string, contentType string, uploaderID string) (*model.File, error)
	GetPresignedUploadURL(ctx context.Context, filename string, contentType string) (string, string, error)
	Load(ctx context.Context, id string) (io.ReadCloser, string, string, int64, error)
	GetPresignedURL(ctx context.Context, id string) (string, error)
	DeleteFileForUser(ctx context.Context, id, userID string, isAdmin bool) error
	DeleteFilesForUser(ctx context.Context, ids []string, userID string, isAdmin bool) error
}

type FileHandler struct {
	fileSvc FileServiceInterface
}

func NewFileHandler(fileSvc FileServiceInterface) *FileHandler {
	return &FileHandler{fileSvc: fileSvc}
}

func (h *FileHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	if header.Size > MaxUploadSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file size exceeds limit"})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedExtensions[ext] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file format"})
		return
	}

	uploaderID := c.GetHeader("X-User-ID")
	if uploaderID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	result, err := h.fileSvc.Upload(c.Request.Context(), file, header.Filename, header.Header.Get("Content-Type"), uploaderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *FileHandler) UploadMultiple(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid form"})
		return
	}

	files := form.File["files"]
	if len(files) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no files uploaded"})
		return
	}

	uploaderID := c.GetHeader("X-User-ID")
	if uploaderID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var results []interface{}
	for _, fileHeader := range files {
		if fileHeader.Size > MaxUploadSize {
			results = append(results, gin.H{"error": "file size exceeds limit", "filename": fileHeader.Filename})
			continue
		}

		ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
		if !allowedExtensions[ext] {
			results = append(results, gin.H{"error": "unsupported file format", "filename": fileHeader.Filename})
			continue
		}

		file, err := fileHeader.Open()
		if err != nil {
			results = append(results, gin.H{"error": "failed to open file", "filename": fileHeader.Filename})
			continue
		}

		res, err := h.fileSvc.Upload(c.Request.Context(), file, fileHeader.Filename, fileHeader.Header.Get("Content-Type"), uploaderID)
		file.Close()

		if err != nil {
			results = append(results, gin.H{"error": err.Error(), "filename": fileHeader.Filename})
		} else {
			results = append(results, res)
		}
	}

	c.JSON(http.StatusOK, results)
}

func (h *FileHandler) GetPresignedUploadURL(c *gin.Context) {
	filename := c.Query("filename")
	contentType := c.Query("contentType")
	if contentType == "" {
		contentType = c.Query("content_type")
	}
	if filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "filename is required"})
		return
	}

	fileID, url, err := h.fileSvc.GetPresignedUploadURL(c.Request.Context(), filename, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":  fileID,
		"url": url,
	})
}

func (h *FileHandler) Load(c *gin.Context) {
	id := c.Param("id")
	reader, filename, contentType, size, err := h.fileSvc.Load(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no such key") {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	defer reader.Close()

	if queryFilename := c.Query("filename"); queryFilename != "" {
		filename = queryFilename
	} else if queryName := c.Query("name"); queryName != "" {
		filename = queryName
	}

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = filepath.Ext(id)
	}

	c.Header("X-Content-Type-Options", "nosniff")
	if isSafeInlineContentType(contentType, ext) {
		c.Header("Content-Disposition", "inline; filename=\""+filename+"\"")
	} else {
		c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	}
	c.Header("Cache-Control", "public, max-age=3600, immutable")
	c.DataFromReader(http.StatusOK, size, contentType, reader, nil)
}

func (h *FileHandler) GetPresignedURL(c *gin.Context) {
	id := c.Param("id")
	url, err := h.fileSvc.GetPresignedURL(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url})
}

func (h *FileHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err := h.fileSvc.DeleteFileForUser(c.Request.Context(), id, userID, c.GetHeader("X-User-Role") == "ADMIN")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

func (h *FileHandler) DeleteMultiple(c *gin.Context) {
	var req struct {
		IDs []string `json:"ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	err := h.fileSvc.DeleteFilesForUser(c.Request.Context(), req.IDs, userID, c.GetHeader("X-User-Role") == "ADMIN")
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "forbidden") {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
