package redis

import (
	"context"
	"social-network-go/logger"
	"social-network-go/user-service/config"

	"github.com/redis/go-redis/v9"
)

var RedisClient *redis.Client

func InitRedis(cfg *config.Config) *redis.Client {
	RedisClient = redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})

	ctx := context.Background()
	_, err := RedisClient.Ping(ctx).Result()
	if err != nil {
		logger.Err(err).Warn("Failed to connect to Redis for user-service")
	} else {
		logger.Info("Successfully connected to Redis for user-service")
	}

	return RedisClient
}
