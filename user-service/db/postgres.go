package db

import (
	"fmt"
	"regexp"

	"social-network-go/logger"
	"social-network-go/user-service/config"
	"social-network-go/user-service/model"

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
		logger.Warn("Warning: Failed to connect to PostgreSQL in User Service: %v", err)
		return nil
	}

	logger.Info("Successfully connected to PostgreSQL Database (User Service)")

	// Auto Migrate schemas for User Service entities
	err = SQLDB.AutoMigrate(
		&model.UserEntity{},
		&model.FriendEntity{},
		&model.FriendRequestEntity{},
		&model.UserBlockEntity{},
	)
	if err != nil {
		logger.Warn("Warning: Failed to auto-migrate PostgreSQL schemas in User Service: %v", err)
	} else {
		logger.Info("PostgreSQL database schemas migrated successfully (User Service)")
	}

	return SQLDB
}
