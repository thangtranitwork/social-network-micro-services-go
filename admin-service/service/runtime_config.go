package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"social-network-go/admin-service/model"
	"social-network-go/admin-service/repository"
)

var (
	ErrRuntimeConfigUnavailable = errors.New("runtime config repository unavailable")
	ErrInvalidRuntimeConfig     = errors.New("invalid runtime config")
)

var runtimeConfigKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)

type RuntimeConfigCreateRequest struct {
	Key          string                 `json:"key"`
	Scope        string                 `json:"scope"`
	Category     string                 `json:"category"`
	Type         string                 `json:"type"`
	Value        string                 `json:"value"`
	DefaultValue string                 `json:"defaultValue"`
	Description  string                 `json:"description"`
	IsSensitive  bool                   `json:"isSensitive"`
	Validation   map[string]interface{} `json:"validation"`
	Reason       string                 `json:"reason"`
}

type RuntimeConfigUpdateRequest struct {
	Value           string `json:"value"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type RuntimeConfigSyncResult struct {
	Synced      bool      `json:"synced"`
	ConfigCount int       `json:"configCount"`
	Version     int64     `json:"version"`
	ClearedKeys []string  `json:"clearedKeys"`
	SyncedAt    time.Time `json:"syncedAt"`
}

func (s *AdminService) ListRuntimeConfigs(ctx context.Context, filter repository.RuntimeConfigListFilter) ([]model.RuntimeConfigResponse, error) {
	if s.runtimeConfigRepo == nil {
		return nil, ErrRuntimeConfigUnavailable
	}
	configs, err := s.runtimeConfigRepo.List(ctx, filter)
	if err != nil {
		return nil, err
	}
	responses := make([]model.RuntimeConfigResponse, 0, len(configs))
	for _, config := range configs {
		responses = append(responses, config.ToResponse())
	}
	return responses, nil
}

func (s *AdminService) CreateRuntimeConfig(ctx context.Context, req RuntimeConfigCreateRequest, actorID string) (*model.RuntimeConfigResponse, error) {
	if s.runtimeConfigRepo == nil {
		return nil, ErrRuntimeConfigUnavailable
	}
	config, err := buildRuntimeConfig(req)
	if err != nil {
		return nil, err
	}
	if err := validateRuntimeConfigValue(config, config.Value); err != nil {
		return nil, err
	}
	if err := validateRuntimeConfigValue(config, config.DefaultValue); err != nil {
		return nil, err
	}
	created, err := s.runtimeConfigRepo.Create(ctx, config, actorID, req.Reason)
	if err != nil {
		return nil, err
	}
	if err := s.writeRuntimeConfigCache(ctx, []model.RuntimeConfig{*created}); err != nil {
		return nil, err
	}
	resp := created.ToResponse()
	return &resp, nil
}

func (s *AdminService) UpdateRuntimeConfig(ctx context.Context, key string, req RuntimeConfigUpdateRequest, actorID string) (*model.RuntimeConfigResponse, error) {
	if s.runtimeConfigRepo == nil {
		return nil, ErrRuntimeConfigUnavailable
	}
	key = strings.TrimSpace(key)
	if key == "" || req.ExpectedVersion <= 0 {
		return nil, ErrInvalidRuntimeConfig
	}
	current, err := s.runtimeConfigRepo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	value := strings.TrimSpace(req.Value)
	if err := validateRuntimeConfigValue(*current, value); err != nil {
		return nil, err
	}
	updated, err := s.runtimeConfigRepo.UpdateValue(ctx, key, value, req.ExpectedVersion, actorID, req.Reason)
	if err != nil {
		return nil, err
	}
	if err := s.writeRuntimeConfigCache(ctx, []model.RuntimeConfig{*updated}); err != nil {
		return nil, err
	}
	resp := updated.ToResponse()
	return &resp, nil
}

func (s *AdminService) ResetRuntimeConfig(ctx context.Context, key, actorID, reason string) (*model.RuntimeConfigResponse, error) {
	if s.runtimeConfigRepo == nil {
		return nil, ErrRuntimeConfigUnavailable
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrInvalidRuntimeConfig
	}
	current, err := s.runtimeConfigRepo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := validateRuntimeConfigValue(*current, current.DefaultValue); err != nil {
		return nil, err
	}
	updated, err := s.runtimeConfigRepo.ResetValue(ctx, key, actorID, reason)
	if err != nil {
		return nil, err
	}
	if err := s.writeRuntimeConfigCache(ctx, []model.RuntimeConfig{*updated}); err != nil {
		return nil, err
	}
	resp := updated.ToResponse()
	return &resp, nil
}

func validateRuntimeConfigValue(config model.RuntimeConfig, value string) error {
	if value == "" && config.Type != model.RuntimeConfigTypeString {
		return fmt.Errorf("%w: value is required", ErrInvalidRuntimeConfig)
	}
	switch config.Type {
	case model.RuntimeConfigTypeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%w: expected integer", ErrInvalidRuntimeConfig)
		}
		return validateRuntimeConfigNumber(config, float64(parsed))
	case model.RuntimeConfigTypeBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return fmt.Errorf("%w: expected boolean", ErrInvalidRuntimeConfig)
		}
	case model.RuntimeConfigTypeString:
		return validateRuntimeConfigEnum(config, value)
	case model.RuntimeConfigTypeDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return fmt.Errorf("%w: expected Go duration string", ErrInvalidRuntimeConfig)
		}
	case model.RuntimeConfigTypeJSON:
		var payload interface{}
		if err := json.Unmarshal([]byte(value), &payload); err != nil {
			return fmt.Errorf("%w: expected valid JSON", ErrInvalidRuntimeConfig)
		}
	default:
		return fmt.Errorf("%w: unknown type %s", ErrInvalidRuntimeConfig, config.Type)
	}
	return nil
}

func validateRuntimeConfigNumber(config model.RuntimeConfig, value float64) error {
	validation := parseValidation(config.ValidationJSON)
	if min, ok := validation["min"].(float64); ok && value < min {
		return fmt.Errorf("%w: value is below minimum %.0f", ErrInvalidRuntimeConfig, min)
	}
	if max, ok := validation["max"].(float64); ok && value > max {
		return fmt.Errorf("%w: value is above maximum %.0f", ErrInvalidRuntimeConfig, max)
	}
	return nil
}

func validateRuntimeConfigEnum(config model.RuntimeConfig, value string) error {
	validation := parseValidation(config.ValidationJSON)
	rawEnum, ok := validation["enum"].([]interface{})
	if !ok || len(rawEnum) == 0 {
		return nil
	}
	for _, raw := range rawEnum {
		if allowed, ok := raw.(string); ok && value == allowed {
			return nil
		}
	}
	return fmt.Errorf("%w: value is not allowed", ErrInvalidRuntimeConfig)
}

func parseValidation(raw string) map[string]interface{} {
	if strings.TrimSpace(raw) == "" {
		return map[string]interface{}{}
	}
	var validation map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &validation); err != nil {
		return map[string]interface{}{}
	}
	return validation
}

func buildRuntimeConfig(req RuntimeConfigCreateRequest) (model.RuntimeConfig, error) {
	config := model.RuntimeConfig{
		Key:          strings.TrimSpace(req.Key),
		Scope:        strings.TrimSpace(req.Scope),
		Category:     strings.TrimSpace(req.Category),
		Type:         strings.ToUpper(strings.TrimSpace(req.Type)),
		Value:        strings.TrimSpace(req.Value),
		DefaultValue: strings.TrimSpace(req.DefaultValue),
		Description:  strings.TrimSpace(req.Description),
		IsSensitive:  req.IsSensitive,
		IsEditable:   true,
		Version:      1,
	}
	if config.DefaultValue == "" {
		config.DefaultValue = config.Value
	}
	if config.Key == "" || config.Scope == "" || config.Category == "" || config.Type == "" {
		return model.RuntimeConfig{}, fmt.Errorf("%w: key, scope, category and type are required", ErrInvalidRuntimeConfig)
	}
	if !runtimeConfigKeyPattern.MatchString(config.Key) {
		return model.RuntimeConfig{}, fmt.Errorf("%w: key must use namespaced lowercase format", ErrInvalidRuntimeConfig)
	}
	if config.Description == "" {
		return model.RuntimeConfig{}, fmt.Errorf("%w: description is required", ErrInvalidRuntimeConfig)
	}
	validation, err := json.Marshal(req.Validation)
	if err != nil {
		return model.RuntimeConfig{}, fmt.Errorf("%w: validation must be JSON serializable", ErrInvalidRuntimeConfig)
	}
	if len(req.Validation) > 0 {
		config.ValidationJSON = string(validation)
	}
	return config, nil
}
