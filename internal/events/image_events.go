package events

import "time"

const (
	TopicPostImageEvents = "post-image-events"

	EventTypePostImageEmbedded = "POST_IMAGE_EMBEDDED"
	EventTypePostImageDeleted  = "POST_IMAGE_DELETED"
	EventTypePostImagesCleared = "POST_IMAGES_CLEARED"
)

type PostImageEmbeddedEvent struct {
	EventType      string    `json:"eventType"`
	PostID         string    `json:"postId"`
	FileID         string    `json:"fileId,omitempty"`
	AuthorID       string    `json:"authorId,omitempty"`
	Privacy        string    `json:"privacy,omitempty"`
	EmbeddingModel string    `json:"embeddingModel,omitempty"`
	ModelVersion   string    `json:"modelVersion,omitempty"`
	Vector         []float32 `json:"vector,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}
