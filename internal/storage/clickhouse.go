package storage

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "github.com/clickhouse/clickhouse-go/v2"

	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
)

type ClickHouseStorage struct {
	db    *sql.DB
	cfg   *config.ClickHouseConfig
	mu    sync.Mutex
	queue []model.ValidationRecord
}

func NewClickHouseStorage(cfg *config.ClickHouseConfig) (*ClickHouseStorage, error) {
	dsn := fmt.Sprintf("clickhouse://%s:%s@%s/%s",
		cfg.Username,
		cfg.Password,
		cfg.Addr,
		cfg.Database,
	)

	db, err := sql.Open("clickhouse", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse: %w", err)
	}

	s := &ClickHouseStorage{
		db:    db,
		cfg:   cfg,
		queue: make([]model.ValidationRecord, 0, cfg.BatchSize),
	}

	// Create tables if not exist
	if err := s.createTables(ctx); err != nil {
		return nil, fmt.Errorf("failed to create tables: %w", err)
	}

	// Start background flusher
	go s.flusher()

	return s, nil
}

func (s *ClickHouseStorage) createTables(ctx context.Context) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS validation_requests (
			device_id String,
			latitude Float64,
			longitude Float64,
			accuracy Float32,
			timestamp Int64,
			has_wift Bool,
			has_bt Bool,
			has_cell Bool,
			result String,
			confidence Float32,
			flow_type String,
			insert_time DateTime
		) ENGINE = MergeTree()
		ORDER BY (device_id, timestamp)`,

		`CREATE TABLE IF NOT EXISTS source_stats (
			type String,
			point_id String,
			latitude Float64,
			longitude Float64,
			observations Int64,
			last_updated DateTime
		) ENGINE = MergeTree()
		ORDER BY (type, point_id)`,
	}

	for _, q := range queries {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return err
		}
	}

	return nil
}

func (s *ClickHouseStorage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Flush remaining
	if len(s.queue) > 0 {
		s.flushLocked()
	}

	return s.db.Close()
}

// ============================================
// Queue Operations
// ============================================

func (s *ClickHouseStorage) QueueValidation(record model.ValidationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queue = append(s.queue, record)

	if len(s.queue) >= s.cfg.BatchSize {
		s.flushLocked()
	}
}

func (s *ClickHouseStorage) QueueValidations(records []model.ValidationRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.queue = append(s.queue, records...)

	if len(s.queue) >= s.cfg.BatchSize {
		s.flushLocked()
	}
}

// ============================================
// Background Flusher
// ============================================

func (s *ClickHouseStorage) flusher() {
	ticker := time.NewTicker(s.cfg.FlushInterval)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		if len(s.queue) > 0 {
			s.flushLocked()
		}
		s.mu.Unlock()
	}
}

func (s *ClickHouseStorage) flushLocked() {
	if len(s.queue) == 0 {
		return
	}

	records := s.queue
	s.queue = make([]model.ValidationRecord, 0, s.cfg.BatchSize)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Batch insert
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("[Storage] Failed to begin tx: %v", err)
		return
	}

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO validation_requests (
			device_id, latitude, longitude, accuracy, timestamp,
			has_wift, has_bt, has_cell, result, confidence, flow_type, insert_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("[Storage] Failed to prepare: %v", err)
		tx.Rollback()
		return
	}
	defer stmt.Close()

	for _, r := range records {
		_, err := stmt.ExecContext(ctx,
			r.DeviceID, r.Latitude, r.Longitude, r.Accuracy, r.Timestamp,
			r.HasWifi, r.HasBT, r.HasCell, string(r.Result), r.Confidence,
			r.FlowType, r.InsertTime,
		)
		if err != nil {
			log.Printf("[Storage] Failed to insert: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[Storage] Failed to commit: %v", err)
	}
}

// ============================================
// Sync Operations (for critical data)
// ============================================

func (s *ClickHouseStorage) InsertValidationSync(ctx context.Context, record model.ValidationRecord) error {
	record.InsertTime = time.Now()

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO validation_requests (
			device_id, latitude, longitude, accuracy, timestamp,
			has_wift, has_bt, has_cell, result, confidence, flow_type, insert_time
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		record.DeviceID, record.Latitude, record.Longitude, record.Accuracy, record.Timestamp,
		record.HasWifi, record.HasBT, record.HasCell, string(record.Result), record.Confidence,
		record.FlowType, record.InsertTime,
	)

	return err
}
