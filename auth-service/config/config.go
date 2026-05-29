package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	GRPCPort              string
	HTTPPort              string
	PostgresDSN           string
	RedisAddr             string
	RedisPass             string
	JWTSecret             string
	JWTRefreshSecret      string
	AccessTokenDuration   time.Duration
	RefreshTokenDuration  time.Duration
	VerifyEmailDuration   time.Duration
	UserGRPCAddr          string
	Mail                  string
	MailPassword          string
	KafkaAddr             string
	FrontendURL           string
	PasswordResetDuration time.Duration
	EmailRateLimitCount   int
	EmailRateLimitWindow  time.Duration
}

func LoadConfig() *Config {
	_ = godotenv.Load("auth-service/.env")
	_ = godotenv.Load(".env")
	_ = godotenv.Load("../.env")
	getEnv := func(key, fallback string) string {
		if val, ok := os.LookupEnv(key); ok {
			return val
		}
		return fallback
	}

	emailLimitCountStr := getEnv("EMAIL_RATE_LIMIT_COUNT", "10")
	emailLimitCount := 10
	if val, err := strconv.Atoi(emailLimitCountStr); err == nil {
		emailLimitCount = val
	}

	emailLimitWindowStr := getEnv("EMAIL_RATE_LIMIT_WINDOW", "1h")
	emailLimitWindow, err := time.ParseDuration(emailLimitWindowStr)
	if err != nil {
		emailLimitWindow = time.Hour
	}

	return &Config{
		GRPCPort:              getEnv("AUTH_GRPC_PORT", "10051"),
		HTTPPort:              getEnv("AUTH_HTTP_PORT", "10081"),
		PostgresDSN:           getEnv("POSTGRES_DSN", "host=localhost user=postgres password=postgres dbname=auth_db port=5432 sslmode=disable"),
		UserGRPCAddr:          getEnv("USER_GRPC_ADDR", "localhost:10052"),
		RedisAddr:             getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPass:             getEnv("REDIS_PASSWORD", ""),
		JWTSecret:             getEnv("JWT_ACCESS_TOKEN_KEY", "your-access-secret-key-very-long-and-secure"),
		JWTRefreshSecret:      getEnv("JWT_REFRESH_TOKEN_KEY", "your-refresh-secret-key-very-long-and-secure"),
		AccessTokenDuration:   15 * time.Minute,
		RefreshTokenDuration:  7 * 24 * time.Hour,
		VerifyEmailDuration:   24 * time.Hour,
		Mail:                  getEnv("MAIL", ""),
		MailPassword:          getEnv("MAIL_PASSWORD", ""),
		KafkaAddr:             getEnv("KAFKA_ADDR", "localhost:9092"),
		FrontendURL:           getEnv("FRONTEND_URL", "http://localhost:10000"),
		PasswordResetDuration: 15 * time.Minute,
		EmailRateLimitCount:   emailLimitCount,
		EmailRateLimitWindow:  emailLimitWindow,
	}
}
