package config

import (
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                   string
	AuthGrpcAddr           string
	AuthHttpAddr           string
	UserHttpAddr           string
	PostHttpAddr           string
	ChatHttpAddr           string
	ChatWsAddr             string
	NotificationHttpAddr   string
	FileHttpAddr           string
	AdminHttpAddr          string
	AIHttpAddr             string
	RedisAddr              string
	SearchHttpAddr         string
	StoryHttpAddr          string
	RecommendationHttpAddr string
	FCMGrpcAddr            string
	FCMHttpAddr            string
	AllowedOrigins         []string
}

func LoadConfig() *Config {
	_ = godotenv.Load()
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	rawOrigins := getEnv("CORS_ALLOWED_ORIGINS", "http://localhost:3000,http://localhost:3001,http://localhost:10000,https://pocpoc.online,https://www.pocpoc.online")
	var origins []string
	for _, p := range strings.Split(rawOrigins, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			origins = append(origins, p)
		}
	}

	return &Config{
		Port:                   getEnv("GATEWAY_PORT", "11111"), // Matching Spring Boot's port for seamless frontend compatibility
		AuthGrpcAddr:           getEnv("AUTH_GRPC_ADDR", "localhost:10051"),
		AuthHttpAddr:           getEnv("AUTH_HTTP_ADDR", "http://localhost:10081"),
		UserHttpAddr:           getEnv("USER_HTTP_ADDR", "http://localhost:10082"),
		PostHttpAddr:           getEnv("POST_HTTP_ADDR", "http://localhost:10083"),
		ChatHttpAddr:           getEnv("CHAT_HTTP_ADDR", "http://localhost:10084"),
		ChatWsAddr:             getEnv("CHAT_WS_ADDR", "ws://localhost:10084"),
		NotificationHttpAddr:   getEnv("NOTIFICATION_HTTP_ADDR", "http://localhost:10085"),
		FileHttpAddr:           getEnv("FILE_HTTP_ADDR", "http://localhost:10087"),
		AdminHttpAddr:          getEnv("ADMIN_HTTP_ADDR", "http://localhost:10088"),
		AIHttpAddr:             getEnv("AI_HTTP_ADDR", "http://localhost:10091"),
		RedisAddr:              getEnv("REDIS_ADDR", "localhost:6379"),
		SearchHttpAddr:         getEnv("SEARCH_HTTP_ADDR", "http://localhost:10089"),
		StoryHttpAddr:          getEnv("STORY_HTTP_ADDR", "http://localhost:10090"),
		RecommendationHttpAddr: getEnv("RECOMMENDATION_HTTP_ADDR", "http://localhost:10092"),
		FCMGrpcAddr:            getEnv("FCM_GRPC_ADDR", "localhost:10056"),
		FCMHttpAddr:            getEnv("FCM_HTTP_ADDR", "http://localhost:10086"),
		AllowedOrigins:         origins,
	}
}
