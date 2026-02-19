package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/go-redis/redis/v8"

	"coordinate-validator/internal/config"
)

type RedisCache struct {
	client *redis.Client
	cfg    *config.ConfigManager
}

// Observation for stationary detection
type Observation struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	Time     time.Time `json:"time"`
}

type SourceObservation struct {
	ObjectID        string        `json:"object_id"`
	SourceType      string        `json:"source_type"` // wifi, cell, ble
	SourceID        string        `json:"source_id"`
	Observations    []Observation `json:"observations"`
	Count           int           `json:"count"`
	Status          string        `json:"status"` // NEW, STATIONARY, RANDOM
	FirstSeen       time.Time     `json:"first_seen"`
	LastSeen        time.Time     `json:"last_seen"`
}

type WifiPoint struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	LastSeen time.Time `json:"last_seen"`
	SSID     string    `json:"ssid,omitempty"`
	EID      int32     `json:"eid,omitempty"`
}

type CellPoint struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	LastSeen time.Time `json:"last_seen"`
	LAC      uint32    `json:"lac,omitempty"`
	MCC      uint32    `json:"mcc,omitempty"`
	MNC      uint32    `json:"mnc,omitempty"`
	EID      int32     `json:"eid,omitempty"`
}

type BluetoothPoint struct {
	Lat      float64   `json:"lat"`
	Lon      float64   `json:"lon"`
	LastSeen time.Time `json:"last_seen"`
	EID      int32     `json:"eid,omitempty"`
}

type LastKnownLocation struct {
	Lat  float64   `json:"lat"`
	Lon  float64   `json:"lon"`
	Time time.Time `json:"time"`
}

type CalculatedCoordinates struct {
	Lat          float64   `json:"lat"`
	Lon          float64   `json:"lon"`
	Confidence   float32   `json:"confidence"`
	Observations int       `json:"observations"`
	CalculatedAt time.Time `json:"calculated_at"`
}

type AbsoluteCoordinates struct {
	Lat       float64   `json:"lat"`
	Lon       float64   `json:"lon"`
	Accuracy  float32   `json:"accuracy"`
	Source    string    `json:"source"`
	Timestamp time.Time `json:"timestamp"`
	ExpiresAt time.Time `json:"expires_at"`
}

