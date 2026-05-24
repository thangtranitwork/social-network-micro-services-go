package handler

import (
	"net/http"
	"social-network-go/file-service/service"
	"strings"

	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	fileSvc *service.FileService
}

func NewFileHandler(fileSvc *service.FileService) *FileHandler {
	return &FileHandler{fileSvc: fileSvc}
}

func (h *FileHandler) Upload(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

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
	result, err := h.fileSvc.Load(c.Request.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "no such key") {
			c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, result)
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
	err := h.fileSvc.DeleteFile(c.Request.Context(), id)
	if err != nil {
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

	err := h.fileSvc.DeleteFiles(c.Request.Context(), req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}
