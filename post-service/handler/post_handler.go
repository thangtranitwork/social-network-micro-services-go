package handler

import (
	"fmt"
	"net/http"
	"time"

	"social-network-go/post-service/service"
	"social-network-go/exception"

	"github.com/gin-gonic/gin"
)

type PostHandler struct {
	PostSvc *service.PostService
}

func NewPostHandler(postSvc *service.PostService) *PostHandler {
	return &PostHandler{PostSvc: postSvc}
}

type ApiResponse struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Timestamp string      `json:"timestamp"`
	Body      interface{} `json:"body,omitempty"`
}

func sendSuccess(c *gin.Context, body interface{}) {
	c.JSON(http.StatusOK, ApiResponse{
		Code:      200,
		Message:   "OK",
		Timestamp: time.Now().Format(time.RFC3339),
		Body:      body,
	})
}

func sendError(c *gin.Context, httpStatus int, code int, msg string) {
	// Look up mapped ErrorCode in exception package
	var mapped exception.ErrorCode
	var found bool

	// Match by msg
	switch msg {
	case "POST_CONTENT_AND_ATTACH_FILES_BOTH_EMPTY":
		mapped = exception.PostContentAndAttachFilesBothEmpty
		found = true
	case "INVALID_POST_CONTENT_LENGTH":
		mapped = exception.InvalidPostContentLength
		found = true
	case "INVALID_NUMBER_OF_POST_ATTACHMENTS":
		mapped = exception.InvalidNumberOfPostAttachments
		found = true
	case "POST_NOT_FOUND":
		mapped = exception.PostNotFound
		found = true
	case "ONLY_PUBLIC_POST_CAN_BE_SHARED":
		mapped = exception.OnlyPublicPostCanBeShared
		found = true
	case "PRIVACY_UNCHANGED":
		mapped = exception.PrivacyUnchanged
		found = true
	case "INVALID_DELETE_ATTACHMENT":
		mapped = exception.InvalidDeleteAttachment
		found = true
	case "POST_CONTENT_UNCHANGED":
		mapped = exception.PostContentUnchanged
		found = true
	case "LIKED_POST":
		mapped = exception.LikedPost
		found = true
	case "NOT_LIKED_POST":
		mapped = exception.NotLikedPost
		found = true
	case "DELETED_POST":
		mapped = exception.DeletedPost
		found = true
	case "COMMENT_NOT_FOUND":
		mapped = exception.CommentNotFound
		found = true
	case "COMMENT_CONTENT_AND_ATTACH_FILE_BOTH_EMPTY":
		mapped = exception.CommentContentAndAttachFileBothEmpty
		found = true
	case "INVALID_COMMENT_CONTENT_LENGTH":
		mapped = exception.InvalidCommentContentLength
		found = true
	case "POST_ID_REQUIRED":
		mapped = exception.PostIdRequired
		found = true
	case "ORIGINAL_COMMENT_ID_REQUIRED":
		mapped = exception.OriginalCommentIdRequired
		found = true
	case "LIKED_COMMENT":
		mapped = exception.LikedComment
		found = true
	case "NOT_LIKED_COMMENT":
		mapped = exception.NotLikedComment
		found = true
	case "CAN_NOT_REPLY_REPLIED_COMMENT":
		mapped = exception.CanNotReplyRepliedComment
		found = true
	case "COMMENT_CONTENT_UNCHANGED":
		mapped = exception.CommentContentUnchanged
		found = true
	case "USERNAME_REQUIRED":
		mapped = exception.UsernameRequired
		found = true
	case "UNAUTHORIZED":
		mapped = exception.Unauthorized
		found = true
	case "INVALID_REQUEST_PAYLOAD":
		mapped = exception.InvalidInput
		found = true
	}

	if found {
		httpStatus = mapped.Status
		code = mapped.Code
		msg = mapped.Message
	}

	c.JSON(httpStatus, ApiResponse{
		Code:      code,
		Message:   msg,
		Timestamp: time.Now().Format(time.RFC3339),
	})
}

func sendAppError(c *gin.Context, err error) {
	if appErr, ok := err.(exception.AppException); ok {
		c.JSON(appErr.ErrCode.Status, ApiResponse{
			Code:      appErr.ErrCode.Code,
			Message:   appErr.ErrCode.Message,
			Timestamp: time.Now().Format(time.RFC3339),
		})
		return
	}
	// Fallback to sendError mapping
	sendError(c, http.StatusBadRequest, 400, err.Error())
}

func getCurrentUser(c *gin.Context) string {
	userID := c.GetHeader("X-User-ID")
	if userID == "" {
		userID = c.Query("current_user_id")
	}
	return userID
}

func getPageable(c *gin.Context) service.Pageable {
	skipStr := c.DefaultQuery("skip", "0")
	limitStr := c.DefaultQuery("limit", "20")
	pageType := c.DefaultQuery("type", "RELEVANT")

	var skip, limit int
	fmt.Sscanf(skipStr, "%d", &skip)
	fmt.Sscanf(limitStr, "%d", &limit)

	return service.Pageable{
		Skip:  int64(skip),
		Limit: int64(limit),
		Type:  pageType,
	}
}

func (h *PostHandler) GetNewsfeed(c *gin.Context) {
	userID := getCurrentUser(c)
	pageable := getPageable(c)

	posts, err := h.PostSvc.GetSuggestedPosts(c.Request.Context(), userID, pageable)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, posts)
}

