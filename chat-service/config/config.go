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
	StringeeSid    string
	StringeeSecret string
	Neo4jURI       string
	Neo4jUser      string
	Neo4jPass      string
	FileGrpcAddr   string
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
		HTTPPort:       getEnv("CHAT_HTTP_PORT", "8084"),
		MongoURI:       getEnv("MONGO_URI", "mongodb://localhost:27017/chat_db"),
		RedisAddr:      getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:      getEnv("REDIS_PASSWORD", ""),
		UserGrpcAddr:   getEnv("USER_GRPC_ADDR", "localhost:50052"),
		StringeeSid:    getEnv("STRINGEE_API_SID", "SK.0.ti3nCMW4vbnmLCmglbkEc12Z34otwkfe"),
		StringeeSecret: getEnv("STRINGEE_API_SECRET_KEY", "dWY2U1RoWlVkN3d0c2hDeUxUTVVvVm82c0RIaHdIYzM="),
		Neo4jURI:       getEnv("NEO4J_URI", "neo4j://localhost:7687"),
		Neo4jUser:      getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:      getEnv("NEO4J_PASS", "password"),
		FileGrpcAddr:   getEnv("FILE_GRPC_ADDR", "localhost:50057"),
	}
}
