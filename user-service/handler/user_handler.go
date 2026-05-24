package handler

import (
	"net/http"
	"time"

	"social-network-go/exception"
	"social-network-go/user-service/model"
	"social-network-go/user-service/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	UserSvc *service.UserService
}

func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{UserSvc: userSvc}
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
	case "USER_NOT_FOUND":
		mapped = exception.UserNotFound
		found = true
	case "GIVEN_NAME_REQUIRED":
		mapped = exception.GivenNameRequired
		found = true
	case "FAMILY_NAME_REQUIRED":
		mapped = exception.FamilyNameRequired
		found = true
	case "BIRTHDATE_REQUIRED":
		mapped = exception.BirthdateRequired
		found = true
	case "AGE_MUST_BE_AT_LEAST_16":
		mapped = exception.AgeMustBeAtLeast16
		found = true
	case "INVALID_GIVEN_NAME_LENGTH":
		mapped = exception.InvalidGivenNameLength
		found = true
	case "INVALID_FAMILY_NAME_LENGTH":
		mapped = exception.InvalidFamilyNameLength
		found = true
	case "LESS_THAN_30_DAYS_SINCE_LAST_BIRTHDATE_CHANGE":
		mapped = exception.LessThan30DaysSinceLastBirthdateChange
		found = true
	case "LESS_THAN_30_DAYS_SINCE_LAST_NAME_CHANGE":
		mapped = exception.LessThan30DaysSinceLastNameChange
		found = true
	case "LESS_THAN_30_DAYS_SINCE_LAST_USERNAME_CHANGE":
		mapped = exception.LessThan30DaysSinceLastUsernameChange
		found = true
	case "USERNAME_REQUIRED":
		mapped = exception.UsernameRequired
		found = true
	case "INVALID_USERNAME":
		mapped = exception.InvalidUsername
		found = true
	case "USERNAME_ALREADY_EXISTS":
		mapped = exception.UsernameAlreadyExists
		found = true
	case "PROFILE_PICTURE_REQUIRED":
		mapped = exception.ProfilePictureRequired
		found = true
	case "NOTHING_CHANGED":
		mapped = exception.NothingChanged
		found = true
	case "CAN_NOT_MAKE_SELF_REQUEST":
		mapped = exception.CanNotMakeSelfRequest
		found = true
	case "SENT_ADD_FRIEND_REQUEST_FAILED":
		mapped = exception.SentAddFriendRequestFailed
		found = true
	case "ADD_FRIEND_REQUEST_SENT_LIMIT_REACHED":
		mapped = exception.AddFriendRequestSentLimitReached
		found = true
	case "ADD_FRIEND_REQUEST_RECEIVED_LIMIT_REACHED":
		mapped = exception.AddFriendRequestReceivedLimitReached
		found = true
	case "REQUEST_NOT_FOUND":
		mapped = exception.RequestNotFound
		found = true
	case "ACCEPT_REQUEST_FAILED":
		mapped = exception.AcceptRequestFailed
		found = true
	case "HAS_BLOCKED":
		mapped = exception.HasBlocked
		found = true
	case "HAS_BEEN_BLOCKED":
		mapped = exception.HasBeenBlocked
		found = true
	case "NOT_BLOCK":
		mapped = exception.NotBlock
		found = true
	case "BLOCK_LIMIT_REACHED":
		mapped = exception.BlockLimitReached
		found = true
	case "CAN_NOT_BLOCK_YOURSELF":
		mapped = exception.CanNotBlockYourself
		found = true
	case "BLOCK_NOT_FOUND":
		mapped = exception.BlockNotFound
		found = true
	case "FRIEND_NOT_FOUND":
		mapped = exception.FriendNotFound
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

func mapUserToProfileResponse(u *model.User) model.UserProfileResponse {
	return model.UserProfileResponse{
		ID:                      u.ID,
		GivenName:               u.GivenName,
		FamilyName:              u.FamilyName,
		Username:                u.Username,
		Email:                   u.Email,
		Bio:                     u.Bio,
		Birthdate:               u.Birthdate,
		ProfilePictureUrl:       u.ProfilePictureId,
		FriendCount:             u.FriendCount,
		BlockCount:              u.BlockCount,
		RequestSentCount:        u.RequestSentCount,
		RequestReceivedCount:    u.RequestReceivedCount,
		NextChangeNameDate:      u.NextChangeNameDate,
		NextChangeBirthdateDate: u.NextChangeBirthdateDate,
		NextChangeUsernameDate:  u.NextChangeUsernameDate,
		CreatedAt:               u.CreatedAt,
	}
}