func (h *PostHandler) GetPostsOfUser(c *gin.Context) {
	username := c.Param("username")
	userID := getCurrentUser(c)
	if username == "" {
		sendError(c, http.StatusBadRequest, 400, "USERNAME_REQUIRED")
		return
	}

	posts, err := h.PostSvc.GetPostsOfUser(c.Request.Context(), username, userID, getPageable(c))
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, posts)
}

func (h *PostHandler) GetPost(c *gin.Context) {
	postID := c.Param("id")
	userID := getCurrentUser(c)

	post, err := h.PostSvc.GetPost(c.Request.Context(), postID, userID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, post)
}

type CreatePostRequest struct {
	Content string   `form:"content" json:"content"`
	Privacy string   `form:"privacy" json:"privacy" binding:"required"`
	Files   []string `form:"files" json:"files"`
}

func (h *PostHandler) CreatePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req CreatePostRequest
	if err := c.ShouldBind(&req); err != nil {
		if errJSON := c.ShouldBindJSON(&req); errJSON != nil {
			sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
			return
		}
	}

	if req.Files == nil {
		req.Files = []string{}
	}

	post, err := h.PostSvc.CreatePost(c.Request.Context(), userID, req.Content, req.Privacy, req.Files)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, post)
}

type SharePostRequest struct {
	OriginalPostID string `json:"originalPostId" binding:"required"`
	Content        string `json:"content"`
	Privacy        string `json:"privacy"`
}

func (h *PostHandler) SharePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req SharePostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	post, err := h.PostSvc.SharePost(c.Request.Context(), userID, req.OriginalPostID, req.Content, req.Privacy)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, post)
}

func (h *PostHandler) LikePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	postID := c.Param("postId")
	err := h.PostSvc.LikePost(c.Request.Context(), userID, postID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UnlikePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	postID := c.Param("postId")
	err := h.PostSvc.UnlikePost(c.Request.Context(), userID, postID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UpdatePrivacy(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	postID := c.Param("postId")
	privacy := c.Query("privacy")
	if privacy == "" {
		sendError(c, http.StatusBadRequest, 400, "PRIVACY_REQUIRED")
		return
	}

	err := h.PostSvc.UpdatePrivacy(c.Request.Context(), userID, postID, privacy)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

type UpdateContentRequest struct {
	Content          *string  `json:"content"`
	NewFiles         []string `json:"newFiles"`
	DeleteOldFileIDs []string `json:"deleteOldFileIds"`
}

func (h *PostHandler) UpdateContent(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	postID := c.Param("postId")
	var req UpdateContentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	err := h.PostSvc.UpdateContent(c.Request.Context(), userID, postID, req.Content, req.NewFiles, req.DeleteOldFileIDs)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) DeletePost(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	postID := c.Param("postId")
	isAdmin := c.GetHeader("X-User-Role") == "ADMIN"
	err := h.PostSvc.DeletePost(c.Request.Context(), postID, userID, isAdmin)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) GetFilesInPostsOfUser(c *gin.Context) {
	username := c.Param("username")
	userID := getCurrentUser(c)
	if username == "" {
		sendError(c, http.StatusBadRequest, 400, "USERNAME_REQUIRED")
		return
	}

	files, err := h.PostSvc.GetFilesInPostsOfUser(c.Request.Context(), username, userID, getPageable(c))
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, files)
}

type CommentRequest struct {
	PostID  string  `json:"postId" binding:"required"`
	Content string  `json:"content"`
	FileID  *string `json:"fileId"`
}

func (h *PostHandler) Comment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	comment, err := h.PostSvc.Comment(c.Request.Context(), userID, req.PostID, req.Content, req.FileID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, comment)
}

type ReplyCommentRequest struct {
	OriginalCommentID string  `json:"originalCommentId" binding:"required"`
	Content           string  `json:"content" binding:"required"`
	FileID            *string `json:"fileId"`
}

func (h *PostHandler) ReplyComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req ReplyCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_PAYLOAD")
		return
	}

	comment, err := h.PostSvc.ReplyComment(c.Request.Context(), userID, req.OriginalCommentID, req.Content, req.FileID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, comment)
}

func (h *PostHandler) LikeComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	commentID := c.Param("commentId")
	err := h.PostSvc.LikeComment(c.Request.Context(), userID, commentID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) UnlikeComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	commentID := c.Param("commentId")
	err := h.PostSvc.UnlikeComment(c.Request.Context(), userID, commentID)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}

func (h *PostHandler) GetComments(c *gin.Context) {
	postID := c.Param("postId")
	userID := getCurrentUser(c)
	
	comments, err := h.PostSvc.GetComments(c.Request.Context(), postID, userID, getPageable(c))
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, comments)
}

func (h *PostHandler) GetRepliedComments(c *gin.Context) {
	commentID := c.Param("commentId")
	userID := getCurrentUser(c)

	comments, err := h.PostSvc.GetRepliedComments(c.Request.Context(), commentID, userID, getPageable(c))
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, comments)
}

func (h *PostHandler) DeleteComment(c *gin.Context) {
	userID := getCurrentUser(c)
	if userID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	commentID := c.Param("commentId")
	isAdmin := c.GetHeader("X-User-Role") == "ADMIN"
	err := h.PostSvc.DeleteComment(c.Request.Context(), userID, commentID, isAdmin)
	if err != nil {
		sendAppError(c, err)
		return
	}

	sendSuccess(c, nil)
}
