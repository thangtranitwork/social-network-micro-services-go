package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	HTTPPort             string
	GRPCPort             string
	MinioEndpoint        string
	MinioPublicEndpoint  string // public-facing endpoint for presigned URLs (e.g. 157.180.116.76:19000)
	MinioAccessKey       string
	MinioSecretKey       string
	MinioUseSSL          bool
	MinioBucket          string
	AuthGrpcAddr         string
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
		HTTPPort:            getEnv("FILE_HTTP_PORT", "10087"),
		GRPCPort:            getEnv("FILE_GRPC_PORT", "10057"),
		MinioEndpoint:       getEnv("MINIO_ENDPOINT", "localhost:19000"),
		MinioPublicEndpoint: getEnv("MINIO_PUBLIC_ENDPOINT", ""), // empty = same as MinioEndpoint
		MinioAccessKey:      getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinioSecretKey:      getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinioUseSSL:         getEnv("MINIO_USE_SSL", "false") == "true",
		MinioBucket:         getEnv("MINIO_BUCKET", "social-network"),
		AuthGrpcAddr:        getEnv("AUTH_GRPC_ADDR", "localhost:10051"),
	}
}
