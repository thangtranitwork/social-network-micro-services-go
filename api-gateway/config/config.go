package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	AuthGrpcAddr   string
	AuthHttpAddr   string
	UserHttpAddr   string
	PostHttpAddr   string
	ChatHttpAddr   string
	ChatWsAddr     string
	NotificationHttpAddr string
	FileHttpAddr   string
	AdminHttpAddr  string
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
		Port:           getEnv("GATEWAY_PORT", "2003"), // Matching Spring Boot's port for seamless frontend compatibility
		AuthGrpcAddr:   getEnv("AUTH_GRPC_ADDR", "localhost:50051"),
		AuthHttpAddr:   getEnv("AUTH_HTTP_ADDR", "http://localhost:8081"),
		UserHttpAddr:   getEnv("USER_HTTP_ADDR", "http://localhost:8082"),
		PostHttpAddr:   getEnv("POST_HTTP_ADDR", "http://localhost:8083"),
		ChatHttpAddr:   getEnv("CHAT_HTTP_ADDR", "http://localhost:8084"),
		ChatWsAddr:     getEnv("CHAT_WS_ADDR", "ws://localhost:8084"),
		NotificationHttpAddr: getEnv("NOTIFICATION_HTTP_ADDR", "http://localhost:8085"),
		FileHttpAddr:   getEnv("FILE_HTTP_ADDR", "http://localhost:8087"),
		AdminHttpAddr:  getEnv("ADMIN_HTTP_ADDR", "http://localhost:8088"),
	}
}
