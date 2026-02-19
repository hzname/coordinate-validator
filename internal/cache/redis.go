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

// EGTS data structures
type WifiPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	Count     int       `json:"count"`
	// EGTS fields
	SSID      string    `json:"ssid,omitempty"`
	EID       int32     `json:"eid,omitempty"` // Inverted RSSI
}

type CellPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	// EGTS fields
	LAC       uint32    `json:"lac,omitempty"`
	MCC       uint32    `json:"mcc,omitempty"`
	MNC       uint32    `json:"mnc,omitempty"`
	EID       int32     `json:"eid,omitempty"` // Inverted RSSI
}

type BluetoothPoint struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	EID       int32     `json:"eid,omitempty"` // Inverted RSSI
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

// ============ WiFi methods (EGTS_ENVELOPE_LOW, type=0) ============

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

// ============ Cell tower methods (EGTS_ENVELOPE_HIGHT) ============

func (r *RedisCache) GetCellPoint(ctx context.Context, cellID uint32, lac uint32) (*CellPoint, error) {
	key := fmt.Sprintf("cell:%d:%d", cellID, lac)
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

func (r *RedisCache) SetCellPoint(ctx context.Context, cellID uint32, lac uint32, point *CellPoint) error {
	key := fmt.Sprintf("cell:%d:%d", cellID, lac)
	data, err := json.Marshal(point)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// Get cell by LAC+MCC+MNC (broader search)
func (r *RedisCache) GetCellByLAC(ctx context.Context, lac, mcc, mnc uint32) ([]CellPoint, error) {
	pattern := fmt.Sprintf("cell:*:%d", lac)
	keys, err := r.client.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	var points []CellPoint
	for _, key := range keys {
		data, err := r.client.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		var point CellPoint
		if err := json.Unmarshal(data, &point); err != nil {
			continue
		}
		if point.MCC == mcc && point.MNC == mnc {
			points = append(points, point)
		}
	}
	return points, nil
}

// ============ Bluetooth methods (EGTS_ENVELOPE_LOW, type=1) ============

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

// ============ Device last known location ============

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
	return r.client.Set(ctx, key, data, 7*24*time.Hour).Err()
}

// ============ EGTS helper: convert EID to RSSI ============

// EID in EGTS is inverted RSSI (* -1), need to convert back
// RSSI = -EID (or EID + 128 for some cases)
func ConvertEIDToRSSI(eid int32) int32 {
	rssi := -eid
	if rssi < -128 {
		rssi += 128 // Handle offset
	}
	return rssi
}

// RSSI in EGTS comes as offset (+128), convert to normal
func ConvertEGTSRSSI(rssi byte) int32 {
	return int32(int8(rssi)) // Convert from offset
}
