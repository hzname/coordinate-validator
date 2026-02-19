package storage

import (
	"context"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"coordinate-validator/internal/config"
)

type ClickHouseStorage struct {
	conn driver.Conn
	cfg  config.ClickHouseConfig
}

type CoordinateRecord struct {
	DeviceID           string
	Latitude           float64
	Longitude          float64
	Accuracy           float32
	Timestamp          time.Time
	HasWifi            bool
	HasBluetooth       bool
	HasCell            bool
	ValidationResult   string
	Confidence         float32
}

func NewClickHouseStorage(cfg config.ClickHouseConfig) (*ClickHouseStorage, error) {
	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("clickhouse connection failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("clickhouse ping failed: %w", err)
	}

	storage := &ClickHouseStorage{
		conn: conn,
		cfg:  cfg,
	}

	if err := storage.createTables(ctx); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	return storage, nil
}

func (s *ClickHouseStorage) Close() error {
	return s.conn.Close()
}

func (s *ClickHouseStorage) createTables(ctx context.Context) error {
	// Create database if not exists
	queries := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", s.cfg.Database),
		// Main coordinate requests table
		fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.coordinate_requests (
				device_id String,
				latitude Float64,
				longitude Float64,
				accuracy Float32,
				timestamp DateTime64,
				has_wireless UInt8,
				has_bluetooth UInt8,
				has_cell UInt8,
				validation_result Enum8('valid'=0, 'invalid'=1, 'uncertain'=2),
				confidence Float32,
				created_at DateTime DEFAULT now()
			) ENGINE = MergeTree()
			PARTITION BY toYYYYMM(created_at)
			ORDER BY (device_id, timestamp)
		`, s.cfg.Database),

		// Point statistics table (for self-learning)
		fmt.Sprintf(`
			CREATE TABLE IF NOT EXISTS %s.point_stats (
				point_type Enum8('wifi'=0, 'cell'=1, 'bt'=2),
				point_id String,
				latitude Float64,
				longitude Float64,
				observations UInt32,
				last_observed DateTime,
				accuracy Float32
			) ENGINE = MergeTree()
			ORDER BY (point_type, point_id)
		`, s.cfg.Database),
	}

	for _, query := range queries {
		if err := s.conn.Exec(ctx, query); err != nil {
			return err
		}
	}

	return nil
}

func (s *ClickHouseStorage) InsertCoordinate(ctx context.Context, record CoordinateRecord) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.coordinate_requests (
			device_id, latitude, longitude, accuracy, timestamp,
			has_wireless, has_bluetooth, has_cell,
			validation_result, confidence
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, s.cfg.Database)

	return s.conn.Exec(ctx, query,
		record.DeviceID,
		record.Latitude,
		record.Longitude,
		record.Accuracy,
		record.Timestamp,
		record.HasWifi,
		record.HasBluetooth,
		record.HasCell,
		record.ValidationResult,
		record.Confidence,
	)
}

// Self-learning: update point statistics
func (s *ClickHouseStorage) UpdatePointStats(ctx context.Context, pointType, pointID string, lat, lon float64, accuracy float32) error {
	query := fmt.Sprintf(`
		INSERT INTO %s.point_stats (
			point_type, point_id, latitude, longitude,
			observations, last_observed, accuracy
		) VALUES (?, ?, ?, ?, 1, now(), ?)
	`, s.cfg.Database)

	return s.conn.Exec(ctx, query,
		pointType,
		pointID,
		lat,
		lon,
		accuracy,
	)
}

// Get historical coordinates for device
func (s *ClickHouseStorage) GetDeviceHistory(ctx context.Context, deviceID string, limit int) ([]CoordinateRecord, error) {
	query := fmt.Sprintf(`
		SELECT device_id, latitude, longitude, accuracy, timestamp,
			   has_wireless, has_bluetooth, has_cell,
			   validation_result, confidence
		FROM %s.coordinate_requests
		WHERE device_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, s.cfg.Database)

	rows, err := s.conn.Query(ctx, query, deviceID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CoordinateRecord
	for rows.Next() {
		var r CoordinateRecord
		var result string
		err := rows.Scan(
			&r.DeviceID, &r.Latitude, &r.Longitude, &r.Accuracy, &r.Timestamp,
			&r.HasWifi, &r.HasBluetooth, &r.HasCell,
			&result, &r.Confidence,
		)
		if err != nil {
			return nil, err
		}
		r.ValidationResult = result
		records = append(records, r)
	}

	return records, rows.Err()
}
