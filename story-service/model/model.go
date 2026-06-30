package model

import "time"

type AuthorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type Story struct {
	ID        string    `json:"id"`
	MediaUrl  string    `json:"mediaUrl"`
	MediaType string    `json:"mediaType"` // IMAGE or VIDEO
	CreatedAt time.Time `json:"createdAt"`
	AuthorID  string    `json:"authorId"`
}

type UserStories struct {
	User    AuthorInfo `json:"user"`
	Stories []*Story   `json:"stories"`
}
