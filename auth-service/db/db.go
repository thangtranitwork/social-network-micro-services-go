package db

import (
	"os"
	"social-network-go/auth-service/config"
	"social-network-go/auth-service/model"
	"social-network-go/logger"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func InitDB(cfg *config.Config) *gorm.DB {
	var err error
	DB, err = gorm.Open(postgres.Open(cfg.PostgresDSN), &gorm.Config{})
	if err != nil {
		logger.Error("Failed to connect to PostgreSQL database: %v", err)
		os.Exit(1)
	}

	logger.Info("Successfully connected to PostgreSQL")

	// Auto Migrate the schemas
	err = DB.AutoMigrate(&model.Account{})
	if err != nil {
		logger.Error("Failed to auto-migrate database schemas: %v", err)
		os.Exit(1)
	}

	logger.Info("Database schemas migrated successfully")
	return DB
}
