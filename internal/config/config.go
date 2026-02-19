package config

import (
	"os"
	"strconv"
	"sync"
	"time"
)

type Config struct {
	Server      ServerConfig
	Redis       RedisConfig
	ClickHouse  ClickHouseConfig
	Validation  ValidationConfig
	Learning    LearningConfig
	Positioning PositioningConfig
	Adaptive    AdaptiveConfig
	Filter      FilterConfig
	Metrics     MetricsConfig
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
	Addr     string
	Database string
	Username string
	Password string
}

type ValidationConfig struct {
	MaxSpeedKmH       float64
	MaxTimeDiff       time.Duration
	MinAccuracyMeters  float64
}

type LearningConfig struct {
	MinObservations   int
	VarianceThreshold float64
	TimeWindowHours   int
}

type PositioningConfig struct {
	RadiusWifiMeters       int
	RadiusBleMeters       int
	RadiusCellLacMeters   int
	RadiusCellAtcMeters   int
	MinSources            int
	DeviationThresholdMeters int
}

type AdaptiveConfig struct {
	AbsoluteThresholdBase int
	AbsoluteThresholdStep int
	DeviationThresholdBase int
	DeviationThresholdStep int
}

type FilterConfig struct {
	RssiChangeThreshold     int
	RssiChangeWindowSeconds int
}

type MetricsConfig struct {
	Enabled bool
	Port    string
}

type ConfigManager struct {
	config  *Config
	mu      sync.RWMutex
	history []ConfigChange
}

type ConfigChange struct {
	Key      string
	OldValue string
	NewValue string
	Reason   string
	ChangedAt time.Time
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		config: Load(),
		history: []ConfigChange{},
	}
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
			MaxSpeedKmH:      getEnvFloat("VALIDATION_MAX_SPEED_KMH", 150),
			MaxTimeDiff:      time.Duration(getEnvInt("VALIDATION_MAX_TIME_DIFF_HOURS", 12)) * time.Hour,
			MinAccuracyMeters: getEnvFloat("VALIDATION_MIN_ACCURACY_METERS", 100),
		},
		Learning: LearningConfig{
			MinObservations:   getEnvInt("LEARNING_MIN_OBSERVATIONS", 3),
			VarianceThreshold: getEnvFloat("LEARNING_VARIANCE_THRESHOLD", 0.0001),
			TimeWindowHours:   getEnvInt("LEARNING_TIME_WINDOW_HOURS", 24),
		},
		Positioning: PositioningConfig{
			RadiusWifiMeters:         getEnvInt("POSITIONING_RADIUS_WIFI_METERS", 50),
			RadiusBleMeters:          getEnvInt("POSITIONING_RADIUS_BLE_METERS", 15),
			RadiusCellLacMeters:      getEnvInt("POSITIONING_RADIUS_CELL_LAC_METERS", 3000),
			RadiusCellAtcMeters:      getEnvInt("POSITIONING_RADIUS_CELL_ATC_METERS", 300),
			MinSources:               getEnvInt("POSITIONING_MIN_SOURCES", 2),
			DeviationThresholdMeters: getEnvInt("POSITIONING_DEVIATION_THRESHOLD_METERS", 50),
		},
		Adaptive: AdaptiveConfig{
			AbsoluteThresholdBase:    getEnvInt("ADAPTIVE_ABSOLUTE_THRESHOLD_BASE", 50),
			AbsoluteThresholdStep:    getEnvInt("ADAPTIVE_ABSOLUTE_THRESHOLD_STEP", 10),
			DeviationThresholdBase:   getEnvInt("ADAPTIVE_DEVIATION_THRESHOLD_BASE", 100),
			DeviationThresholdStep:   getEnvInt("ADAPTIVE_DEVIATION_THRESHOLD_STEP", 20),
		},
		Filter: FilterConfig{
			RssiChangeThreshold:     getEnvInt("FILTER_RSSI_CHANGE_THRESHOLD", 10),
			RssiChangeWindowSeconds: getEnvInt("FILTER_RSSI_CHANGE_WINDOW_SECONDS", 60),
		},
		Metrics: MetricsConfig{
			Enabled: getEnvBool("METRICS_ENABLED", true),
			Port:    getEnv("METRICS_PORT", "9090"),
		},
	}
}

