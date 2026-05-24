package model

import "time"

type AuthorInfo struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	GivenName         string `json:"givenName"`
	FamilyName        string `json:"familyName"`
	ProfilePictureUrl string `json:"profilePictureUrl"`
}

type Post struct {
	ID                  string      `json:"id"`
	Content             string      `json:"content"`
	AuthorID            string      `json:"authorId"`
	CreatedAt           time.Time   `json:"createdAt"`
	UpdatedAt           *time.Time  `json:"updatedAt,omitempty"`
	Privacy             string      `json:"privacy"`
	LikeCount           int         `json:"likeCount"`
	CommentCount        int         `json:"commentCount"`
	ShareCount          int         `json:"shareCount"`
	Liked               bool        `json:"isLiked"`
	Author              AuthorInfo  `json:"author"`
	Files               []string    `json:"files"`
	Images              []string    `json:"images"` // Alias for files to match some Java/Frontend parts
	SharedPost          bool        `json:"sharedPost"`
	OriginalPostID      string      `json:"originalPostId,omitempty"`
	OriginalAuthorID    string      `json:"originalAuthorId,omitempty"`
	OriginalPostCanView bool        `json:"originalPostCanView"`
	OriginalPost        *Post       `json:"originalPost,omitempty"`
}

type Comment struct {
	ID                string      `json:"id"`
	PostID            string      `json:"postId"`
	AuthorID          string      `json:"authorId"`
	Content           string      `json:"content"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         *time.Time  `json:"updatedAt,omitempty"`
	Author            AuthorInfo  `json:"author"`
	LikeCount         int         `json:"likeCount"`
	ReplyCount        int         `json:"replyCount"`
	Liked             bool        `json:"isLiked"`
	OriginalCommentID string      `json:"originalCommentId,omitempty"`
	Files             []string    `json:"files"`
	FileUrl           string      `json:"fileUrl"` // Single file URL for Java compatibility
}
