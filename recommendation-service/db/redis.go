package db

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"social-network-go/logger"
	"social-network-go/recommendation-service/config"
)

var RedisClient *redis.Client

func InitRedis(cfg *config.Config) *redis.Client {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Warn("Warning: Failed to connect to Redis in recommendation-service: %v", err)
	} else {
		logger.Info("Successfully connected to Redis (recommendation-service)")
	}
	RedisClient = client
	return client
}
