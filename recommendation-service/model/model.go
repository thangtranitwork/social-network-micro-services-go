package model

type RecommendedUserCandidate struct {
	ID                string `json:"id"`
	UserID            string `json:"userId"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
	Avatar            string `json:"avatar"`
	MutualFriendsCount int   `json:"mutualFriendsCount"`
}

type RecommendedPostCandidate struct {
	PostID   string  `json:"postId"`
	AuthorID string  `json:"authorId"`
	Score    float64 `json:"score,omitempty"`
}

type Response struct {
	Code    int         `json:"code,omitempty"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Body    interface{} `json:"body,omitempty"`
	Message string      `json:"message,omitempty"`
}
