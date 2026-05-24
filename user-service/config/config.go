package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	GRPCPort  string
	HTTPPort  string
	Neo4jURI  string
	Neo4jUser string
	Neo4jPass string
	RedisAddr string
	RedisPass    string
	FileGrpcAddr string
}

func LoadConfig() *Config {
	_ = godotenv.Load("user-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		GRPCPort:     getEnv("USER_GRPC_PORT", "50052"),
		HTTPPort:     getEnv("USER_HTTP_PORT", "8082"),
		Neo4jURI:     getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:    getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:    getEnv("NEO4J_PASS", "password"),
		RedisAddr:    getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:    getEnv("REDIS_PASSWORD", ""),
		FileGrpcAddr: getEnv("FILE_GRPC_ADDR", "localhost:50057"),
	}
}
