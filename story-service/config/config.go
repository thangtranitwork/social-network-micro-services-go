package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort       string
	Neo4jURI       string
	Neo4jUser      string
	Neo4jPass      string
	RedisAddr      string
	RedisPass      string
	UserGrpcAddr   string
	FileServiceURL string
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
		HTTPPort:       getEnv("STORY_HTTP_PORT", "10090"),
		Neo4jURI:       getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:      getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:      getEnv("NEO4J_PASS", "password"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:      getEnv("REDIS_PASSWORD", ""),
		UserGrpcAddr:   getEnv("USER_GRPC_ADDR", "localhost:10052"),
		FileServiceURL: getEnv("FILE_SERVICE_URL", "http://localhost:11111/v1/files"),
	}
}
