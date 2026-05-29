package database

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gobench/pkg/config"
	"gobench/internal/model"
)

var DB *gorm.DB

func Init() error {
	if config.AppConfig == nil {
		return fmt.Errorf("config not initialized")
	}

	dsn := config.AppConfig.MySQL.DSN
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return fmt.Errorf("failed to connect to mysql: %w", err)
	}

	// Auto Migrate the schema
	if err := DB.AutoMigrate(&model.User{}, &model.Task{}); err != nil {
		return fmt.Errorf("failed to auto migrate: %w", err)
	}

	return nil
}
