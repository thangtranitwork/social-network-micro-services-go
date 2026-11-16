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

// GetPublicMapConfig returns non-sensitive client map and location configuration,
// reading from Redis cache first and falling back to PostgreSQL or defaults.
func (s *AdminService) GetPublicMapConfig(ctx context.Context) map[string]interface{} {
	values := map[string]string{
		"map.provider":                     "carto",
		"map.style":                        "auto",
		"map.google_tiles_url":             "https://mt{s}.google.com/vt/lyrs=r&x={x}&y={y}&z={z}&hl=vi",
		"map.google_api_key":               "",
		"map.default_center":               `{"lat":10.7769,"lng":106.7009,"zoom":13}`,
		"location.nearby_enabled":          "true",
		"location.nearby_max_radius_km":    "10",
		"location.gps_min_interval_seconds": "30",
	}

	loadedFromRedis := false
	if db.RedisClient != nil {
		if rawMap, err := db.RedisClient.HGetAll(ctx, runtimeConfigScopeKey("client")).Result(); err == nil && len(rawMap) > 0 {
			for k, raw := range rawMap {
				var env runtimeConfigCacheEnvelope
				if jsonErr := json.Unmarshal([]byte(raw), &env); jsonErr == nil {
					values[k] = env.Value
					loadedFromRedis = true
				}
			}
		}
	}

	if !loadedFromRedis && s.runtimeConfigRepo != nil {
		if items, err := s.runtimeConfigRepo.List(ctx, repository.RuntimeConfigListFilter{Scope: "client"}); err == nil {
			for _, item := range items {
				values[item.Key] = item.Value
			}
		}
	}

	var defaultCenter map[string]interface{}
	if err := json.Unmarshal([]byte(values["map.default_center"]), &defaultCenter); err != nil {
		defaultCenter = map[string]interface{}{"lat": 10.7769, "lng": 106.7009, "zoom": 13}
	}

	nearbyEnabled := values["location.nearby_enabled"] == "true"
	maxRadiusKm := 10
	if _, err := fmt.Sscanf(values["location.nearby_max_radius_km"], "%d", &maxRadiusKm); err != nil || maxRadiusKm <= 0 {
		maxRadiusKm = 10
	}
	minIntervalSec := 30
	if _, err := fmt.Sscanf(values["location.gps_min_interval_seconds"], "%d", &minIntervalSec); err != nil || minIntervalSec <= 0 {
		minIntervalSec = 30
	}

	res := map[string]interface{}{
		"provider":              values["map.provider"],
		"style":                 values["map.style"],
		"googleTilesUrl":        values["map.google_tiles_url"],
		"defaultCenter":         defaultCenter,
		"nearbyEnabled":         nearbyEnabled,
		"nearbyMaxRadiusKm":     maxRadiusKm,
		"gpsMinIntervalSeconds": minIntervalSec,
	}
	if values["map.provider"] == "google_sdk" && values["map.google_api_key"] != "" {
		res["googleApiKey"] = values["map.google_api_key"]
	}
	return res
}
