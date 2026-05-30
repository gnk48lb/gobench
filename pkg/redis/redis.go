package redis

import (
	"context"
	"fmt"

	redisClient "github.com/redis/go-redis/v9"
	"gobench/pkg/config"
)

var Client *redisClient.Client

func Init() error {
	if config.AppConfig == nil {
		return fmt.Errorf("config not initialized")
	}

	Client = redisClient.NewClient(&redisClient.Options{
		Addr:     config.AppConfig.Redis.Addr,
		Password: config.AppConfig.Redis.Password,
		DB:       config.AppConfig.Redis.DB,
	})

	if err := Client.Ping(context.Background()).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	return nil
}
