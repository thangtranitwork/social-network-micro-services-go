package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

type CreatePostRequest struct {
	Content string   `form:"content" json:"content"`
	Privacy string   `form:"privacy" json:"privacy" binding:"required"`
	Files   []string `form:"files" json:"files"`
}

type SharePostRequest struct {
	OriginalPostID string `json:"originalPostId" binding:"required"`
	Content        string `json:"content"`
	Privacy        string `json:"privacy"`
}

type UpdateContentRequest struct {
	Content          *string  `json:"content"`
	NewFiles         []string `json:"newFiles"`
	DeleteOldFileIDs []string `json:"deleteOldFileIds"`
}

func (h *PostHandler) GetNewsfeed(c *gin.Context) {
	userID := getCurrentUser(c)
	pageable := getPageable(c)

	posts, err := h.PostSvc.GetSuggestedPosts(c.Request.Context(), userID, pageable)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetNewsfeed failed")
		exception.SendError(c, exception.FailToGetPost)
		return
	}

	sendSuccess(c, posts)
}

func (h *PostHandler) GetPostsOfUser(c *gin.Context) {
	username := c.Param("username")
	userID := getCurrentUser(c)
	if username == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	posts, err := h.PostSvc.GetPostsOfUser(c.Request.Context(), username, userID, getPageable(c))
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetPostsOfUser failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.FailToGetPost)
		return
	}

	sendSuccess(c, posts)
}

func (h *PostHandler) GetPost(c *gin.Context) {
	postID := c.Param("id")
	userID := getCurrentUser(c)

	post, err := h.PostSvc.GetPost(c.Request.Context(), postID, userID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetPost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.PostNotFound)
		return
	}

	sendSuccess(c, post)
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req CreatePostRequest
	if err := c.ShouldBind(&req); err != nil {
		if errJSON := c.ShouldBindJSON(&req); errJSON != nil {
			exception.SendError(c, exception.InvalidInput)
			return
		}
	}

	if req.Files == nil {
		req.Files = []string{}
	}

	post, err := h.PostSvc.CreatePost(c.Request.Context(), userID, req.Content, req.Privacy, req.Files)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("CreatePost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.CreatePostFailed)
		return
	}

	sendSuccess(c, post)
}

func (h *PostHandler) SharePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req SharePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	post, err := h.PostSvc.SharePost(c.Request.Context(), userID, req.OriginalPostID, req.Content, req.Privacy)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("SharePost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.SharePostFailed)
		return
	}

	sendSuccess(c, post)
}

func (h *PostHandler) LikePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	postID := c.Param("postId")
	err := h.PostSvc.LikePost(c.Request.Context(), userID, postID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("LikePost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.LikePostFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UnlikePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	postID := c.Param("postId")
	err := h.PostSvc.UnlikePost(c.Request.Context(), userID, postID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UnlikePost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnlikePostFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UpdatePrivacy(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	postID := c.Param("postId")
	privacy := c.Query("privacy")
	if privacy == "" {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.PostSvc.UpdatePrivacy(c.Request.Context(), userID, postID, privacy)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdatePrivacy failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdatePrivacyFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UpdateContent(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	postID := c.Param("postId")
	var req UpdateContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	err := h.PostSvc.UpdateContent(c.Request.Context(), userID, postID, req.Content, req.NewFiles, req.DeleteOldFileIDs)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UpdateContent failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UpdatePostFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	postID := c.Param("postId")
	isAdmin := c.GetHeader("X-User-Role") == "ADMIN"
	err := h.PostSvc.DeletePost(c.Request.Context(), postID, userID, isAdmin)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("DeletePost failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.DeletePostFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) GetFilesInPostsOfUser(c *gin.Context) {
	username := c.Param("username")
	userID := getCurrentUser(c)
	if username == "" {
		exception.SendError(c, exception.UsernameRequired)
		return
	}

	files, err := h.PostSvc.GetFilesInPostsOfUser(c.Request.Context(), username, userID, getPageable(c))
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetFilesInPostsOfUser failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.FailToGetPost)
		return
	}

	sendSuccess(c, files)
}
