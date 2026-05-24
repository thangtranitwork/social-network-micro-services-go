package db

import (
	"social-network-go/admin-service/config"
	"social-network-go/logger"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
)

var (
	Neo4jDriver neo4j.DriverWithContext
	RedisClient *redis.Client
)

func InitDB(cfg *config.Config) {
	// Initialize Neo4j Driver
	driver, err := neo4j.NewDriverWithContext(cfg.Neo4jURI, neo4j.BasicAuth(cfg.Neo4jUser, cfg.Neo4jPass, ""))
	if err != nil {
		logger.Warn("Warning: Failed to connect to Neo4j at %s: %v", cfg.Neo4jURI, err)
	} else {
		Neo4jDriver = driver
		logger.Info("Admin-Service connected to Neo4j successfully")
	}

	// Initialize Redis Client
	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPass,
		DB:       0,
	})
	RedisClient = rdb
	logger.Info("Admin-Service initialized Redis client on address %s", cfg.RedisAddr)
}
