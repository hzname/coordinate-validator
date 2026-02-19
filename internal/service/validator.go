package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/storage"
	pb "coordinate-validator/pkg/pb"
)

type ValidatorService struct {
	cache    *cache.RedisCache
	storage  *storage.ClickHouseStorage
	cfg      config.ValidationConfig
	pb.UnimplementedCoordinateValidatorServer
}

func NewValidatorService(
	cache *cache.RedisCache,
	storage *storage.ClickHouseStorage,
	cfg config.ValidationConfig,
) *ValidatorService {
	return &ValidatorService{
		cache: cache,
		storage:  storage,
		cfg:      cfg,
	}
}

func (s *ValidatorService) Validate(ctx context.Context, req *pb.CoordinateRequest) (*pb.CoordinateResponse, error) {
	result := pb.ValidationResult_VALID
	confidence := float32(1.0)
	reasons := []string{}

	// 1. Time validation
	now := time.Now()
	reqTime := time.Unix(req.Timestamp, 0)
	timeDiff := now.Sub(reqTime)

	if timeDiff < 0 {
		// Timestamp in the future
		result = pb.ValidationResult_INVALID
		reasons = append(reasons, "timestamp in the future")
	} else if timeDiff > s.cfg.MaxTimeDiff {
		// Timestamp too old
		result = pb.ValidationResult_INVALID
		reasons = append(reasons, fmt.Sprintf("timestamp too old: %v", timeDiff))
	}

	// 2. Speed validation (if we have previous location)
	lastLoc, err := s.cache.GetLastKnownLocation(ctx, req.DeviceId)
	if err == nil && lastLoc != nil {
		speed := calculateSpeedKmH(
			lastLoc.Lat, lastLoc.Lon, lastLoc.Time,
			req.Latitude, req.Longitude, reqTime,
		)

		if speed > s.cfg.MaxSpeedKmH {
			result = pb.ValidationResult_INVALID
			reasons = append(reasons, fmt.Sprintf("impossible speed: %.1f km/h", speed))
		}
	}

	// 3. WiFi validation (if available)
	if len(req.Wifi) > 0 {
		hasKnownWifi := false
		for _, wifi := range req.Wifi {
			wifiPoint, err := s.cache.GetWifiPoint(ctx, wifi.Bssid)
			if err == nil && wifiPoint != nil {
				// Known WiFi - boost confidence
				confidence += s.cfg.WifiWeight * 0.3
				hasKnownWifi = true
			} else {
				// Unknown WiFi - record for learning
				go s.recordWifiPoint(ctx, wifi.Bssid, req.Latitude, req.Longitude, req.Accuracy)
			}
		}
		if hasKnownWifi {
			reasons = append(reasons, "known WiFi access points found")
		}
	}

	// 4. Cell tower validation (if available)
	if req.CellTower != nil {
		cellPoint, err := s.cache.GetCellPoint(ctx, req.CellTower.CellId, int(req.CellTower.Lac))
		if err == nil && cellPoint != nil {
			// Known cell tower - boost confidence
			confidence += s.cfg.CellWeight * 0.3
			reasons = append(reasons, "known cell tower found")
		} else if req.CellTower.CellId != "" {
			// Unknown cell - record for learning
			go s.recordCellPoint(ctx, req.CellTower.CellId, int(req.CellTower.Lac), req.Latitude, req.Longitude)
		}
	}

	// 5. Bluetooth validation (if available)
	if len(req.Bluetooth) > 0 {
		for _, bt := range req.Bluetooth {
			btPoint, err := s.cache.GetBluetoothPoint(ctx, bt.Mac)
			if err == nil && btPoint != nil {
				confidence += s.cfg.BluetoothWeight * 0.3
				reasons = append(reasons, "known Bluetooth device found")
			}
		}
	}

	// Adjust confidence
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0 {
		confidence = 0
	}

	// If INVALID, confidence is 0
	if result == pb.ValidationResult_INVALID {
		confidence = 0
	}

	// Save to history
	go s.saveToHistory(ctx, req, result, confidence)

	// Update last known location
	s.cache.SetLastKnownLocation(ctx, req.DeviceId, &cache.LastKnownLocation{
		Lat:  req.Latitude,
		Lon:  req.Longitude,
		Time: reqTime,
	})

	response := &pb.CoordinateResponse{
		Result:            result,
		Confidence:        confidence,
		EstimatedAccuracy: req.Accuracy,
	}
	if len(reasons) > 0 {
		response.Reason = fmt.Sprintf("; ", reasons...)
	}

	return response, nil
}

func (s *ValidatorService) ValidateBatch(stream pb.CoordinateValidator_ValidateBatchServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			break
		}

		resp, err := s.Validate(stream.Context(), req)
		if err != nil {
			return err
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

// Helper: calculate speed in km/h
func calculateSpeedKmH(lat1, lon1 float64, time1 time.Time, lat2, lon2 float64, time2 time.Time) float64 {
	// Haversine distance
	const R = 6371.0 // Earth's radius in km

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := R * c

	// Time in hours
	hours := time2.Sub(time1).Hours()
	if hours <= 0 {
		hours = 0.001 // avoid division by zero
	}

	return distance / hours
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180
}

// Background: record new WiFi point
func (s *ValidatorService) recordWifiPoint(ctx context.Context, bssid string, lat, lon float64, accuracy float32) {
	point := &cache.WifiPoint{
		Lat:      lat,
		Lon:      lon,
		LastSeen: time.Now(),
		Count:    1,
	}
	s.cache.SetWifiPoint(ctx, bssid, point)
	s.storage.UpdatePointStats(ctx, "wifi", bssid, lat, lon, accuracy)
}

// Background: record new cell tower
func (s *ValidatorService) recordCellPoint(ctx context.Context, cellID string, lac int, lat, lon float64) {
	point := &cache.CellPoint{
		Lat:      lat,
		Lon:      lon,
		LastSeen: time.Now(),
	}
	s.cache.SetCellPoint(ctx, cellID, lac, point)
	s.storage.UpdatePointStats(ctx, "cell", fmt.Sprintf("%s:%d", cellID, lac), lat, lon, 0)
}

// Background: save to ClickHouse
func (s *ValidatorService) saveToHistory(ctx context.Context, req *pb.CoordinateRequest, result pb.ValidationResult, confidence float32) {
	resultStr := "valid"
	switch result {
	case pb.ValidationResult_INVALID:
		resultStr = "invalid"
	case pb.ValidationResult_UNCERTAIN:
		resultStr = "uncertain"
	}

	record := storage.CoordinateRecord{
		DeviceID:         req.DeviceId,
		Latitude:         req.Latitude,
		Longitude:        req.Longitude,
		Accuracy:         req.Accuracy,
		Timestamp:        time.Unix(req.Timestamp, 0),
		HasWifi:          len(req.Wifi) > 0,
		HasBluetooth:     len(req.Bluetooth) > 0,
		HasCell:          req.CellTower != nil,
		ValidationResult: resultStr,
		Confidence:       confidence,
	}

	s.storage.InsertCoordinate(context.Background(), record)
}