func mapUsersToCommonInfo(users []*model.User) []model.UserCommonInformationResponse {
	res := make([]model.UserCommonInformationResponse, len(users))
	for i, u := range users {
		res[i] = model.UserCommonInformationResponse{
			ID:                u.ID,
			Username:          u.Username,
			GivenName:         u.GivenName,
			FamilyName:        u.FamilyName,
			ProfilePictureUrl: u.ProfilePictureId,
		}
	}
	return res
}

func (h *UserHandler) GetUserProfile(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		sendError(c, http.StatusBadRequest, 400, "USERNAME_REQUIRED")
		return
	}

	currentUserID := getCurrentUser(c)
	email := c.GetHeader("X-User-Email")
	if currentUserID != "" && email != "" {
		_, _ = h.UserSvc.EnsureProfile(c.Request.Context(), currentUserID, email)
	}

	profile, err := h.UserSvc.GetUserProfile(c.Request.Context(), username, currentUserID)
	if err != nil {
		sendError(c, http.StatusNotFound, 404, err.Error())
		return
	}

	sendSuccess(c, mapUserToProfileResponse(profile))
}

func (h *UserHandler) GetFriends(c *gin.Context) {
	username := c.Param("username")
	if username == "" {
		sendError(c, http.StatusBadRequest, 400, "USERNAME_REQUIRED")
		return
	}

	currentUserID := getCurrentUser(c)
	friends, err := h.UserSvc.GetFriends(c.Request.Context(), username, currentUserID)
	if err != nil {
		sendError(c, http.StatusNotFound, 404, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(friends))
}

func (h *UserHandler) GetSuggestedFriends(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	suggestions, err := h.UserSvc.GetSuggestedFriends(c.Request.Context(), currentUserID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(suggestions))
}

func (h *UserHandler) GetMutualFriends(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	mutual, err := h.UserSvc.GetMutualFriends(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(mutual))
}

func (h *UserHandler) Unfriend(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Unfriend(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) Block(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Block(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) Unblock(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.Unblock(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) GetBlockedUsers(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	blocked, err := h.UserSvc.GetBlockedUsers(c.Request.Context(), currentUserID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(blocked))
}

func (h *UserHandler) SendFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.SendFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) AcceptFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.AcceptFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) DeleteFriendRequest(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	username := c.Param("username")
	err := h.UserSvc.DeleteFriendRequest(c.Request.Context(), currentUserID, username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) GetSentRequests(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	requests, err := h.UserSvc.GetSentRequests(c.Request.Context(), currentUserID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(requests))
}

func (h *UserHandler) GetReceivedRequests(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	requests, err := h.UserSvc.GetReceivedRequests(c.Request.Context(), currentUserID)
	if err != nil {
		sendError(c, http.StatusInternalServerError, 500, err.Error())
		return
	}

	sendSuccess(c, mapUsersToCommonInfo(requests))
}

func (h *UserHandler) UpdateBio(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req model.UpdateBioRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	err := h.UserSvc.UpdateBio(c.Request.Context(), currentUserID, req.Bio)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateBirthdate(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req model.UpdateBirthdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "BIRTHDATE_REQUIRED")
		return
	}

	err := h.UserSvc.UpdateBirthdate(c.Request.Context(), currentUserID, req.Birthdate)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, req.Birthdate)
}

func (h *UserHandler) UpdateName(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req model.UpdateNameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "INVALID_REQUEST_BODY")
		return
	}

	err := h.UserSvc.UpdateName(c.Request.Context(), currentUserID, req.FamilyName, req.GivenName)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateUsername(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req model.UpdateUsernameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "USERNAME_REQUIRED")
		return
	}

	err := h.UserSvc.UpdateUsername(c.Request.Context(), currentUserID, req.Username)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, nil)
}

func (h *UserHandler) UpdateProfilePicture(c *gin.Context) {
	currentUserID := getCurrentUser(c)
	if currentUserID == "" {
		sendError(c, http.StatusUnauthorized, 401, "UNAUTHORIZED")
		return
	}

	var req struct {
		FileId string `json:"fileId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		sendError(c, http.StatusBadRequest, 400, "FILE_ID_REQUIRED")
		return
	}

	path, err := h.UserSvc.UpdateProfilePicture(c.Request.Context(), currentUserID, req.FileId)
	if err != nil {
		sendError(c, http.StatusBadRequest, 400, err.Error())
		return
	}

	sendSuccess(c, path)
}
