package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"gorm.io/gorm/clause"

	"social-network-go/logger"
	"social-network-go/user-service/db"
	"social-network-go/user-service/model"
	"social-network-go/user-service/redis"
)

const (
	NearbyUsersGeoKey    = "nearby:users"
	NearbyAliveKeyPrefix = "nearby:alive:"
	UserLastLocKeyPrefix = "user:loc:"
	LocationRatePrefix   = "loc:rate:"
	TelemetryKafkaTopic  = "user.telemetry.events"
)

var ErrInvalidCoordinates = errors.New("invalid latitude or longitude")

func (s *UserService) GetLocationSettings(ctx context.Context, userID string) (*model.UserLocationSettingsEntity, error) {
	settings := &model.UserLocationSettingsEntity{
		UserID:                  userID,
		DiscoverableNearby:      false,
		LocationPersonalization: true,
	}
	if db.SQLDB == nil || userID == "" {
		return settings, nil
	}
	var existing model.UserLocationSettingsEntity
	err := db.SQLDB.WithContext(ctx).Where("user_id = ?", userID).First(&existing).Error
	if err != nil {
		return settings, nil
	}
	return &existing, nil
}

func (s *UserService) UpdateLocationSettings(ctx context.Context, userID string, req model.LocationSettingsUpdateRequest) (*model.UserLocationSettingsEntity, error) {
	settings, _ := s.GetLocationSettings(ctx, userID)
	settings.UserID = userID
	if req.DiscoverableNearby != nil {
		settings.DiscoverableNearby = *req.DiscoverableNearby
	}
	if req.LocationPersonalization != nil {
		settings.LocationPersonalization = *req.LocationPersonalization
	}
	settings.UpdatedAt = time.Now()

	if db.SQLDB != nil {
		if err := db.SQLDB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"discoverable_nearby", "location_personalization", "updated_at"}),
		}).Create(settings).Error; err != nil {
			return nil, err
		}
	}

	if !settings.DiscoverableNearby {
		_ = s.ClearNearbyLocation(ctx, userID)
	}
	return settings, nil
}

func (s *UserService) UpdateUserLocation(ctx context.Context, userID string, req model.LocationUpdateRequest, meta model.ClientTelemetryMeta) (map[string]interface{}, error) {
	if req.Lat < -90 || req.Lat > 90 || req.Lng < -180 || req.Lng > 180 {
		return nil, ErrInvalidCoordinates
	}

	settings, _ := s.GetLocationSettings(ctx, userID)
	if req.City != "" || req.Country != "" {
		settings.UserID = userID
		if req.City != "" {
			settings.LastCity = req.City
		}
		if req.Country != "" {
			settings.LastCountry = req.Country
		}
		settings.UpdatedAt = time.Now()
		if db.SQLDB != nil {
			_ = db.SQLDB.WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "user_id"}},
				DoUpdates: clause.AssignmentColumns([]string{"last_city", "last_country", "updated_at"}),
			}).Create(settings).Error
		}
	}

	// Upsert device & IP session in Postgres if telemetry collection is enabled
	collectTelemetry := s.runtimeBool(ctx, "telemetry.collect_ip_device_enabled", true)
	if collectTelemetry && db.SQLDB != nil && meta.DeviceID != "" {
		sessionID := fmt.Sprintf("%s:%s", userID, meta.DeviceID)
		sess := model.UserDeviceSessionEntity{
			ID:           sessionID,
			UserID:       userID,
			DeviceID:     meta.DeviceID,
			Platform:     meta.Platform,
			DeviceModel:  meta.DeviceModel,
			OSVersion:    meta.OSVersion,
			AppVersion:   meta.AppVersion,
			IPAddress:    meta.IPAddress,
			UserAgent:    meta.UserAgent,
			LastActiveAt: time.Now(),
			CreatedAt:    time.Now(),
		}
		_ = db.SQLDB.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"platform", "device_model", "os_version", "app_version", "ip_address", "user_agent", "last_active_at"}),
		}).Create(&sess).Error
	}

	// Update Redis hot caches
	nearbyActive := false
	if redis.RedisClient != nil {
		if settings.LocationPersonalization {
			locPayload, _ := json.Marshal(map[string]interface{}{
				"lat":       req.Lat,
				"lng":       req.Lng,
				"accuracy":  req.Accuracy,
				"city":      req.City,
				"country":   req.Country,
				"updatedAt": time.Now().UTC().Format(time.RFC3339),
			})
			_ = redis.RedisClient.Set(ctx, UserLastLocKeyPrefix+userID, locPayload, 24*time.Hour).Err()
		}

		if settings.DiscoverableNearby {
			fuzzMeters := s.runtimeInt(ctx, "location.privacy_fuzz_meters", 100)
			ttlSeconds := s.runtimeInt(ctx, "location.nearby_ttl_seconds", 900)
			fuzzedLat, fuzzedLng := fuzzCoordinates(req.Lat, req.Lng, fuzzMeters)

			pipe := redis.RedisClient.Pipeline()
			pipe.GeoAdd(ctx, NearbyUsersGeoKey, &goredis.GeoLocation{
				Name:      userID,
				Longitude: fuzzedLng,
				Latitude:  fuzzedLat,
			})
			pipe.Set(ctx, NearbyAliveKeyPrefix+userID, "1", time.Duration(ttlSeconds)*time.Second)
			if _, err := pipe.Exec(ctx); err != nil {
				logger.Warn("Failed to update Redis GEO for user %s: %v", userID, err)
			} else {
				nearbyActive = true
			}
		}
	}

	// Publish persistent telemetry event to Kafka -> Cassandra asynchronously
	if collectTelemetry {
		eventType := req.EventType
		if eventType == "" {
			eventType = "app_location_ping"
		}
		source := req.Source
		if source == "" {
			source = "gps"
		}
		ev := model.TelemetryEvent{
			EventID:        uuid.NewString(),
			UserID:         userID,
			EventType:      eventType,
			Lat:            req.Lat,
			Lng:            req.Lng,
			Accuracy:       req.Accuracy,
			LocationSource: source,
			IPAddress:      meta.IPAddress,
			GeoCountry:     req.Country,
			GeoCity:        req.City,
			DeviceID:       meta.DeviceID,
			Platform:       meta.Platform,
			DeviceModel:    meta.DeviceModel,
			OSVersion:      meta.OSVersion,
			AppVersion:     meta.AppVersion,
			UserAgent:      meta.UserAgent,
			CreatedAt:      time.Now().UTC(),
		}
		go s.publishTelemetryEvent(ev)
	}

	return map[string]interface{}{
		"discoverableNearby": settings.DiscoverableNearby,
		"nearbyActive":       nearbyActive,
		"updatedAt":          time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *UserService) ClearNearbyLocation(ctx context.Context, userID string) error {
	if redis.RedisClient == nil || userID == "" {
		return nil
	}
	pipe := redis.RedisClient.Pipeline()
	pipe.ZRem(ctx, NearbyUsersGeoKey, userID)
	pipe.Del(ctx, NearbyAliveKeyPrefix+userID)
	_, err := pipe.Exec(ctx)
	return err
}

