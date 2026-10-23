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
	Highlights   []string   `json:"highlights,omitempty"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversationId"`
	SenderID       string    `json:"senderId"`
	Content        string    `json:"content"`
	MessageType    string    `json:"messageType"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Group struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Privacy     string    `json:"privacy"`
	MemberCount int       `json:"memberCount"`
	CreatedAt   time.Time `json:"createdAt"`
}

type PostSearchFacets struct {
	TotalHits int64            `json:"totalHits"`
	Privacy   map[string]int64 `json:"privacy"`
	Authors   map[string]int64 `json:"authors,omitempty"`
}

type SearchResults struct {
	USER    []*User           `json:"USER"`
	POST    []*Post           `json:"POST"`
	MESSAGE []*Message        `json:"MESSAGE,omitempty"`
	GROUP   []*Group          `json:"GROUP,omitempty"`
	Facets  *PostSearchFacets `json:"facets,omitempty"`
}

type PostImageDoc struct {
	ID             string    `json:"id"`
	PostID         string    `json:"post_id"`
	FileID         string    `json:"file_id"`
	AuthorID       string    `json:"author_id"`
	Privacy        string    `json:"privacy"`
	EmbeddingModel string    `json:"embedding_model"`
	ModelVersion   string    `json:"model_version"`
	ImageVector    []float32 `json:"image_vector"`
	CreatedAt      time.Time `json:"created_at"`
}

type ImageSearchResult struct {
	PostID         string    `json:"postId"`
	FileID         string    `json:"fileId"`
	AuthorID       string    `json:"authorId"`
	Privacy        string    `json:"privacy"`
	Score          float64   `json:"score"`          // Elasticsearch _score: (1 + cos) / 2
	RawCosine      float64   `json:"rawCosine"`      // Raw cosine similarity: 2 * score - 1
	RelevanceLabel string    `json:"relevanceLabel"` // "Rất giống", "Tương đồng cao", etc.
	CreatedAt      time.Time `json:"createdAt"`
}

type VisualPostSearchResult struct {
	Post           *Post   `json:"post"`
	MatchedFileID  string  `json:"matchedFileId"`
	RawCosine      float64 `json:"rawCosine"`
	RelevanceLabel string  `json:"relevanceLabel"`
}
