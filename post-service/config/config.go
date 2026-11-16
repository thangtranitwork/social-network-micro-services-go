package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

const (
	RankingModeLoaded        = "LOADED"
	RankingModeShadow        = "SHADOW"
	RankingModeQualifiedView = "QUALIFIED_VIEW"
)

type Config struct {
	HTTPPort             string
	Neo4jURI             string
	Neo4jUser            string
	Neo4jPass            string
	RedisAddr            string
	RedisPass            string
	UserGrpcAddr         string
	FileGrpcAddr         string
	KafkaAddr            string
	FileServiceURL       string
	AdminServiceURL      string
	GRPCPort             string
	PostgresDSN          string
	RankingViewMode         string
	ViewPenaltyWeight       int
	ViewDecayWindow         time.Duration
	KafkaTopicViewEvents    string
	KafkaTopicViewEventsDLQ string
	CassandraHosts          string
	CassandraKeyspace       string
	CassandraRetentionDays  int
	KafkaGroupIDHistory     string
}

func LoadConfig() *Config {
	_ = godotenv.Load("post-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Overload(".env")
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	getEnvInt := func(key string, fallback int) int {
		if val, ok := os.LookupEnv(key); ok {
			if intVal, err := strconv.Atoi(val); err == nil {
				return intVal
			}
		}
		return fallback
	}

	decayHours := getEnvInt("POST_VIEW_DECAY_HOURS", 24)

	return &Config{
		HTTPPort:             getEnv("POST_HTTP_PORT", "10083"),
		Neo4jURI:             getEnv("NEO4J_URI", "bolt://localhost:17687"),
		Neo4jUser:            getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:            getEnv("NEO4J_PASS", "password"),
		RedisAddr:            getEnv("REDIS_ADDR", "localhost:16379"),
		RedisPass:            getEnv("REDIS_PASSWORD", ""),
		UserGrpcAddr:         getEnv("USER_GRPC_ADDR", "localhost:10052"),
		FileGrpcAddr:         getEnv("FILE_GRPC_ADDR", "localhost:10057"),
		KafkaAddr:            getEnv("KAFKA_ADDR", "localhost:19092"),
		FileServiceURL:       getEnv("FILE_SERVICE_URL", "http://localhost:11111/v1/files"),
		AdminServiceURL:      getEnv("ADMIN_SERVICE_URL", "http://localhost:10088"),
		GRPCPort:             getEnv("POST_GRPC_PORT", "10053"),
		PostgresDSN:          getEnv("POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=auth_db port=15432 sslmode=disable"),
		RankingViewMode:         getEnv("POST_RANKING_VIEW_MODE", RankingModeLoaded),
		ViewPenaltyWeight:       getEnvInt("POST_VIEW_PENALTY_WEIGHT", 40),
		ViewDecayWindow:         time.Duration(decayHours) * time.Hour,
		KafkaTopicViewEvents:    getEnv("KAFKA_TOPIC_VIEW_EVENTS", "post-view-events"),
		KafkaTopicViewEventsDLQ: getEnv("KAFKA_TOPIC_VIEW_EVENTS_DLQ", "post-view-events-dlq"),
		CassandraHosts:          getEnv("CASSANDRA_HOSTS", "127.0.0.1:19042"),
		CassandraKeyspace:       getEnv("CASSANDRA_KEYSPACE", "post_history"),
		CassandraRetentionDays:  getEnvInt("CASSANDRA_RETENTION_DAYS", 90),
		KafkaGroupIDHistory:     getEnv("KAFKA_GROUP_ID_VIEW_EVENTS_HISTORY", "post-history-view-consumer"),
	}
}