func (s *UserService) ClearAllUserLocation(ctx context.Context, userID string) error {
	if redis.RedisClient != nil && userID != "" {
		pipe := redis.RedisClient.Pipeline()
		pipe.ZRem(ctx, NearbyUsersGeoKey, userID)
		pipe.Del(ctx, NearbyAliveKeyPrefix+userID, UserLastLocKeyPrefix+userID)
		_, _ = pipe.Exec(ctx)
	}
	if db.SQLDB != nil && userID != "" {
		_ = db.SQLDB.WithContext(ctx).Model(&model.UserLocationSettingsEntity{}).
			Where("user_id = ?", userID).
			Updates(map[string]interface{}{"discoverable_nearby": false, "updated_at": time.Now()}).Error
	}
	return nil
}

func (s *UserService) GetDeviceSessions(ctx context.Context, userID string) ([]model.UserDeviceSessionEntity, error) {
	if db.SQLDB == nil || userID == "" {
		return []model.UserDeviceSessionEntity{}, nil
	}
	var sessions []model.UserDeviceSessionEntity
	err := db.SQLDB.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("last_active_at DESC").
		Limit(20).
		Find(&sessions).Error
	return sessions, err
}

func (s *UserService) publishTelemetryEvent(ev model.TelemetryEvent) {
	if s.cfg == nil || s.cfg.KafkaAddr == "" {
		return
	}
	payload, err := json.Marshal(ev)
	if err != nil {
		return
	}
	writer := &kafka.Writer{
		Addr:         kafka.TCP(s.cfg.KafkaAddr),
		Topic:        TelemetryKafkaTopic,
		Balancer:     &kafka.LeastBytes{},
		BatchTimeout: 50 * time.Millisecond,
	}
	defer writer.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(ev.UserID),
		Value: payload,
	}); err != nil {
		logger.Warn("Failed to publish telemetry event to Kafka: %v", err)
	}
}

func (s *UserService) runtimeBool(ctx context.Context, key string, fallback bool) bool {
	if s == nil || s.RuntimeCfg == nil {
		return fallback
	}
	raw, ok := s.RuntimeCfg.String(ctx, key)
	if !ok {
		return fallback
	}
	return raw == "true"
}

// fuzzCoordinates shifts (lat, lng) randomly within [0, maxMeters] to protect exact user coordinates.
func fuzzCoordinates(lat, lng float64, maxMeters int) (float64, float64) {
	if maxMeters <= 0 {
		return lat, lng
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return lat, lng
	}
	r1 := float64(binary.LittleEndian.Uint64(buf[:8])) / float64(math.MaxUint64)
	r2 := float64(binary.LittleEndian.Uint64(buf[8:])) / float64(math.MaxUint64)

	dist := math.Sqrt(r1) * float64(maxMeters)
	angle := r2 * 2 * math.Pi

	dLat := (dist * math.Cos(angle)) / 111320.0
	cosLat := math.Cos(lat * math.Pi / 180.0)
	if math.Abs(cosLat) < 0.01 {
		cosLat = 0.01
	}
	dLng := (dist * math.Sin(angle)) / (111320.0 * cosLat)
	return lat + dLat, lng + dLng
}
