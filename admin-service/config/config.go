package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort    string
	Neo4jURI    string
	Neo4jUser   string
	Neo4jPass   string
	RedisAddr   string
	RedisPass   string
	PostgresDSN string
	KafkaAddr   string
}

func LoadConfig() *Config {
	_ = godotenv.Load("admin-service/.env")
	_ = godotenv.Load("auth-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		HTTPPort:    getEnv("ADMIN_HTTP_PORT", "10088"),
		Neo4jURI:    getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:   getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:   getEnv("NEO4J_PASS", "password"),
		RedisAddr:   getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:   getEnv("REDIS_PASSWORD", ""),
		PostgresDSN: getEnv("POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=auth_db port=5432 sslmode=disable"),
		KafkaAddr:   getEnv("KAFKA_ADDR", "localhost:9092"),
	}
}
