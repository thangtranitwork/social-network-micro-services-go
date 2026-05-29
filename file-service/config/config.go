package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort       string
	GRPCPort       string
	MinioEndpoint  string
	MinioAccessKey string
	MinioSecretKey string
	MinioUseSSL    bool
	MinioBucket    string
	AuthGrpcAddr   string
}

func LoadConfig() *Config {
	_ = godotenv.Load("file-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")

	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	return &Config{
		HTTPPort:       getEnv("FILE_HTTP_PORT", "10087"),
		GRPCPort:       getEnv("FILE_GRPC_PORT", "10057"),
		MinioEndpoint:  getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinioAccessKey: getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey: getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioUseSSL:    getEnv("MINIO_USE_SSL", "false") == "true",
		MinioBucket:    getEnv("MINIO_BUCKET", "social-network"),
		AuthGrpcAddr:   getEnv("AUTH_GRPC_ADDR", "localhost:10051"),
	}
}
