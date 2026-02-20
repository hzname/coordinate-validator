package core

import (
	"context"
	"math"
	"time"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
)

type ValidationCore struct {
	cache *cache.RedisCache
	cfg   *config.ValidationConfig
}

func NewValidationCore(cache *cache.RedisCache, cfg *config.ValidationConfig) *ValidationCore {
	return &ValidationCore{
		cache: cache,
		cfg:   cfg,
	}
}

// ============================================
// Main Validation Flow
// ============================================

func (v *ValidationCore) Validate(ctx context.Context, req *model.CoordinateRequest) (*model.CoordinateResponse, error) {
	// Layer 1: Rule-based validation
	if err := v.validateTime(ctx, req); err != nil {
		return &model.CoordinateResponse{
			Result:   model.ValidationResultInvalid,
			Reason:  err.Error(),
		}, nil
	}

	speedCheck, err := v.validateSpeed(ctx, req)
	if err != nil {
		return &model.CoordinateResponse{
			Result:   model.ValidationResultInvalid,
			Reason:  err.Error(),
		}, nil
	}

	// Layer 2: Triangulation via sources
	confidence, estimatedAccuracy, reasons := v.triangulate(ctx, req)

	// Apply speed check result
	if !speedCheck.valid {
		confidence *= 0.5
		reasons = append(reasons, speedCheck.reason)
	}

	// Determine final result
	result := v.determineResult(confidence)
	reason := ""
	if len(reasons) > 0 {
		reason = reasons[0]
	}

	return &model.CoordinateResponse{
		Result:             result,
		Confidence:         confidence,
		EstimatedAccuracy: estimatedAccuracy,
		Reason:             reason,
	}, nil
}

// ============================================
// Layer 1: Time Validation
// ============================================

func (v *ValidationCore) validateTime(ctx context.Context, req *model.CoordinateRequest) error {
	now := time.Now().Unix()
	reqTime := req.Timestamp

	// Check if timestamp is in the future
	if reqTime > now {
		return &ValidationError{
			Code:    "FUTURE_TIMESTAMP",
			Message: "Timestamp is in the future",
		}
	}

	// Check if timestamp is too old
	diff := time.Duration(now-reqTime) * time.Second
	if diff > v.cfg.MaxTimeDiff {
		return &ValidationError{
			Code:    "TIMESTAMP_TOO_OLD",
			Message: "Timestamp is older than max allowed",
		}
	}

	return nil
}

// ============================================
// Layer 1: Speed Validation
// ============================================

type speedCheckResult struct {
	valid  bool
	reason string
}

func (v *ValidationCore) validateSpeed(ctx context.Context, req *model.CoordinateRequest) (speedCheckResult, error) {
	// Get last known position for this device
	lastPos, err := v.cache.GetDevicePosition(ctx, req.DeviceID)
	if err != nil {
		return speedCheckResult{valid: true, reason: ""}, err
	}

	// No previous data - skip check
	if lastPos == nil {
		return speedCheckResult{valid: true, reason: ""}, nil
	}

	// Calculate time difference
	timeDiff := time.Duration(req.Timestamp-lastPos.Timestamp) * time.Second
	if timeDiff <= 0 {
		return speedCheckResult{valid: true, reason: ""}, nil
	}

	// Calculate distance
	distance := HaversineDistance(
		lastPos.Latitude, lastPos.Longitude,
		req.Latitude, req.Longitude,
	)

	// Calculate speed in km/h
	speed := (distance / timeDiff.Hours())

	if speed > v.cfg.MaxSpeedKmH {
		return speedCheckResult{
			valid:  false,
			reason: "Speed exceeds maximum",
		}, nil
	}

	return speedCheckResult{valid: true, reason: ""}, nil
}

// ============================================
// Layer 2: Triangulation
// ============================================

