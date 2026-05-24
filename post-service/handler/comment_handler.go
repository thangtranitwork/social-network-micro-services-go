package handler

import (
	"social-network-go/exception"
	"social-network-go/logger"

	"github.com/gin-gonic/gin"
)

type CommentRequest struct {
	PostID  string  `json:"postId" binding:"required"`
	Content string  `json:"content"`
	FileID  *string `json:"fileId"`
}

type ReplyCommentRequest struct {
	OriginalCommentID string  `json:"originalCommentId" binding:"required"`
	Content           string  `json:"content" binding:"required"`
	FileID            *string `json:"fileId"`
}

func (h *PostHandler) Comment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	comment, err := h.PostSvc.Comment(c.Request.Context(), userID, req.PostID, req.Content, req.FileID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("Comment failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.CommentFailed)
		return
	}

	sendSuccess(c, comment)
}

func (h *PostHandler) ReplyComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	var req ReplyCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		exception.SendError(c, exception.InvalidInput)
		return
	}

	comment, err := h.PostSvc.ReplyComment(c.Request.Context(), userID, req.OriginalCommentID, req.Content, req.FileID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("ReplyComment failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.ReplyCommentFailed)
		return
	}

	sendSuccess(c, comment)
}

func (h *PostHandler) LikeComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	commentID := c.Param("commentId")
	err := h.PostSvc.LikeComment(c.Request.Context(), userID, commentID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("LikeComment failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.LikeCommentFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UnlikeComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	commentID := c.Param("commentId")
	err := h.PostSvc.UnlikeComment(c.Request.Context(), userID, commentID)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("UnlikeComment failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.UnlikeCommentFailed)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) GetComments(c *gin.Context) {
	postID := c.Param("postId")
	userID := getCurrentUser(c)

	comments, err := h.PostSvc.GetComments(c.Request.Context(), postID, userID, getPageable(c))
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetComments failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetCommentsFailed)
		return
	}

	sendSuccess(c, comments)
}

func (h *PostHandler) GetRepliedComments(c *gin.Context) {
	commentID := c.Param("commentId")
	userID := getCurrentUser(c)

	comments, err := h.PostSvc.GetRepliedComments(c.Request.Context(), commentID, userID, getPageable(c))
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("GetRepliedComments failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.GetCommentsFailed)
		return
	}

	sendSuccess(c, comments)
}

func (h *PostHandler) DeleteComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		exception.SendError(c, exception.Unauthorized)
		return
	}

	commentID := c.Param("commentId")
	isAdmin := c.GetHeader("X-User-Role") == "ADMIN"
	err := h.PostSvc.DeleteComment(c.Request.Context(), userID, commentID, isAdmin)
	if err != nil {
		logger.WithContext(c.Request.Context()).Err(err).Error("DeleteComment failed")
		if mapped, found := exception.MapAppError(err); found {
			exception.SendError(c, mapped)
			return
		}
		exception.SendError(c, exception.DeleteCommentFailed)
		return
	}

	sendSuccess(c, nil)
}
