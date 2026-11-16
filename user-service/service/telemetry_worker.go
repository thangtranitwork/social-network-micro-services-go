package service

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gocql/gocql"
	"github.com/segmentio/kafka-go"

	"social-network-go/internal/runtimeconfig"
	"social-network-go/logger"
	"social-network-go/user-service/config"
	"social-network-go/user-service/model"
)

// StartTelemetryConsumer consumes user.telemetry.events from Kafka and persists them to Cassandra
// for long-term analytics (with configurable TTL).
func StartTelemetryConsumer(ctx context.Context, cfg *config.Config, session *gocql.Session, rtCfg *runtimeconfig.Provider) {
	if cfg == nil || cfg.KafkaAddr == "" || session == nil {
		logger.Info("Telemetry Kafka->Cassandra consumer skipped (Kafka or Cassandra not configured)")
		return
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  []string{cfg.KafkaAddr},
		Topic:    TelemetryKafkaTopic,
		GroupID:  "user-service-telemetry-cassandra",
		MinBytes: 1,
		MaxBytes: 10e6,
	})

	go func() {
		defer reader.Close()
		logger.Info("Started telemetry consumer on topic %s", TelemetryKafkaTopic)

		for {
			msg, err := reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				logger.Warn("Telemetry consumer read error: %v", err)
				time.Sleep(2 * time.Second)
				continue
			}

			var ev model.TelemetryEvent
			if err := json.Unmarshal(msg.Value, &ev); err != nil {
				continue
			}
			if ev.UserID == "" || ev.EventID == "" {
				continue
			}
			if ev.CreatedAt.IsZero() {
				ev.CreatedAt = time.Now().UTC()
			}

			retentionDays := 180
			if rtCfg != nil {
				retentionDays = rtCfg.Int(ctx, "telemetry.retention_days", 180)
			}
			if retentionDays < 7 {
				retentionDays = 7
			}
			ttlSeconds := retentionDays * 86400

			yearMonth := ev.CreatedAt.Format("2006-01")
			eventDate := ev.CreatedAt.Format("2006-01-02")

			q1 := `INSERT INTO user_telemetry_events (
				user_id, year_month, created_at, event_id, event_type,
				lat, lng, accuracy, location_source,
				ip_address, geo_country, geo_city,
				device_id, platform, device_model, os_version, app_version, user_agent
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) USING TTL ?`

			if err := session.Query(q1,
				ev.UserID, yearMonth, ev.CreatedAt, ev.EventID, ev.EventType,
				ev.Lat, ev.Lng, ev.Accuracy, ev.LocationSource,
				ev.IPAddress, ev.GeoCountry, ev.GeoCity,
				ev.DeviceID, ev.Platform, ev.DeviceModel, ev.OSVersion, ev.AppVersion, ev.UserAgent,
				ttlSeconds,
			).Exec(); err != nil {
				logger.Warn("Failed to insert user_telemetry_events to Cassandra: %v", err)
			}

			city := ev.GeoCity
			if city == "" {
				city = "unknown"
			}
			q2 := `INSERT INTO location_analytics_by_city_day (
				geo_city, event_date, created_at, event_id, user_id, lat, lng, platform
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?) USING TTL ?`

			if err := session.Query(q2,
				city, eventDate, ev.CreatedAt, ev.EventID, ev.UserID, ev.Lat, ev.Lng, ev.Platform,
				ttlSeconds,
			).Exec(); err != nil {
				logger.Warn("Failed to insert location_analytics_by_city_day to Cassandra: %v", err)
			}
		}
	}()
}
