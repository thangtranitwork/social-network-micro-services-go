package db

import (
	"social-network-go/admin-service/config"
	"social-network-go/admin-service/model"
	"social-network-go/logger"

	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var (
	Neo4jDriver neo4j.DriverWithContext
	RedisClient *redis.Client
	PostgresDB  *gorm.DB
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

	// Initialize Postgres DB
	pgDB, err := gorm.Open(postgres.Open(cfg.PostgresDSN), &gorm.Config{})
	if err != nil {
		logger.Warn("Warning: Failed to connect to PostgreSQL database: %v", err)
	} else {
		PostgresDB = pgDB
		logger.Info("Admin-Service connected to PostgreSQL successfully")

		// Auto Migrate Admin-owned PostgreSQL tables.
		err = pgDB.AutoMigrate(
			&model.AdCampaign{},
			&model.AdInteraction{},
			&model.ModerationQueueRecord{},
			&model.ModerationReportRecord{},
			&model.ModerationAuditRecord{},
		)
		if err != nil {
			logger.Error("Failed to auto-migrate admin PostgreSQL tables: %v", err)
		} else {
			logger.Info("Successfully auto-migrated admin PostgreSQL tables")
		}
	}
}
