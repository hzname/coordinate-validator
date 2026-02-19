package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server   ServerConfig
	Redis    RedisConfig
	ClickHouse ClickHouseConfig
	Validation ValidationConfig
}

type ServerConfig struct {
	Port         string
	MaxBatchSize int
}

type RedisConfig struct {
	Addr         string
	Password     string
	DB           int
	PoolSize     int
}

type ClickHouseConfig struct {
	Addr         string
	Database     string
	Username     string
	Password     string
}

type ValidationConfig struct {
	MaxSpeedKmH     float64   // 150 km/h
	MaxTimeDiff    time.Duration // 12 hours
	WifiWeight      float32
	CellWeight      float32
	BluetoothWeight float32
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         getEnv("SERVER_PORT", "50051"),
			MaxBatchSize: getEnvInt("MAX_BATCH_SIZE", 100),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
			PoolSize: getEnvInt("REDIS_POOL_SIZE", 100),
		},
		ClickHouse: ClickHouseConfig{
			Addr:     getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
			Database: getEnv("CLICKHOUSE_DB", "coordinates"),
			Username: getEnv("CLICKHOUSE_USER", "default"),
			Password: getEnv("CLICKHOUSE_PASSWORD", ""),
		},
		Validation: ValidationConfig{
			MaxSpeedKmH:     150.0,
			MaxTimeDiff:     12 * time.Hour,
			WifiWeight:      0.4,
			CellWeight:      0.3,
			BluetoothWeight: 0.3,
		},
	}
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
