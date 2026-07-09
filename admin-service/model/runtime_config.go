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
	}
}
