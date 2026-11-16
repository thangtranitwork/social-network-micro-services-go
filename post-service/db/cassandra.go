package db

import (
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	"social-network-go/logger"
	"social-network-go/post-service/config"
)

var CassandraSession *gocql.Session

// InitCassandra initializes the gocql cluster session for view history
func InitCassandra(cfg *config.Config) *gocql.Session {
	if cfg == nil || cfg.CassandraHosts == "" {
		logger.Warn("Cassandra hosts not configured; view history Cassandra storage disabled")
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
		logger.Warn("No valid Cassandra hosts parsed")
		return nil
	}

	cluster := gocql.NewCluster(hosts...)
	cluster.Port = port
	cluster.Timeout = 5 * time.Second
	cluster.ConnectTimeout = 5 * time.Second
	cluster.Consistency = gocql.One // Single-node local / dev compatibility

	// Initial connection without keyspace to ensure keyspace exists
	initSession, err := cluster.CreateSession()
	if err != nil {
		logger.Warn("Warning: Cassandra is unreachable at %v:%d: %v", hosts, port, err)
		return nil
	}

	keyspace := cfg.CassandraKeyspace
	if keyspace == "" {
		keyspace = "post_history"
	}

	createKeyspaceCQL := `
		CREATE KEYSPACE IF NOT EXISTS ` + keyspace + `
		WITH replication = {
			'class': 'SimpleStrategy',
			'replication_factor': 1
		}
	`
	if err := initSession.Query(createKeyspaceCQL).Exec(); err != nil {
		logger.Warn("Failed to ensure Cassandra keyspace %s: %v", keyspace, err)
		initSession.Close()
		return nil
	}
	initSession.Close()

	// Connect with keyspace
	cluster.Keyspace = keyspace
	session, err := cluster.CreateSession()
	if err != nil {
		logger.Warn("Failed to create Cassandra session with keyspace %s: %v", keyspace, err)
		return nil
	}

	createTableCQL := `
		CREATE TABLE IF NOT EXISTS ` + keyspace + `.views_by_user_month (
			user_id text,
			month text,
			observed_at timestamp,
			event_id text,
			post_id text,
			visible_duration_ms int,
			visible_ratio double,
			surface text,
			schema_version int,
			PRIMARY KEY ((user_id, month), observed_at, event_id)
		) WITH CLUSTERING ORDER BY (observed_at DESC, event_id ASC)
	`
	if err := session.Query(createTableCQL).Exec(); err != nil {
		logger.Warn("Failed to ensure Cassandra table views_by_user_month: %v", err)
	} else {
		logger.Info("Cassandra table views_by_user_month verified/created successfully")
	}

	CassandraSession = session
	logger.Info("Successfully connected to Cassandra View History Storage (%s on %v:%d)", keyspace, hosts, port)
	return CassandraSession
}
