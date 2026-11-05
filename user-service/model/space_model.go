package model

import "time"

const (
	MaxSpatialNodes        = 60
	MinCanvasCoord         = -3000.0
	MaxCanvasCoord         = 3000.0
	MinNodeScale           = 0.5
	MaxNodeScale           = 2.5
	MinNodeRotation        = -180.0
	MaxNodeRotation        = 180.0
	MaxGuestbookMessageLen = 280
	MaxGuestbookEntries    = 50
)

// SpatialNode represents a single interactive artifact placed on the user's spatial canvas
type SpatialNode struct {
	ID         string                 `json:"id"`
	Type       string                 `json:"type"`
	X          float64                `json:"x"`
	Y          float64                `json:"y"`
	Scale      float64                `json:"scale"`
	Rotation   float64                `json:"rotation"`
	ZIndex     int                    `json:"zIndex"`
	FrameStyle string                 `json:"frameStyle"`
	Content    map[string]interface{} `json:"content"`
}

// SpaceGuestbookEntry represents a guestbook note in the domain layer
type SpaceGuestbookEntry struct {
	ID               string    `json:"id"`
	SpaceOwnerID     string    `json:"spaceOwnerId"`
	AuthorID         string    `json:"authorId"`
	AuthorUsername   string    `json:"authorUsername"`
	AuthorGivenName  string    `json:"authorGivenName"`
	AuthorFamilyName string    `json:"authorFamilyName"`
	AuthorAvatarURL  string    `json:"authorAvatarUrl"`
	Message          string    `json:"message"`
	CardColor        string    `json:"cardColor"`
	PosX             float64   `json:"posX"`
	PosY             float64   `json:"posY"`
	CreatedAt        time.Time `json:"createdAt"`
}

// UserSpaceConfig represents the full spatial profile configuration and guestbook entries
type UserSpaceConfig struct {
	UserID           string                `json:"userId"`
	IsConfigured     bool                  `json:"isConfigured"`
	Theme            string                `json:"theme"`
	DefaultViewMode  string                `json:"defaultViewMode"`
	CustomAccent     string                `json:"customAccent"`
	AllowGuestbook   bool                  `json:"allowGuestbook"`
	AutoplayNodeID   string                `json:"autoplayNodeId"`
	Nodes            []SpatialNode         `json:"nodes"`
	GuestbookEntries []SpaceGuestbookEntry `json:"guestbookEntries"`
	UpdatedAt        time.Time             `json:"updatedAt"`
}

// UpdateSpaceConfigRequest is the strict DTO for updating the spatial profile layout
type UpdateSpaceConfigRequest struct {
	Theme           string        `json:"theme" binding:"required"`
	DefaultViewMode string        `json:"defaultViewMode" binding:"required"`
	CustomAccent    string        `json:"customAccent" binding:"required"`
	AllowGuestbook  *bool         `json:"allowGuestbook" binding:"required"`
	AutoplayNodeID  string        `json:"autoplayNodeId"`
	Nodes           []SpatialNode `json:"nodes" binding:"required"`
}

// CreateSpaceGuestbookRequest is the strict DTO for adding a visitor guestbook note
type CreateSpaceGuestbookRequest struct {
	Message   string  `json:"message" binding:"required"`
	CardColor string  `json:"cardColor" binding:"required"`
	PosX      float64 `json:"posX"`
	PosY      float64 `json:"posY"`
}
