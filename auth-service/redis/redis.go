package redis

import (
	"context"
	"social-network-go/auth-service/config"
	"social-network-go/logger"

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
		logger.Field("addr", cfg.RedisAddr).Field("error", err).Warn("Failed to connect to Redis")
	} else {
		logger.Info("Successfully connected to Redis")
	}

	return RedisClient
}
