package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	MySQL    MySQLConfig    `mapstructure:"mysql"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Redis    RedisConfig    `mapstructure:"redis"`
	Worker   WorkerConfig   `mapstructure:"worker"`
	Executor ExecutorConfig `mapstructure:"executor"`
	Tracing  TracingConfig  `mapstructure:"tracing"`
}

type ServerConfig struct {
	Port int `mapstructure:"port"`
}

type MySQLConfig struct {
	DSN string `mapstructure:"dsn"`
}

type JWTConfig struct {
	Secret string `mapstructure:"secret"`
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type WorkerConfig struct {
	QueueKey    string `mapstructure:"queue_key"`
	Concurrency int    `mapstructure:"concurrency"`
}

type ExecutorConfig struct {
	Addr string `mapstructure:"addr"` // executor-service gRPC 地址，如 "localhost:9000"
}

type TracingConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	OTLPEndpoint string `mapstructure:"otlp_endpoint"` // 如 "localhost:4317"
	ServiceName  string `mapstructure:"service_name"`
}

var AppConfig *Config

func Init(configFile string) error {
	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")

	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	// 显式绑定关键字段，保证 Unmarshal 时能读到环境变量
	_ = viper.BindEnv("mysql.dsn", "MYSQL_DSN")
	_ = viper.BindEnv("jwt.secret", "JWT_SECRET")
	_ = viper.BindEnv("redis.addr", "REDIS_ADDR")
	_ = viper.BindEnv("redis.password", "REDIS_PASSWORD")
	_ = viper.BindEnv("server.port", "SERVER_PORT")
	_ = viper.BindEnv("worker.concurrency", "WORKER_CONCURRENCY")
	_ = viper.BindEnv("executor.addr", "EXECUTOR_ADDR")
	_ = viper.BindEnv("tracing.enabled", "TRACING_ENABLED")
	_ = viper.BindEnv("tracing.otlp_endpoint", "TRACING_OTLP_ENDPOINT")
	_ = viper.BindEnv("tracing.service_name", "TRACING_SERVICE_NAME")

	if err := viper.ReadInConfig(); err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	AppConfig = &Config{}
	if err := viper.Unmarshal(AppConfig); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return nil
}
