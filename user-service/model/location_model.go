package model

import "time"

// UserLocationSettingsEntity persists a user's opt-in preferences for People Nearby and personalization.
type UserLocationSettingsEntity struct {
	UserID                  string    `gorm:"primaryKey;size:128" json:"userId"`
	DiscoverableNearby      bool      `gorm:"not null;default:false" json:"discoverableNearby"`
	LocationPersonalization bool      `gorm:"not null;default:true" json:"locationPersonalization"`
	LastCountry             string    `gorm:"size:64" json:"lastCountry"`
	LastCity                string    `gorm:"size:128" json:"lastCity"`
	UpdatedAt               time.Time `json:"updatedAt"`
}

func (UserLocationSettingsEntity) TableName() string {
	return "user_location_settings"
}

// UserDeviceSessionEntity records active or recent client devices and IPs per user.
type UserDeviceSessionEntity struct {
	ID           string    `gorm:"primaryKey;size:128" json:"id"`
	UserID       string    `gorm:"size:128;not null;index" json:"userId"`
	DeviceID     string    `gorm:"size:128;not null;index" json:"deviceId"`
	Platform     string    `gorm:"size:32" json:"platform"`
	DeviceModel  string    `gorm:"size:128" json:"deviceModel"`
	OSVersion    string    `gorm:"size:64" json:"osVersion"`
	AppVersion   string    `gorm:"size:32" json:"appVersion"`
	IPAddress    string    `gorm:"size:64" json:"ipAddress"`
	UserAgent    string    `gorm:"type:text" json:"userAgent"`
	LastActiveAt time.Time `gorm:"index" json:"lastActiveAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (UserDeviceSessionEntity) TableName() string {
	return "user_device_sessions"
}

type LocationUpdateRequest struct {
	Lat       float64 `json:"lat" binding:"required"`
	Lng       float64 `json:"lng" binding:"required"`
	Accuracy  float64 `json:"accuracy"`
	Source    string  `json:"source"`
	City      string  `json:"city"`
	Country   string  `json:"country"`
	EventType string  `json:"eventType"`
}

type LocationSettingsUpdateRequest struct {
	DiscoverableNearby      *bool `json:"discoverableNearby"`
	LocationPersonalization *bool `json:"locationPersonalization"`
}

type ClientTelemetryMeta struct {
	IPAddress   string `json:"ipAddress"`
	DeviceID    string `json:"deviceId"`
	Platform    string `json:"platform"`
	DeviceModel string `json:"deviceModel"`
	OSVersion   string `json:"osVersion"`
	AppVersion  string `json:"appVersion"`
	UserAgent   string `json:"userAgent"`
}

type TelemetryEvent struct {
	EventID        string    `json:"eventId"`
	UserID         string    `json:"userId"`
	EventType      string    `json:"eventType"`
	Lat            float64   `json:"lat"`
	Lng            float64   `json:"lng"`
	Accuracy       float64   `json:"accuracy"`
	LocationSource string    `json:"locationSource"`
	IPAddress      string    `json:"ipAddress"`
	GeoCountry     string    `json:"geoCountry"`
	GeoCity        string    `json:"geoCity"`
	DeviceID       string    `json:"deviceId"`
	Platform       string    `json:"platform"`
	DeviceModel    string    `json:"deviceModel"`
	OSVersion      string    `json:"osVersion"`
	AppVersion     string    `json:"appVersion"`
	UserAgent      string    `json:"userAgent"`
	CreatedAt      time.Time `json:"createdAt"`
}
