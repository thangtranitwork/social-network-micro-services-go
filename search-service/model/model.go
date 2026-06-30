package model

import "time"

type AuthorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type User struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
	Email             string `json:"email"`
	Bio               string `json:"bio"`
	IsOnline          bool   `json:"isOnline"`
	LastOnline        string `json:"lastOnline"`
}

type Post struct {
	ID           string     `json:"id"`
	Content      string     `json:"content"`
	AuthorID     string     `json:"authorId"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    *time.Time `json:"updatedAt,omitempty"`
	Privacy      string     `json:"privacy"`
	LikeCount    int        `json:"likeCount"`
	CommentCount int        `json:"commentCount"`
	ShareCount   int        `json:"shareCount"`
	Liked        bool       `json:"liked"`
	Author       AuthorInfo `json:"author"`
	Files        []string   `json:"files"`
	Images       []string   `json:"images"`
}

type SearchResults struct {
	USER []*User `json:"USER"`
	POST []*Post `json:"POST"`
}