func (v *ValidationCore) triangulate(ctx context.Context, req *model.CoordinateRequest) (float32, float32, []string) {
	var reasons []string
	var totalConfidence float32 = 0.0
	var weight float32 = 0.0

	// Check WiFi
	if len(req.Wifi) > 0 {
		conf, acc, r := v.checkWifi(ctx, req.Wifi)
		if conf > 0 {
			totalConfidence += conf * 0.4
			weight += 0.4
			if r != "" {
				reasons = append(reasons, r)
			}
		}
	}

	// Check Cell Towers
	if len(req.CellTowers) > 0 {
		conf, acc, r := v.checkCellTowers(ctx, req.CellTowers)
		if conf > 0 {
			totalConfidence += conf * 0.35
			weight += 0.35
			if r != "" {
				reasons = append(reasons, r)
			}
		}
	}

	// Check Bluetooth
	if len(req.Bluetooth) > 0 {
		conf, acc, r := v.checkBluetooth(ctx, req.Bluetooth)
		if conf > 0 {
			totalConfidence += conf * 0.25
			weight += 0.25
			if r != "" {
				reasons = append(reasons, r)
			}
		}
	}

	// Normalize by weight
	if weight > 0 {
		totalConfidence /= weight
	} else {
		// No sources available - use base confidence
		totalConfidence = 0.3
		weight = 1.0
	}

	// Estimated accuracy based on available sources
	estimatedAccuracy := float32(req.Accuracy) * (1.0 - totalConfidence*0.5)

	return totalConfidence, estimatedAccuracy, reasons
}

func (v *ValidationCore) checkWifi(ctx context.Context, wifi []model.WifiAP) (float32, float32, string) {
	var maxConf float32 = 0
	var avgAccuracy float32 = 0

	for _, w := range wifi {
		cached, err := v.cache.GetWifi(ctx, w.BSSID)
		if err != nil || cached == nil {
			continue
		}

		// Boost confidence based on cached data
		conf := float32(cached.Confidence)
		if conf > maxConf {
			maxConf = conf
		}
		avgAccuracy += float32(cached.Confidence * 10) // approximate
	}

	if maxConf > 0 {
		avgAccuracy /= float32(len(wifi))
		return maxConf, avgAccuracy, "WiFi triangulation matched"
	}

	return 0, 0, ""
}

func (v *ValidationCore) checkCellTowers(ctx context.Context, cells []model.CellTower) (float32, float32, string) {
	var maxConf float32 = 0
	var avgAccuracy float32 = 0

	for _, c := range cells {
		cached, err := v.cache.GetCell(ctx, c.CellID, c.LAC)
		if err != nil || cached == nil {
			continue
		}

		conf := float32(cached.Confidence)
		if conf > maxConf {
			maxConf = conf
		}
		avgAccuracy += 500 // cell tower approximate accuracy in meters
	}

	if maxConf > 0 {
		avgAccuracy /= float32(len(cells))
		return maxConf, avgAccuracy, "Cell tower triangulation matched"
	}

	return 0, 0, ""
}

func (v *ValidationCore) checkBluetooth(ctx context.Context, bt []model.BluetoothDev) (float32, float32, string) {
	var maxConf float32 = 0

	for _, b := range bt {
		cached, err := v.cache.GetBT(ctx, b.MAC)
		if err != nil || cached == nil {
			continue
		}

		conf := float32(cached.Confidence)
		if conf > maxConf {
			maxConf = conf
		}
	}

	if maxConf > 0 {
		return maxConf, 2.0, "Bluetooth triangulation matched"
	}

	return 0, 0, ""
}

// ============================================
// Result Determination
// ============================================

func (v *ValidationCore) determineResult(confidence float32) model.ValidationResult {
	if confidence >= v.cfg.ConfidenceThresholds.High {
		return model.ValidationResultValid
	}
	if confidence <= v.cfg.ConfidenceThresholds.Low {
		return model.ValidationResultInvalid
	}
	return model.ValidationResultUncertain
}

// ============================================
// Update Device Position (for speed check)
// ============================================

func (v *ValidationCore) UpdateDevicePosition(ctx context.Context, req *model.CoordinateRequest) error {
	pos := &model.DevicePosition{
		DeviceID:  req.DeviceID,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Timestamp: req.Timestamp,
		LastSeen:  time.Now(),
	}
	return v.cache.SetDevicePosition(ctx, pos)
}

// ============================================
// Helper: Haversine Distance (km)
// ============================================

func HaversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371.0 // Earth radius in km

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

// ============================================
// Error Type
// ============================================

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Code + ": " + e.Message
}
