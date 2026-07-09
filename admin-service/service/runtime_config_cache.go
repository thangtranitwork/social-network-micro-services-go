package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"social-network-go/admin-service/db"
	"social-network-go/admin-service/model"
	"social-network-go/admin-service/repository"
)

const (
	runtimeConfigAllKey        = "runtime_config:all"
	runtimeConfigVersionKey    = "runtime_config:version"
	runtimeConfigSyncLockKey   = "runtime_config:sync_lock"
	runtimeConfigChangedChan   = "runtime_config:changed"
	runtimeConfigSyncedChan    = "runtime_config:synced"
	runtimeConfigSyncLockTTL   = 30 * time.Second
	runtimeConfigScopeKeyPrefx = "runtime_config:"
)

var (
	ErrRuntimeConfigCacheUnavailable = errors.New("runtime config cache unavailable")
	ErrRuntimeConfigSyncInProgress   = errors.New("runtime config sync is already in progress")
)

type runtimeConfigCacheEnvelope struct {
	Key       string    `json:"key"`
	Scope     string    `json:"scope"`
	Type      string    `json:"type"`
	Value     string    `json:"value"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (s *AdminService) SyncRuntimeConfigCache(ctx context.Context, actorID, reason string) (*RuntimeConfigSyncResult, error) {
	if s.runtimeConfigRepo == nil {
		return nil, ErrRuntimeConfigUnavailable
	}
	if db.RedisClient == nil {
		return nil, ErrRuntimeConfigCacheUnavailable
	}
	locked, err := db.RedisClient.SetNX(ctx, runtimeConfigSyncLockKey, actorID, runtimeConfigSyncLockTTL).Result()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrRuntimeConfigSyncInProgress
	}
	defer db.RedisClient.Del(context.Background(), runtimeConfigSyncLockKey)

	configs, err := s.runtimeConfigRepo.List(ctx, repository.RuntimeConfigListFilter{})
	if err != nil {
		return nil, err
	}
	result, err := writeRuntimeConfigSnapshot(ctx, db.RedisClient, configs)
	if err != nil {
		return nil, err
	}
	event := map[string]interface{}{
		"version":  result.Version,
		"syncedBy": actorID,
		"reason":   reason,
		"syncedAt": result.SyncedAt.Format(time.RFC3339),
	}
	publishRuntimeConfigEvent(ctx, runtimeConfigSyncedChan, event)
	return result, nil
}

func (s *AdminService) writeRuntimeConfigCache(ctx context.Context, configs []model.RuntimeConfig) error {
	if len(configs) == 0 {
		return nil
	}
	if db.RedisClient == nil {
		return ErrRuntimeConfigCacheUnavailable
	}
	pipe := db.RedisClient.Pipeline()
	for _, config := range configs {
		payload, err := json.Marshal(runtimeConfigCacheEnvelope{
			Key:       config.Key,
			Scope:     config.Scope,
			Type:      config.Type,
			Value:     config.Value,
			Version:   config.Version,
			UpdatedAt: config.UpdatedAt,
		})
		if err != nil {
			return err
		}
		pipe.HSet(ctx, runtimeConfigAllKey, config.Key, payload)
		pipe.HSet(ctx, runtimeConfigScopeKey(config.Scope), config.Key, payload)
	}
	pipe.Incr(ctx, runtimeConfigVersionKey)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	for _, config := range configs {
		publishRuntimeConfigEvent(ctx, runtimeConfigChangedChan, map[string]interface{}{
			"key":     config.Key,
			"scope":   config.Scope,
			"version": config.Version,
		})
	}
	return nil
}

func writeRuntimeConfigSnapshot(ctx context.Context, client *redis.Client, configs []model.RuntimeConfig) (*RuntimeConfigSyncResult, error) {
	scopeSet := make(map[string]struct{})
	version := int64(0)
	for _, config := range configs {
		scopeSet[config.Scope] = struct{}{}
		if config.Version > version {
			version = config.Version
		}
	}
	clearedKeys := []string{runtimeConfigAllKey, runtimeConfigVersionKey}
	for scope := range scopeSet {
		clearedKeys = append(clearedKeys, runtimeConfigScopeKey(scope))
	}

	pipe := client.Pipeline()
	pipe.Del(ctx, clearedKeys...)
	for _, config := range configs {
		payload, err := json.Marshal(runtimeConfigCacheEnvelope{
			Key:       config.Key,
			Scope:     config.Scope,
			Type:      config.Type,
			Value:     config.Value,
			Version:   config.Version,
			UpdatedAt: config.UpdatedAt,
		})
		if err != nil {
			return nil, err
		}
		pipe.HSet(ctx, runtimeConfigAllKey, config.Key, payload)
		pipe.HSet(ctx, runtimeConfigScopeKey(config.Scope), config.Key, payload)
	}
	pipe.Set(ctx, runtimeConfigVersionKey, version, 0)
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, err
	}

	return &RuntimeConfigSyncResult{
		Synced:      true,
		ConfigCount: len(configs),
		Version:     version,
		ClearedKeys: clearedKeys,
		SyncedAt:    time.Now(),
	}, nil
}

func runtimeConfigScopeKey(scope string) string {
	return fmt.Sprintf("%s%s", runtimeConfigScopeKeyPrefx, scope)
}

func publishRuntimeConfigEvent(ctx context.Context, channel string, payload map[string]interface{}) {
	if db.RedisClient == nil {
		return
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_ = db.RedisClient.Publish(ctx, channel, encoded).Err()
}