func NewRedisCache(cfg *config.ConfigManager) (*RedisCache, error) {
	redisCfg := cfg.Get().Redis
	client := redis.NewClient(&redis.Options{
		Addr:     redisCfg.Addr,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
		PoolSize: redisCfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client, cfg: cfg}, nil
}

func (r *RedisCache) Close() error {
	return r.client.Close()
}

// ============================================
// Observation Management (for stationary detection)
// ============================================

func (r *RedisCache) GetObservation(ctx context.Context, objectID, sourceType, sourceID string) (*SourceObservation, error) {
	key := fmt.Sprintf("observation:%s:%s:%s", objectID, sourceType, sourceID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var obs SourceObservation
	if err := json.Unmarshal(data, &obs); err != nil {
		return nil, err
	}
	return &obs, nil
}

func (r *RedisCache) AddObservation(ctx context.Context, objectID, sourceType, sourceID string, lat, lon float64) (*SourceObservation, error) {
	key := fmt.Sprintf("observation:%s:%s:%s", objectID, sourceType, sourceID)
	now := time.Now()

	// Get existing or create new
	obs, err := r.GetObservation(ctx, objectID, sourceType, sourceID)
	if err != nil {
		return nil, err
	}

	if obs == nil {
		obs = &SourceObservation{
			ObjectID:   objectID,
			SourceType: sourceType,
			SourceID:   sourceID,
			Status:     "NEW",
			FirstSeen:  now,
		}
	}

	// Add new observation
	obs.Observations = append(obs.Observations, Observation{
		Lat:  lat,
		Lon:  lon,
		Time: now,
	})
	obs.Count++
	obs.LastSeen = now

	// Keep only last N observations (configurable)
	maxObs := r.cfg.Get().Learning.MinObservations * 10
	if len(obs.Observations) > maxObs {
		obs.Observations = obs.Observations[len(obs.Observations)-maxObs:]
	}

	// Calculate variance and update status
	obs.Status = r.calculateStatus(obs)

	// Save
	data, err := json.Marshal(obs)
	if err != nil {
		return nil, err
	}

	learningCfg := r.cfg.Get().Learning
	ttl := time.Duration(learningCfg.TimeWindowHours) * time.Hour
	if err := r.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return nil, err
	}

	return obs, nil
}

func (r *RedisCache) calculateStatus(obs *SourceObservation) string {
	cfg := r.cfg.Get().Learning
	minObs := cfg.MinObservations
	varianceThreshold := cfg.VarianceThreshold

	// Not enough observations
	if obs.Count < minObs {
		return "NEW"
	}

	// Calculate variance
	if len(obs.Observations) < 2 {
		return "NEW"
	}

	// Calculate mean
	var sumLat, sumLon float64
	for _, o := range obs.Observations {
		sumLat += o.Lat
		sumLon += o.Lon
	}
	meanLat := sumLat / float64(len(obs.Observations))
	meanLon := sumLon / float64(len(obs.Observations))

	// Calculate variance
	var varLat, varLon float64
	for _, o := range obs.Observations {
		dLat := o.Lat - meanLat
		dLon := o.Lon - meanLon
		varLat += dLat * dLat
		varLon += dLon * dLon
	}
	varLat /= float64(len(obs.Observations))
	varLon /= float64(len(obs.Observations))

	// Check if stationary (low variance)
	if varLat < varianceThreshold && varLon < varianceThreshold {
		return "STATIONARY"
	}

	return "RANDOM"
}

func (r *RedisCache) GetVariance(ctx context.Context, objectID, sourceType, sourceID string) (float64, float64, error) {
	obs, err := r.GetObservation(ctx, objectID, sourceType, sourceID)
	if err != nil {
		return 0, 0, err
	}
	if obs == nil || len(obs.Observations) < 2 {
		return 0, 0, nil
	}

	// Calculate mean
	var sumLat, sumLon float64
	for _, o := range obs.Observations {
		sumLat += o.Lat
		sumLon += o.Lon
	}
	meanLat := sumLat / float64(len(obs.Observations))
	meanLon := sumLon / float64(len(obs.Observations))

	// Calculate variance
	var varLat, varLon float64
	for _, o := range obs.Observations {
		dLat := o.Lat - meanLat
		dLon := o.Lon - meanLon
		varLat += dLat * dLat
		varLon += dLon * dLon
	}
	varLat /= float64(len(obs.Observations))
	varLon /= float64(len(obs.Observations))

	return varLat, varLon, nil
}

// ============================================
// WiFi methods
// ============================================

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

// ============================================
// Cell tower methods
// ============================================

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

// ============================================
// Bluetooth methods
// ============================================

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

// ============================================
// Device last known location
// ============================================

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

// ============================================
// Calculated coordinates
// ============================================

func (r *RedisCache) GetCalculated(ctx context.Context, sourceType, sourceID string) (*CalculatedCoordinates, error) {
	key := fmt.Sprintf("calculated:%s:%s", sourceType, sourceID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var calc CalculatedCoordinates
	if err := json.Unmarshal(data, &calc); err != nil {
		return nil, err
	}
	return &calc, nil
}

func (r *RedisCache) SetCalculated(ctx context.Context, sourceType, sourceID string, calc *CalculatedCoordinates) error {
	key := fmt.Sprintf("calculated:%s:%s", sourceType, sourceID)
	data, err := json.Marshal(calc)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, 30*24*time.Hour).Err()
}

// ============================================
// Absolute coordinates
// ============================================

func (r *RedisCache) GetAbsolute(ctx context.Context, sourceType, sourceID string) (*AbsoluteCoordinates, error) {
	key := fmt.Sprintf("absolute:%s:%s", sourceType, sourceID)
	data, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var abs AbsoluteCoordinates
	if err := json.Unmarshal(data, &abs); err != nil {
		return nil, err
	}
	return &abs, nil
}

func (r *RedisCache) SetAbsolute(ctx context.Context, sourceType, sourceID string, abs *AbsoluteCoordinates) error {
	key := fmt.Sprintf("absolute:%s:%s", sourceType, sourceID)
	data, err := json.Marshal(abs)
	if err != nil {
		return err
	}

	// TTL based on expires_at
	ttl := abs.ExpiresAt.Sub(time.Now())
	if ttl <= 0 {
		return nil // Already expired
	}
	
	return r.client.Set(ctx, key, data, ttl).Err()
}

func (r *RedisCache) DeleteAbsolute(ctx context.Context, sourceType, sourceID string) error {
	key := fmt.Sprintf("absolute:%s:%s", sourceType, sourceID)
	return r.client.Del(ctx, key).Err()
}

// ============================================
// Helper functions
// ============================================

func ConvertEIDToRSSI(eid int32) int32 {
	rssi := -eid
	if rssi < -128 {
		rssi += 128
	}
	return rssi
}

func CalculateHaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth radius in meters

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return R * c
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180
}
