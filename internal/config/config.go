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
	Kafka    KafkaConfig
	Validation ValidationConfig
}

type ServerConfig struct {
	Port            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	RefinementAddr  string
	LearningAddr    string
	StorageAddr     string
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
	BatchSize    int
	FlushInterval time.Duration
}

type KafkaConfig struct {
	Brokers      []string
	RefinementTopic string
	LearningTopic  string
	ProducerID    string
}

type ValidationConfig struct {
	MaxSpeedKmH    float64
	MaxTimeDiff    time.Duration
	ConfidenceThresholds ConfidenceThresholds
}

type ConfidenceThresholds struct {
	High   float64
	Medium float64
	Low    float64
}

func Load() *Config {
	return &Config{
		Server: ServerConfig{
			Port:            getEnv("SERVER_PORT", "50050"),
			ReadTimeout:     getDurationEnv("SERVER_READ_TIMEOUT", 30*time.Second),
			WriteTimeout:    getDurationEnv("SERVER_WRITE_TIMEOUT", 30*time.Second),
			RefinementAddr:  getEnv("REFINEMENT_ADDR", "localhost:50051"),
			LearningAddr:    getEnv("LEARNING_ADDR", "localhost:50052"),
			StorageAddr:     getEnv("STORAGE_ADDR", "localhost:50053"),
		},
		Redis: RedisConfig{
			Addr:     getEnv("REDIS_ADDR", "localhost:6379"),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getIntEnv("REDIS_DB", 0),
			PoolSize: getIntEnv("REDIS_POOL_SIZE", 10),
		},
		ClickHouse: ClickHouseConfig{
			Addr:         getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
			Database:     getEnv("CLICKHOUSE_DB", "coordinates"),
			Username:     getEnv("CLICKHOUSE_USER", "default"),
			Password:     getEnv("CLICKHOUSE_PASSWORD", ""),
			BatchSize:    getIntEnv("CLICKHOUSE_BATCH_SIZE", 1000),
			FlushInterval: getDurationEnv("CLICKHOUSE_FLUSH_INTERVAL", 5*time.Second),
		},
		Kafka: KafkaConfig{
			Brokers:        getEnvSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			RefinementTopic: getEnv("KAFKA_REFINEMENT_TOPIC", "coord-validation"),
			LearningTopic:  getEnv("KAFKA_LEARNING_TOPIC", "coord-learning"),
			ProducerID:     getEnv("KAFKA_PRODUCER_ID", "validator"),
		},
		Validation: ValidationConfig{
			MaxSpeedKmH: getFloatEnv("MAX_SPEED_KMH", 150.0),
			MaxTimeDiff: getDurationEnv("MAX_TIME_DIFF", 12*time.Hour),
			ConfidenceThresholds: ConfidenceThresholds{
				High:   0.8,
				Medium: 0.5,
				Low:    0.3,
			},
		},
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return defaultValue
}

func getFloatEnv(key string, defaultValue float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if v := os.Getenv(key); v != "" {
		return []string{v}
	}
	return defaultValue
}
