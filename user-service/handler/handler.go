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

func mapUserToProfileResponse(u *model.User, isSelf bool) model.UserProfileResponse {
	resp := model.UserProfileResponse{
		ID:                 u.ID,
		GivenName:          u.GivenName,
		FamilyName:         u.FamilyName,
		Username:           u.Username,
		Bio:                u.Bio,
		ProfilePictureUrl:  u.ProfilePictureId,
		FriendCount:        u.FriendCount,
		CreatedAt:          u.CreatedAt,
		IsFriend:           u.IsFriend,
		MutualFriendsCount: u.MutualFriendsCount,
		Request:            u.Request,
		BlockStatus:        u.BlockStatus,
		PostCount:          u.PostCount,
	}

	if isSelf {
		resp.Email = u.Email
		if !u.Birthdate.IsZero() && u.Birthdate.Year() > 1900 {
			t := u.Birthdate
			resp.Birthdate = &t
		}
		en := u.EmailNotifications
		resp.EmailNotifications = &en
		pn := u.PushNotifications
		resp.PushNotifications = &pn
		resp.DigestFrequency = u.DigestFrequency
		bc := u.BlockCount
		resp.BlockCount = &bc
		rsc := u.RequestSentCount
		resp.RequestSentCount = &rsc
		rrc := u.RequestReceivedCount
		resp.RequestReceivedCount = &rrc
		if !u.NextChangeNameDate.IsZero() && u.NextChangeNameDate.Year() > 1970 {
			t := u.NextChangeNameDate
			resp.NextChangeNameDate = &t
		}
		if !u.NextChangeBirthdateDate.IsZero() && u.NextChangeBirthdateDate.Year() > 1970 {
			t := u.NextChangeBirthdateDate
			resp.NextChangeBirthdateDate = &t
		}
		if !u.NextChangeUsernameDate.IsZero() && u.NextChangeUsernameDate.Year() > 1970 {
			t := u.NextChangeUsernameDate
			resp.NextChangeUsernameDate = &t
		}
	}

	return resp
}

func mapUsersToCommonInfo(users []*model.User) []model.UserCommonInformationResponse {
	res := make([]model.UserCommonInformationResponse, len(users))
	for i, u := range users {
		res[i] = model.UserCommonInformationResponse{
			ID:                 u.ID,
			Username:           u.Username,
			GivenName:          u.GivenName,
			FamilyName:         u.FamilyName,
			ProfilePictureUrl:  u.ProfilePictureId,
			IsFriend:           u.IsFriend,
			MutualFriendsCount: u.MutualFriendsCount,
			Request:            u.Request,
			BlockStatus:        u.BlockStatus,
		}
	}
	return res
}
