package model

type RecommendedUserCandidate struct {
	UserID            string `json:"userId"`
	Username          string `json:"username"`
	MutualFriendsCount int   `json:"mutualFriendsCount"`
}

type RecommendedPostCandidate struct {
	PostID   string  `json:"postId"`
	AuthorID string  `json:"authorId"`
	Score    float64 `json:"score,omitempty"`
}

type Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Message string      `json:"message,omitempty"`
}
