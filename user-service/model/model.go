package model

import "time"

const (
	MaxFriendCount           = 100
	MaxBlockCount            = 100
	ChangeNameCooldownDay    = 30
	ChangeUsernameCooldownDay = 30
	ChangeBirthdateCooldownDay = 30
	MaxGivenNameLength       = 64
	MaxFamilyNameLength      = 64
	MaxUsernameLength        = 32
	MinAge                   = 16
	MaxSentRequestCount      = 100
	MaxReceivedRequestCount  = 100
)

type User struct {
	ID                      string    `json:"id"`
	GivenName               string    `json:"givenName"`
	FamilyName              string    `json:"familyName"`
	Username                string    `json:"username"`
	Email                   string    `json:"email"`
	Bio                     string    `json:"bio"`
	Birthdate               time.Time `json:"birthdate"`
	ProfilePictureId        string    `json:"profilePictureId"`
	FriendCount             int       `json:"friendCount"`
	BlockCount              int       `json:"blockCount"`
	RequestSentCount        int       `json:"requestSentCount"`
	RequestReceivedCount    int       `json:"requestReceivedCount"`
	NextChangeNameDate      time.Time `json:"nextChangeNameDate"`
	NextChangeBirthdateDate time.Time `json:"nextChangeBirthdateDate"`
	NextChangeUsernameDate  time.Time `json:"nextChangeUsernameDate"`
	CreatedAt               time.Time `json:"createdAt"`
}

// UserProfileResponse matches Java UserProfileResponse
type UserProfileResponse struct {
	ID                      string    `json:"id"`
	GivenName               string    `json:"givenName"`
	FamilyName              string    `json:"familyName"`
	Username                string    `json:"username"`
	Email                   string    `json:"email"`
	Bio                     string    `json:"bio"`
	Birthdate               time.Time `json:"birthdate"`
	ProfilePictureUrl       string    `json:"profilePictureUrl"`
	FriendCount             int       `json:"friendCount"`
	BlockCount              int       `json:"blockCount"`
	RequestSentCount        int       `json:"requestSentCount"`
	RequestReceivedCount    int       `json:"requestReceivedCount"`
	NextChangeNameDate      time.Time `json:"nextChangeNameDate"`
	NextChangeBirthdateDate time.Time `json:"nextChangeBirthdateDate"`
	NextChangeUsernameDate  time.Time `json:"nextChangeUsernameDate"`
	CreatedAt               time.Time `json:"createdAt"`
}

// UserCommonInformationResponse matches Java UserCommonInformationResponse
type UserCommonInformationResponse struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

// UpdateBioRequest
type UpdateBioRequest struct {
	Bio string `json:"bio"`
}

// UpdateNameRequest
type UpdateNameRequest struct {
	GivenName  string `json:"givenName" binding:"required,max=64"`
	FamilyName string `json:"familyName" binding:"required,max=64"`
}

// UpdateUsernameRequest
type UpdateUsernameRequest struct {
	Username string `json:"username" binding:"required,max=32"`
}

// UpdateBirthdateRequest
type UpdateBirthdateRequest struct {
	Birthdate string `json:"birthdate" binding:"required"` // Format: YYYY-MM-DD
}
