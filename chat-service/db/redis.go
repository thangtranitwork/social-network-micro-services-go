package db

import (
	"context"
	"time"

	"social-network-go/chat-service/config"
	"social-network-go/logger"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedis(cfg *config.Config) {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		logger.Warn("Warning: Failed to connect to Redis for chat-service: %v", err)
	} else {
		logger.Info("Successfully connected to Redis for chat-service")
	}
}
