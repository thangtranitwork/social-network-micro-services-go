package dto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"social-network-go/post-service/model"
)

type ViewHistoryRecord struct {
	EventID           string    `json:"eventId"`
	PostID            string    `json:"postId"`
	ObservedAt        time.Time `json:"observedAt"`
	VisibleDurationMs int       `json:"visibleDurationMs"`
	VisibleRatio      float64   `json:"visibleRatio"`
	Surface           string    `json:"surface"`
	SchemaVersion     int       `json:"schemaVersion"`
}

type ViewHistoryItem struct {
	EventID           string      `json:"eventId"`
	PostID            string      `json:"postId"`
	ViewedAt          time.Time   `json:"viewedAt"`
	VisibleDurationMs int         `json:"visibleDurationMs"`
	VisibleRatio      float64     `json:"visibleRatio"`
	Surface           string      `json:"surface"`
	Post              *model.Post `json:"post,omitempty"`
}

type ViewHistoryResponse struct {
	Items      []*ViewHistoryItem `json:"items"`
	NextCursor string             `json:"nextCursor,omitempty"`
	HasMore    bool               `json:"hasMore"`
}

type OpaqueHistoryCursor struct {
	Month     string `json:"m"` // YYYY-MM
	PageState []byte `json:"p"` // Cassandra pagination state bytes
}

func EncodeHistoryCursor(month string, pageState []byte) string {
	if month == "" && len(pageState) == 0 {
		return ""
	}
	c := OpaqueHistoryCursor{
		Month:     month,
		PageState: pageState,
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(raw)
}

func DecodeHistoryCursor(cursorStr string) (string, []byte, error) {
	if cursorStr == "" {
		return "", nil, nil
	}
	raw, err := base64.URLEncoding.DecodeString(cursorStr)
	if err != nil {
		return "", nil, errors.New("invalid cursor encoding")
	}
	var c OpaqueHistoryCursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return "", nil, errors.New("malformed cursor payload")
	}
	return c.Month, c.PageState, nil
}
