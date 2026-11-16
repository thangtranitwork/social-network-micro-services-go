package db

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"social-network-go/logger"
	"social-network-go/user-service/config"
)

var CassandraSession *gocql.Session

// InitCassandra initializes the gocql session and ensures telemetry tables exist.
// Failure to connect is non-fatal so user-service keeps operating if Cassandra is down.
func InitCassandra(cfg *config.Config) *gocql.Session {
	if cfg == nil || strings.TrimSpace(cfg.CassandraHosts) == "" {
		logger.Warn("Cassandra hosts not configured; persistent telemetry storage disabled")
		return nil
	}

	rawHosts := strings.Split(cfg.CassandraHosts, ",")
	var hosts []string
	port := 9042

	for _, h := range rawHosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if strings.Contains(h, ":") {
			parts := strings.Split(h, ":")
			hosts = append(hosts, parts[0])
			if p, err := strconv.Atoi(parts[1]); err == nil && p > 0 {
				port = p
			}
		} else {
			hosts = append(hosts, h)
		}
	}

	if len(hosts) == 0 {
		return nil
	}

	cluster := gocql.NewCluster(hosts...)
	cluster.Port = port
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Consistency = gocql.One

	initSession, err := cluster.CreateSession()
	if err != nil {
		logger.Warn("Cassandra unreachable at %v:%d (telemetry storage disabled): %v", hosts, port, err)
		return nil
	}

	keyspace := cfg.CassandraKeyspace
	if keyspace == "" {
		keyspace = "social_network"
	}

	createKS := fmt.Sprintf(`CREATE KEYSPACE IF NOT EXISTS %s WITH replication = {'class': 'SimpleStrategy', 'replication_factor': 1}`, keyspace)
	if err := initSession.Query(createKS).Exec(); err != nil {
		logger.Warn("Failed to ensure Cassandra keyspace %s: %v", keyspace, err)
		initSession.Close()
		return nil
	}
	initSession.Close()

	cluster.Keyspace = keyspace
	session, err := cluster.CreateSession()
	if err != nil {
		logger.Warn("Failed to connect to Cassandra keyspace %s: %v", keyspace, err)
		return nil
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS user_telemetry_events (
			user_id text,
			year_month text,
			created_at timestamp,
			event_id text,
			event_type text,
			lat double,
			lng double,
			accuracy double,
			location_source text,
			ip_address text,
			geo_country text,
			geo_city text,
			device_id text,
			platform text,
			device_model text,
			os_version text,
			app_version text,
			user_agent text,
			PRIMARY KEY ((user_id, year_month), created_at, event_id)
		) WITH CLUSTERING ORDER BY (created_at DESC, event_id ASC)`,
		`CREATE TABLE IF NOT EXISTS location_analytics_by_city_day (
			geo_city text,
			event_date text,
			created_at timestamp,
			event_id text,
			user_id text,
			lat double,
			lng double,
			platform text,
			PRIMARY KEY ((geo_city, event_date), created_at, event_id)
		) WITH CLUSTERING ORDER BY (created_at DESC, event_id ASC)`,
	}

	for _, q := range queries {
		if err := session.Query(q).Exec(); err != nil {
			logger.Warn("Failed to create Cassandra telemetry table: %v", err)
		}
	}

	CassandraSession = session
	logger.Info("Successfully connected to Cassandra (%s) for persistent telemetry", keyspace)
	return session
}
