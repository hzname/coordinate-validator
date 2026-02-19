package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"

	"coordinate-validator/internal/config"
)

type RedisCache struct {
	client *redis.Client
}

type WifiPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
}

type CellPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
}

type BluetoothPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
}

type LastKnownLocation struct {
	Lat   float64   `json:"lat"`
	Lon   float64   `json:"lon"`
	Time  time.Time `json:"time"`
}

func NewRedisCache(cfg config.RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}

// WiFi methods
func (r *RedisCache) GetWifiPoint(ctx context.Context, bssid string) (*WifiPoint, error) {
	key := fmt.Sprintf("wifi:%s", bssid)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var point WifiPoint
	if err := json.Unmarshal(data, &point); err != nil {
		return nil, err
	}
	return &point, nil
}

func (r *RedisCache) SetWifiPoint(ctx context.Context, bssid string, point *WifiPoint) error {
	key := fmt.Sprintf("wifi:%s", bssid)
	data, err := json.Marshal(point)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// Cell tower methods
func (r *RedisCache) GetCellPoint(ctx context.Context, cellID string, lac int) (*CellPoint, error) {
	key := fmt.Sprintf("cell:%s:%d", cellID, lac)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var point CellPoint
	if err := json.Unmarshal(data, &point); err != nil {
		return nil, err
	}
	return &point, nil
}

func (r *RedisCache) SetCellPoint(ctx context.Context, cellID string, lac int, point *CellPoint) error {
	key := fmt.Sprintf("cell:%s:%d", cellID, lac)
	data, err := json.Marshal(point)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// Bluetooth methods
func (r *RedisCache) GetBluetoothPoint(ctx context.Context, mac string) (*BluetoothPoint, error) {
	key := fmt.Sprintf("bt:%s", mac)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var point BluetoothPoint
	if err := json.Unmarshal(data, &point); err != nil {
		return nil, err
	}
	return &point, nil
}

func (r *RedisCache) SetBluetoothPoint(ctx context.Context, mac string, point *BluetoothPoint) error {
	key := fmt.Sprintf("bt:%s", mac)
	data, err := json.Marshal(point)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// Last known location for device
func (r *RedisCache) GetLastKnownLocation(ctx context.Context, deviceID string) (*LastKnownLocation, error) {
	key := fmt.Sprintf("device:%s:last_known", deviceID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var loc LastKnownLocation
	if err := json.Unmarshal(data, &loc); err != nil {
		return nil, err
	}
	return &loc, nil
}

func (r *RedisCache) SetLastKnownLocation(ctx context.Context, deviceID string, loc *LastKnownLocation) error {
	key := fmt.Sprintf("device:%s:last_known", deviceID)
	data, err := json.Marshal(loc)
	if err != nil {
		return err
	}
	// Keep for 7 days
	return r.client.Set(ctx, key, data, 7*24*time.Hour).Err()
}
