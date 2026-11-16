package model

import (
	"encoding/json"
	"time"
)

const (
	RuntimeConfigTypeInt      = "INT"
	RuntimeConfigTypeBool     = "BOOL"
	RuntimeConfigTypeString   = "STRING"
	RuntimeConfigTypeDuration = "DURATION"
	RuntimeConfigTypeJSON     = "JSON"
)

type RuntimeConfig struct {
	Key            string    `gorm:"primaryKey;size:160" json:"key"`
	Scope          string    `gorm:"size:80;not null;index" json:"scope"`
	Type           string    `gorm:"size:24;not null" json:"type"`
	Value          string    `gorm:"type:text;not null" json:"value"`
	DefaultValue   string    `gorm:"type:text;not null" json:"defaultValue"`
	Description    string    `gorm:"type:text" json:"description"`
	Category       string    `gorm:"size:80;not null;index" json:"category"`
	IsSensitive    bool      `gorm:"not null;default:false" json:"isSensitive"`
	IsEditable     bool      `gorm:"not null;default:true" json:"isEditable"`
	ValidationJSON string    `gorm:"column:validation_json;type:jsonb" json:"-"`
	Version        int64     `gorm:"not null;default:1" json:"version"`
	UpdatedBy      string    `gorm:"size:128" json:"updatedBy"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (RuntimeConfig) TableName() string {
	return "runtime_configs"
}

type RuntimeConfigResponse struct {
	Key          string                 `json:"key"`
	Scope        string                 `json:"scope"`
	Category     string                 `json:"category"`
	Type         string                 `json:"type"`
	Value        string                 `json:"value"`
	DefaultValue string                 `json:"defaultValue"`
	Description  string                 `json:"description"`
	IsSensitive  bool                   `json:"isSensitive"`
	IsEditable   bool                   `json:"isEditable"`
	Validation   map[string]interface{} `json:"validation,omitempty"`
	Version      int64                  `json:"version"`
	UpdatedBy    string                 `json:"updatedBy"`
	UpdatedAt    time.Time              `json:"updatedAt"`
}

func (c RuntimeConfig) ToResponse() RuntimeConfigResponse {
	resp := RuntimeConfigResponse{
		Key:          c.Key,
		Scope:        c.Scope,
		Category:     c.Category,
		Type:         c.Type,
		Value:        c.Value,
		DefaultValue: c.DefaultValue,
		Description:  c.Description,
		IsSensitive:  c.IsSensitive,
		IsEditable:   c.IsEditable,
		Version:      c.Version,
		UpdatedBy:    c.UpdatedBy,
		UpdatedAt:    c.UpdatedAt,
	}
	if c.ValidationJSON != "" {
		var validation map[string]interface{}
		if err := json.Unmarshal([]byte(c.ValidationJSON), &validation); err == nil {
			resp.Validation = validation
		}
	}
	return resp
}

type RuntimeConfigAudit struct {
	ID        string    `gorm:"primaryKey;size:128" json:"id"`
	Key       string    `gorm:"size:160;not null;index" json:"key"`
	OldValue  string    `gorm:"type:text" json:"oldValue"`
	NewValue  string    `gorm:"type:text" json:"newValue"`
	Version   int64     `gorm:"not null;index" json:"version"`
	UpdatedBy string    `gorm:"size:128;index" json:"updatedBy"`
	Reason    string    `gorm:"type:text" json:"reason"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
}

func (RuntimeConfigAudit) TableName() string {
	return "runtime_config_audits"
}

