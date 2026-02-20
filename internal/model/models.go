package model

import "time"

// ============================================
// Coordinate Validation
// ============================================

type CoordinateRequest struct {
	DeviceID    string         `json:"device_id"`
	Latitude    float64        `json:"latitude"`
	Longitude   float64        `json:"longitude"`
	Accuracy    float32        `json:"accuracy"`
	Timestamp   int64          `json:"timestamp"`
	Wifi        []WifiAP       `json:"wifi,omitempty"`
	Bluetooth   []BluetoothDev `json:"bluetooth,omitempty"`
	CellTowers  []CellTower    `json:"cell_towers,omitempty"`
}

type CoordinateResponse struct {
	Result             ValidationResult `json:"result"`
	Confidence         float32          `json:"confidence"`
	EstimatedAccuracy  float32          `json:"estimated_accuracy"`
	Reason             string           `json:"reason"`
}

type ValidationResult string

const (
	ValidationResultValid      ValidationResult = "VALID"
	ValidationResultInvalid   ValidationResult = "INVALID"
	ValidationResultUncertain ValidationResult = "UNCERTAIN"
)

// ============================================
// WiFi / Bluetooth / Cell Models
// ============================================

type WifiAP struct {
	SSID  string `json:"ssid"`
	BSSID string `json:"bssid"`
	RSSI  int32  `json:"rssi"`
}

type BluetoothDev struct {
	MAC  string `json:"mac"`
	RSSI int32  `json:"rssi"`
}

type CellTower struct {
	CellID uint32 `json:"cell_id"`
	LAC    uint32 `json:"lac"`
	MCC    uint32 `json:"mcc"`
	MNC    uint32 `json:"mnc"`
	RSSI   int32  `json:"rssi"`
}

// ============================================
// Learning Models
// ============================================

type LearnRequest struct {
	ObjectID   string         `json:"object_id"`
	Latitude   float64        `json:"latitude"`
	Longitude  float64        `json:"longitude"`
	Accuracy   float32        `json:"accuracy"`
	Timestamp  int64          `json:"timestamp"`
	Wifi       []WifiAP       `json:"wifi,omitempty"`
	Bluetooth  []BluetoothDev `json:"bluetooth,omitempty"`
	CellTowers []CellTower    `json:"cell_towers,omitempty"`
}

type LearnResponse struct {
	Result             LearningResult `json:"result"`
	StationarySources  []string       `json:"stationary_sources,omitempty"`
	RandomSources      []string       `json:"random_sources,omitempty"`
}

type LearningResult string

const (
	LearningResultLeared          LearningResult = "LEARNED"
	LearningResultNeedMoreData   LearningResult = "NEED_MORE_DATA"
	LearningResultStationary     LearningResult = "STATIONARY_DETECTED"
	LearningResultRandomExcluded LearningResult = "RANDOM_EXCLUDED"
)

// ============================================
// Cache Models (Redis)
// ============================================

type CachedWifi struct {
	BSSID     string    `json:"bssid"`
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	Version   int64     `json:"version"`
	ObsCount  int64     `json:"obs_count"`
	Confidence float64  `json:"confidence"`
}

type CachedCell struct {
	CellID    uint32  `json:"cell_id"`
	LAC       uint32  `json:"lac"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	Version   int64   `json:"version"`
	ObsCount  int64   `json:"obs_count"`
	Confidence float64 `json:"confidence"`
}

type CachedBT struct {
	MAC       string  `json:"mac"`
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
	LastSeen  time.Time `json:"last_seen"`
	Version   int64   `json:"version"`
	ObsCount  int64   `json:"obs_count"`
	Confidence float64 `json:"confidence"`
}

type DevicePosition struct {
	DeviceID  string    `json:"device_id"`
	Latitude  float64   `json:"lat"`
	Longitude float64   `json:"lon"`
	Timestamp int64     `json:"timestamp"`
	LastSeen  time.Time `json:"last_seen"`
}

// ============================================
// Kafka Events
// ============================================

type RefinementEvent struct {
	DeviceID    string          `json:"device_id"`
	Latitude    float64         `json:"latitude"`
	Longitude   float64         `json:"longitude"`
	Accuracy    float32         `json:"accuracy"`
	Timestamp   int64           `json:"timestamp"`
	Result      ValidationResult `json:"result"`
	Confidence  float32         `json:"confidence"`
	HasWifi     bool            `json:"has_wifi"`
	HasBT       bool            `json:"has_bt"`
	HasCell     bool            `json:"has_cell"`
	EventTime   time.Time       `json:"event_time"`
}

type LearningEvent struct {
	ObjectID    string         `json:"object_id"`
	Latitude    float64        `json:"latitude"`
	Longitude   float64        `json:"longitude"`
	Timestamp   int64          `json:"timestamp"`
	Wifi        []WifiAP       `json:"wifi,omitempty"`
	Bluetooth   []BluetoothDev `json:"bluetooth,omitempty"`
	CellTowers  []CellTower    `json:"cell_towers,omitempty"`
	IsCompanion bool            `json:"is_companion"`
	EventTime   time.Time      `json:"event_time"`
}

// ============================================
// Companion Detection
// ============================================

type CompanionSource struct {
	PointID       string    `json:"point_id"`
	PointType     PointType `json:"point_type"`
	Observations  int32     `json:"observations"`
	Stability     float32   `json:"stability"`
	IsStationary bool      `json:"is_stationary"`
	FirstSeen     int64     `json:"first_seen"`
	LastSeen      int64     `json:"last_seen"`
}

type PointType string

const (
	PointTypeWifi PointType = "WIFI"
	PointTypeCell PointType = "CELL"
	PointTypeBT   PointType = "BLE"
)

// ============================================
// Storage Models (ClickHouse)
// ============================================

type ValidationRecord struct {
	DeviceID     string          `json:"device_id"`
	Latitude     float64         `json:"latitude"`
	Longitude    float64         `json:"longitude"`
	Accuracy     float32         `json:"accuracy"`
	Timestamp    int64           `json:"timestamp"`
	HasWifi      bool            `json:"has_wifi"`
	HasBT        bool            `json:"has_bt"`
	HasCell      bool            `json:"has_cell"`
	Result       ValidationResult `json:"result"`
	Confidence   float32         `json:"confidence"`
	FlowType     string          `json:"flow_type"` // "refinement" or "learning"
	InsertTime   time.Time       `json:"insert_time"`
}

type SourceStatsRecord struct {
	Type        string    `json:"type"` // wifi, cell, bt
	PointID     string    `json:"point_id"`
	Latitude    float64   `json:"lat"`
	Longitude   float64   `json:"lon"`
	Observations int64    `json:"obs"`
	LastUpdated time.Time `json:"last_updated"`
}
