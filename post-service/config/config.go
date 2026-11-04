package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort        string
	Neo4jURI        string
	Neo4jUser       string
	Neo4jPass       string
	RedisAddr       string
	RedisPass       string
	UserGrpcAddr    string
	FileGrpcAddr    string
	KafkaAddr       string
	FileServiceURL  string
	AdminServiceURL string
	GRPCPort        string
	PostgresDSN     string
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

	return &Config{
		HTTPPort:        getEnv("POST_HTTP_PORT", "10083"),
		Neo4jURI:        getEnv("NEO4J_URI", "bolt://localhost:17687"),
		Neo4jUser:       getEnv("NEO4J_USER", "neo4j"),
		Neo4jPass:       getEnv("NEO4J_PASS", "password"),
		RedisAddr:       getEnv("REDIS_ADDR", "localhost:16379"),
		RedisPass:       getEnv("REDIS_PASSWORD", ""),
		UserGrpcAddr:    getEnv("USER_GRPC_ADDR", "localhost:10052"),
		FileGrpcAddr:    getEnv("FILE_GRPC_ADDR", "localhost:10057"),
		KafkaAddr:       getEnv("KAFKA_ADDR", "localhost:19092"),
		FileServiceURL:  getEnv("FILE_SERVICE_URL", "http://localhost:11111/v1/files"),
		AdminServiceURL: getEnv("ADMIN_SERVICE_URL", "http://localhost:10088"),
		GRPCPort:        getEnv("POST_GRPC_PORT", "10053"),
		PostgresDSN:     getEnv("POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=auth_db port=15432 sslmode=disable"),
	}
}