func DefaultRuntimeConfigs() []RuntimeConfig {
	newsfeedWeights := `{"friendRelationship":100,"secondDegreeOrRequested":50,"viewForward":2,"viewBackward":1,"like":2,"comment":3,"share":5,"loadedPenalty":-20}`
	return []RuntimeConfig{
		{Key: "post.max_content_length", Scope: "post-service", Type: RuntimeConfigTypeInt, Value: "5000", DefaultValue: "5000", Description: "Maximum characters allowed in a post", Category: "limits", ValidationJSON: `{"min":1,"max":20000}`, Version: 1, IsEditable: true},
		{Key: "post.max_attach_files", Scope: "post-service", Type: RuntimeConfigTypeInt, Value: "10", DefaultValue: "10", Description: "Maximum number of files attached to one post", Category: "limits", ValidationJSON: `{"min":0,"max":50}`, Version: 1, IsEditable: true},
		{Key: "post.max_comment_content_length", Scope: "post-service", Type: RuntimeConfigTypeInt, Value: "1000", DefaultValue: "1000", Description: "Maximum characters allowed in a comment", Category: "limits", ValidationJSON: `{"min":1,"max":10000}`, Version: 1, IsEditable: true},
		{Key: "post.default_page_limit", Scope: "post-service", Type: RuntimeConfigTypeInt, Value: "20", DefaultValue: "20", Description: "Default post page size", Category: "limits", ValidationJSON: `{"min":1,"max":100}`, Version: 1, IsEditable: true},
		{Key: "post.max_page_limit", Scope: "post-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Maximum post page size", Category: "limits", ValidationJSON: `{"min":1,"max":500}`, Version: 1, IsEditable: true},
		{Key: "user.max_friend_count", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Maximum number of friends per user", Category: "limits", ValidationJSON: `{"min":0,"max":10000}`, Version: 1, IsEditable: true},
		{Key: "user.max_block_count", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Maximum number of blocked users", Category: "limits", ValidationJSON: `{"min":0,"max":10000}`, Version: 1, IsEditable: true},
		{Key: "user.max_sent_request_count", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Maximum number of sent friend requests", Category: "limits", ValidationJSON: `{"min":0,"max":10000}`, Version: 1, IsEditable: true},
		{Key: "user.max_received_request_count", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Maximum number of received friend requests", Category: "limits", ValidationJSON: `{"min":0,"max":10000}`, Version: 1, IsEditable: true},
		{Key: "user.max_given_name_length", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "64", DefaultValue: "64", Description: "Maximum given name length", Category: "limits", ValidationJSON: `{"min":1,"max":256}`, Version: 1, IsEditable: true},
		{Key: "user.max_family_name_length", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "64", DefaultValue: "64", Description: "Maximum family name length", Category: "limits", ValidationJSON: `{"min":1,"max":256}`, Version: 1, IsEditable: true},
		{Key: "user.max_username_length", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "32", DefaultValue: "32", Description: "Maximum username length", Category: "limits", ValidationJSON: `{"min":1,"max":128}`, Version: 1, IsEditable: true},
		{Key: "user.min_age", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "16", DefaultValue: "16", Description: "Minimum allowed user age", Category: "limits", ValidationJSON: `{"min":1,"max":120}`, Version: 1, IsEditable: true},
		{Key: "newsfeed.score_weights", Scope: "post-service", Type: RuntimeConfigTypeJSON, Value: newsfeedWeights, DefaultValue: newsfeedWeights, Description: "Rule-based newsfeed score weights", Category: "newsfeed", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		// Map & Location Runtime Configs
		{Key: "map.provider", Scope: "client", Type: RuntimeConfigTypeString, Value: "carto", DefaultValue: "carto", Description: "Active map rendering provider: google_tiles | google_sdk | carto | osm | disabled", Category: "map", ValidationJSON: `{"enum":["google_tiles","google_sdk","carto","osm","disabled"]}`, Version: 1, IsEditable: true},
		{Key: "map.style", Scope: "client", Type: RuntimeConfigTypeString, Value: "auto", DefaultValue: "auto", Description: "Map visual style: auto | voyager | positron | dark-matter | roadmap | satellite", Category: "map", ValidationJSON: `{"enum":["auto","voyager","positron","dark-matter","roadmap","satellite"]}`, Version: 1, IsEditable: true},
		{Key: "map.google_tiles_url", Scope: "client", Type: RuntimeConfigTypeString, Value: "https://mt{s}.google.com/vt/lyrs=r&x={x}&y={y}&z={z}&hl=vi", DefaultValue: "https://mt{s}.google.com/vt/lyrs=r&x={x}&y={y}&z={z}&hl=vi", Description: "Tile URL template when map.provider is google_tiles (use {s} for mt0..mt3)", Category: "map", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		{Key: "map.google_api_key", Scope: "client", Type: RuntimeConfigTypeString, Value: "", DefaultValue: "", Description: "Google Maps JavaScript API key when map.provider is google_sdk", Category: "map", ValidationJSON: `{}`, Version: 1, IsSensitive: true, IsEditable: true},
		{Key: "map.default_center", Scope: "client", Type: RuntimeConfigTypeJSON, Value: `{"lat":10.7769,"lng":106.7009,"zoom":13}`, DefaultValue: `{"lat":10.7769,"lng":106.7009,"zoom":13}`, Description: "Default map center coordinates and zoom level", Category: "map", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		{Key: "location.nearby_enabled", Scope: "client", Type: RuntimeConfigTypeBool, Value: "true", DefaultValue: "true", Description: "Enable or disable People Nearby discovery feature", Category: "location", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		{Key: "location.nearby_require_mutual_optin", Scope: "user-service", Type: RuntimeConfigTypeBool, Value: "false", DefaultValue: "false", Description: "Require user to enable discoverable_nearby before viewing nearby people", Category: "location", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		{Key: "location.nearby_max_radius_km", Scope: "client", Type: RuntimeConfigTypeInt, Value: "10", DefaultValue: "10", Description: "Maximum search radius in kilometers for People Nearby", Category: "location", ValidationJSON: `{"min":1,"max":100}`, Version: 1, IsEditable: true},
		{Key: "location.nearby_ttl_seconds", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "900", DefaultValue: "900", Description: "Heartbeat TTL in seconds for active People Nearby entries in Redis", Category: "location", ValidationJSON: `{"min":60,"max":86400}`, Version: 1, IsEditable: true},
		{Key: "location.gps_min_interval_seconds", Scope: "client", Type: RuntimeConfigTypeInt, Value: "30", DefaultValue: "30", Description: "Minimum interval in seconds between GPS updates from client", Category: "location", ValidationJSON: `{"min":5,"max":3600}`, Version: 1, IsEditable: true},
		{Key: "location.privacy_fuzz_meters", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "100", DefaultValue: "100", Description: "Random coordinate fuzzing radius in meters before storing in Redis GEO", Category: "location", ValidationJSON: `{"min":0,"max":1000}`, Version: 1, IsEditable: true},
		{Key: "telemetry.collect_ip_device_enabled", Scope: "api-gateway", Type: RuntimeConfigTypeBool, Value: "true", DefaultValue: "true", Description: "Enable persistent IP, device, and GPS telemetry collection via Kafka/Cassandra", Category: "telemetry", ValidationJSON: `{}`, Version: 1, IsEditable: true},
		{Key: "telemetry.retention_days", Scope: "user-service", Type: RuntimeConfigTypeInt, Value: "180", DefaultValue: "180", Description: "Number of days to retain raw telemetry events in Cassandra", Category: "telemetry", ValidationJSON: `{"min":7,"max":730}`, Version: 1, IsEditable: true},
	}
}
