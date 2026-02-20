package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
)

type RedisCache struct {
	client *redis.Client
	cfg    *config.RedisConfig
}

func NewRedisCache(cfg *config.RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisCache{
		client: client,
		cfg:    cfg,
	}, nil
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

// ============================================
// WiFi Operations
// ============================================

func (c *RedisCache) GetWifi(ctx context.Context, bssid string) (*model.CachedWifi, error) {
	key := fmt.Sprintf("wifi:%s", bssid)
	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var wifi model.CachedWifi
	if err := json.Unmarshal([]byte(data), &wifi); err != nil {
		return nil, err
	}
	return &wifi, nil
}

func (c *RedisCache) SetWifi(ctx context.Context, wifi *model.CachedWifi) error {
	key := fmt.Sprintf("wifi:%s", wifi.BSSID)
	data, err := json.Marshal(wifi)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, 0).Err()
}

// ============================================
// Cell Tower Operations
// ============================================

func (c *RedisCache) GetCell(ctx context.Context, cellID uint32, lac uint32) (*model.CachedCell, error) {
	key := fmt.Sprintf("cell:%d:%d", cellID, lac)
	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var cell model.CachedCell
	if err := json.Unmarshal([]byte(data), &cell); err != nil {
		return nil, err
	}
	return &cell, nil
}

func (c *RedisCache) SetCell(ctx context.Context, cell *model.CachedCell) error {
	key := fmt.Sprintf("cell:%d:%d", cell.CellID, cell.LAC)
	data, err := json.Marshal(cell)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, 0).Err()
}

// ============================================
// Bluetooth Operations
// ============================================

func (c *RedisCache) GetBT(ctx context.Context, mac string) (*model.CachedBT, error) {
	key := fmt.Sprintf("bt:%s", mac)
	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var bt model.CachedBT
	if err := json.Unmarshal([]byte(data), &bt); err != nil {
		return nil, err
	}
	return &bt, nil
}

func (c *RedisCache) SetBT(ctx context.Context, bt *model.CachedBT) error {
	key := fmt.Sprintf("bt:%s", bt.MAC)
	data, err := json.Marshal(bt)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, 0).Err()
}

// ============================================
// Device Position Operations
// ============================================

func (c *RedisCache) GetDevicePosition(ctx context.Context, deviceID string) (*model.DevicePosition, error) {
	key := fmt.Sprintf("device:%s", deviceID)
	data, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var pos model.DevicePosition
	if err := json.Unmarshal([]byte(data), &pos); err != nil {
		return nil, err
	}
	return &pos, nil
}

func (c *RedisCache) SetDevicePosition(ctx context.Context, pos *model.DevicePosition) error {
	key := fmt.Sprintf("device:%s", pos.DeviceID)
	data, err := json.Marshal(pos)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, 0).Err()
}

// ============================================
// Companion Detection
// ============================================

func (c *RedisCache) AddCompanion(ctx context.Context, objectID, pointID string, pointType model.PointType) error {
	key := fmt.Sprintf("companions:%s", objectID)
	member := fmt.Sprintf("%s:%s", pointType, pointID)
	return c.client.SAdd(ctx, key, member).Err()
}

func (c *RedisCache) GetCompanions(ctx context.Context, objectID string) ([]model.CompanionSource, error) {
	key := fmt.Sprintf("companions:%s", objectID)
	members, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var companions []model.CompanionSource
	for _, m := range members {
		var pt model.PointType
		var pointID string
		fmt.Sscanf(m, "%s:%s", &pt, &pointID)
		companions = append(companions, model.CompanionSource{
			PointID:   pointID,
			PointType: pt,
		})
	}
	return companions, nil
}

// ============================================
// Batch Operations for Learning
// ============================================

func (c *RedisCache) MGetWifi(ctx context.Context, bssids []string) (map[string]*model.CachedWifi, error) {
	if len(bssids) == 0 {
		return nil, nil
	}

	keys := make([]string, len(bssids))
	for i, b := range bssids {
		keys[i] = fmt.Sprintf("wifi:%s", b)
	}

	results, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	m := make(map[string]*model.CachedWifi)
	for i, r := range results {
		if r == nil {
			continue
		}
		var wifi model.CachedWifi
		if err := json.Unmarshal([]byte(r.(string)), &wifi); err != nil {
			continue
		}
		m[bssids[i]] = &wifi
	}
	return m, nil
}

func (c *RedisCache) MGetCell(ctx context.Context, cells []struct {
	CellID uint32
	LAC    uint32
}) (map[string]*model.CachedCell, error) {
	if len(cells) == 0 {
		return nil, nil
	}

	keys := make([]string, len(cells))
	for i, c := range cells {
		keys[i] = fmt.Sprintf("cell:%d:%d", c.CellID, c.LAC)
	}

	results, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	m := make(map[string]*model.CachedCell)
	for i, r := range results {
		if r == nil {
			continue
		}
		var cell model.CachedCell
		if err := json.Unmarshal([]byte(r.(string)), &cell); err != nil {
			continue
		}
		key := fmt.Sprintf("%d:%d", cells[i].CellID, cells[i].LAC)
		m[key] = &cell
	}
	return m, nil
}
