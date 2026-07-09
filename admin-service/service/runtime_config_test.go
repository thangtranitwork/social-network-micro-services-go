package service

import (
	"errors"
	"testing"

	"social-network-go/admin-service/model"
)

func TestValidateRuntimeConfigValueIntegerRange(t *testing.T) {
	config := model.RuntimeConfig{
		Key:            "post.max_content_length",
		Type:           model.RuntimeConfigTypeInt,
		ValidationJSON: `{"min":1,"max":5000}`,
	}

	if err := validateRuntimeConfigValue(config, "5000"); err != nil {
		t.Fatalf("expected valid integer config, got %v", err)
	}
	if err := validateRuntimeConfigValue(config, "5001"); !errors.Is(err, ErrInvalidRuntimeConfig) {
		t.Fatalf("expected invalid range error, got %v", err)
	}
	if err := validateRuntimeConfigValue(config, "abc"); !errors.Is(err, ErrInvalidRuntimeConfig) {
		t.Fatalf("expected invalid integer error, got %v", err)
	}
}

func TestValidateRuntimeConfigValueJSON(t *testing.T) {
	config := model.RuntimeConfig{
		Key:  "newsfeed.score_weights",
		Type: model.RuntimeConfigTypeJSON,
	}

	if err := validateRuntimeConfigValue(config, `{"like":2}`); err != nil {
		t.Fatalf("expected valid json config, got %v", err)
	}
	if err := validateRuntimeConfigValue(config, `{"like":}`); !errors.Is(err, ErrInvalidRuntimeConfig) {
		t.Fatalf("expected invalid json error, got %v", err)
	}
}

func TestBuildRuntimeConfigRequest(t *testing.T) {
	config, err := buildRuntimeConfig(RuntimeConfigCreateRequest{
		Key:          "post.custom_limit",
		Scope:        "post-service",
		Category:     "limits",
		Type:         "int",
		Value:        "10",
		DefaultValue: "5",
		Description:  "Custom limit",
		Validation: map[string]interface{}{
			"min": float64(1),
			"max": float64(20),
		},
	})
	if err != nil {
		t.Fatalf("expected valid create request, got %v", err)
	}
	if config.Type != model.RuntimeConfigTypeInt || config.ValidationJSON == "" {
		t.Fatalf("unexpected config: %+v", config)
	}

	_, err = buildRuntimeConfig(RuntimeConfigCreateRequest{
		Key:         "bad key",
		Scope:       "post-service",
		Category:    "limits",
		Type:        "INT",
		Value:       "10",
		Description: "Bad key",
	})
	if !errors.Is(err, ErrInvalidRuntimeConfig) {
		t.Fatalf("expected invalid key error, got %v", err)
	}
}

func TestDefaultRuntimeConfigsIncludeFirstBatch(t *testing.T) {
	configs := model.DefaultRuntimeConfigs()
	byKey := make(map[string]model.RuntimeConfig, len(configs))
	for _, config := range configs {
		byKey[config.Key] = config
	}

	for _, key := range []string{
		"post.max_content_length",
		"post.max_attach_files",
		"user.max_friend_count",
		"user.min_age",
		"newsfeed.score_weights",
	} {
		if _, ok := byKey[key]; !ok {
			t.Fatalf("expected default runtime config %s", key)
		}
	}
}
