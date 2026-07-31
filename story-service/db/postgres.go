package db

import (
	"fmt"
	"regexp"

	"social-network-go/logger"
	"social-network-go/story-service/config"
	"social-network-go/story-service/model"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var SQLDB *gorm.DB

func ensureDatabaseExists(dsn string) {
	re := regexp.MustCompile(`dbname=([^\s]+)`)
	matches := re.FindStringSubmatch(dsn)
	if len(matches) < 2 {
		return
	}
	dbName := matches[1]

	postgresDSN := re.ReplaceAllString(dsn, "dbname=postgres")
	sysDB, err := gorm.Open(postgres.Open(postgresDSN), &gorm.Config{})
	if err != nil {
		return
	}
	sqlInstance, _ := sysDB.DB()
	if sqlInstance != nil {
		defer sqlInstance.Close()
	}

	var count int64
	sysDB.Raw("SELECT count(*) FROM pg_database WHERE datname = ?", dbName).Scan(&count)
	if count == 0 {
		sysDB.Exec(fmt.Sprintf("CREATE DATABASE %s;", dbName))
		logger.Info("Created PostgreSQL Database: %s", dbName)
	}
}

func InitPostgres(cfg *config.Config) *gorm.DB {
	ensureDatabaseExists(cfg.PostgresDSN)

	var err error
	SQLDB, err = gorm.Open(postgres.Open(cfg.PostgresDSN), &gorm.Config{})
	if err != nil {
		logger.Warn("Warning: Failed to connect to PostgreSQL in Story Service: %v", err)
		return nil
	}

	logger.Info("Successfully connected to PostgreSQL Database (Story Service)")

	// Auto Migrate schemas for Story Service entities
	err = SQLDB.AutoMigrate(
		&model.StoryEntity{},
		&model.StoryViewEntity{},
	)
	if err != nil {
		logger.Warn("Warning: Failed to auto-migrate PostgreSQL schemas in Story Service: %v", err)
	} else {
		logger.Info("PostgreSQL database schemas migrated successfully (Story Service)")
	}

	return SQLDB
}
