package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort       string
	MongoURI       string
	RedisAddr      string
	RedisPass      string
	UserGrpcAddr   string
	Neo4jURI       string
	Neo4jUser      string
	Neo4jPass      string
	FileGrpcAddr   string
	FileServiceURL string
	// WebRTC TURN server configuration
	TurnURL  string
	TurnUser string
	TurnPass string
}

func LoadConfig() *Config {
	_ = godotenv.Load()
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		HTTPPort:       getEnv("CHAT_HTTP_PORT", "10084"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017/chat_db"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:      getEnv("REDIS_PASSWORD", ""),
		UserGrpcAddr:   getEnv("USER_GRPC_ADDR", "localhost:10052"),
		Neo4jURI:       getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:      getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:      getEnv("NEO4J_PASS", "password"),
		FileGrpcAddr:   getEnv("FILE_GRPC_ADDR", "localhost:10057"),
		FileServiceURL: getEnv("FILE_SERVICE_URL", "http://localhost:11111/v1/files"),
		TurnURL:        getEnv("TURN_URL", ""),
		TurnUser:       getEnv("TURN_USER", ""),
		TurnPass:       getEnv("TURN_PASS", ""),
	}
}
