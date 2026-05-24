package handler

import (
	"net/http"
	"time"

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