func (c *ConfigManager) Get() *Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.config
}

func (c *ConfigManager) Update(key, value, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldValue := ""
	
	switch key {
	case "validation.max_speed_kmh":
		oldValue = strconv.FormatFloat(c.config.Validation.MaxSpeedKmH, 'f', -1, 64)
		c.config.Validation.MaxSpeedKmH, _ = strconv.ParseFloat(value, 64)
	case "validation.max_time_diff_hours":
		hours, _ := strconv.Atoi(value)
		oldValue = strconv.Itoa(int(c.config.Validation.MaxTimeDiff.Hours()))
		c.config.Validation.MaxTimeDiff = time.Duration(hours) * time.Hour
	case "validation.min_accuracy_meters":
		oldValue = strconv.FormatFloat(c.config.Validation.MinAccuracyMeters, 'f', -1, 64)
		c.config.Validation.MinAccuracyMeters, _ = strconv.ParseFloat(value, 64)
	case "learning.min_observations":
		oldValue = strconv.Itoa(c.config.Learning.MinObservations)
		c.config.Learning.MinObservations, _ = strconv.Atoi(value)
	case "learning.variance_threshold":
		oldValue = strconv.FormatFloat(c.config.Learning.VarianceThreshold, 'f', -1, 64)
		c.config.Learning.VarianceThreshold, _ = strconv.ParseFloat(value, 64)
	case "learning.time_window_hours":
		oldValue = strconv.Itoa(c.config.Learning.TimeWindowHours)
		c.config.Learning.TimeWindowHours, _ = strconv.Atoi(value)
	case "positioning.radius_wifi_meters":
		oldValue = strconv.Itoa(c.config.Positioning.RadiusWifiMeters)
		c.config.Positioning.RadiusWifiMeters, _ = strconv.Atoi(value)
	case "positioning.radius_ble_meters":
		oldValue = strconv.Itoa(c.config.Positioning.RadiusBleMeters)
		c.config.Positioning.RadiusBleMeters, _ = strconv.Atoi(value)
	case "positioning.radius_cell_lac_meters":
		oldValue = strconv.Itoa(c.config.Positioning.RadiusCellLacMeters)
		c.config.Positioning.RadiusCellLacMeters, _ = strconv.Atoi(value)
	case "positioning.min_sources":
		oldValue = strconv.Itoa(c.config.Positioning.MinSources)
		c.config.Positioning.MinSources, _ = strconv.Atoi(value)
	case "positioning.deviation_threshold_meters":
		oldValue = strconv.Itoa(c.config.Positioning.DeviationThresholdMeters)
		c.config.Positioning.DeviationThresholdMeters, _ = strconv.Atoi(value)
	default:
		return nil
	}

	c.history = append(c.history, ConfigChange{
		Key:      key,
		OldValue: oldValue,
		NewValue: value,
		Reason:   reason,
		ChangedAt: time.Now(),
	})

	return nil
}

func (c *ConfigManager) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config = Load()
	c.history = append(c.history, ConfigChange{
		Key:      "ALL",
		OldValue: "custom",
		NewValue: "default",
		Reason:   "Reset to defaults",
		ChangedAt: time.Now(),
	})
}

func (c *ConfigManager) GetHistory() []ConfigChange {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.history
}

func (c *ConfigManager) GetAdaptiveThreshold(base, step, sourceCount int) int {
	return base + (sourceCount-1)*step
}

// Helper functions
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

func getEnvFloat(key string, defaultValue float64) float64 {
	if value, exists := os.LookupEnv(key); exists {
		if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
			return floatValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}
