package service

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/storage"
	pb "coordinate-validator/pkg/pb"
)

type ValidatorService struct {
	cache   *cache.RedisCache
	storage  *storage.ClickHouseStorage
	cfg      config.ValidationConfig
	wg       sync.WaitGroup
	cacheMu  sync.Mutex
	pb.UnimplementedCoordinateValidatorServer
}

func NewValidatorService(
	cache *cache.RedisCache,
	storage *storage.ClickHouseStorage,
	cfg config.ValidationConfig,
) *ValidatorService {
	return &ValidatorService{
		cache:  cache,
		storage: storage,
		cfg:     cfg,
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
		result = pb.ValidationResult_INVALID
		reasons = append(reasons, "timestamp in the future")
	} else if timeDiff > s.cfg.MaxTimeDiff {
		result = pb.ValidationResult_INVALID
		reasons = append(reasons, fmt.Sprintf("timestamp too old: %v", timeDiff))
	}

	// 2. Speed validation (if we have previous location)
	lastLoc, err := s.cache.GetLastKnownLocation(ctx, req.DeviceId)
	if err == nil && lastLoc != nil && result != pb.ValidationResult_INVALID {
		speed := calculateSpeedKmH(
			lastLoc.Lat, lastLoc.Lon, lastLoc.Time,
			req.Latitude, req.Longitude, reqTime,
		)

		if speed > s.cfg.MaxSpeedKmH {
			result = pb.ValidationResult_INVALID
			reasons = append(reasons, fmt.Sprintf("impossible speed: %.1f km/h", speed))
		}
	}

	// 3. EGTS_ENVELOPE_LOW (92) - WiFi / BLE validation
	if len(req.Wifi) > 0 && result != pb.ValidationResult_INVALID {
		hasKnownWifi := false
		for _, wifi := range req.Wifi {
			wifiPoint, err := s.cache.GetWifiPoint(ctx, wifi.Bssid)
			if err == nil && wifiPoint != nil {
				confidence += s.cfg.WifiWeight * 0.3
				hasKnownWifi = true
				reasons = append(reasons, fmt.Sprintf("known WiFi: %s", wifi.Bssid))
			} else {
				rssi := wifi.Rssi
				if rssi == 0 && wifi.Eid != 0 {
					rssi = cache.ConvertEIDToRSSI(wifi.Eid)
				}
				// FIX #3: proper context with timeout
				s.wg.Add(1)
				go func(bssid, ssid string, lat, lon float64, acc float32, rssi int32) {
					defer s.wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					s.recordWifiPointFromEGTS(ctx, bssid, ssid, lat, lon, acc, rssi)
				}(wifi.Bssid, wifi.Ssid, req.Latitude, req.Longitude, req.Accuracy, rssi)
			}
		}
		if hasKnownWifi {
			reasons = append(reasons, "known WiFi access points found")
		}
	}

	// 4. EGTS_ENVELOPE_LOW (92) - Bluetooth validation
	if len(req.Bluetooth) > 0 && result != pb.ValidationResult_INVALID {
		for _, bt := range req.Bluetooth {
			btPoint, err := s.cache.GetBluetoothPoint(ctx, bt.Mac)
			if err == nil && btPoint != nil {
				confidence += s.cfg.BluetoothWeight * 0.3
				reasons = append(reasons, fmt.Sprintf("known BLE: %s", bt.Mac))
			}
		}
	}

	// 5. EGTS_ENVELOPE_HIGHT (91) - Cell tower validation
	if len(req.CellTowers) > 0 && result != pb.ValidationResult_INVALID {
		hasKnownCell := false
		for _, cell := range req.CellTowers {
			cellPoint, err := s.cache.GetCellPoint(ctx, cell.CellId, cell.Lac)
			if err == nil && cellPoint != nil {
				confidence += s.cfg.CellWeight * 0.3
				hasKnownCell = true
				reasons = append(reasons, fmt.Sprintf("known cell: CID=%d LAC=%d", cell.CellId, cell.Lac))
			} else {
				rssi := cell.Rssi
				if rssi == 0 && cell.Eid != 0 {
					rssi = cache.ConvertEIDToRSSI(cell.Eid)
				}
				// FIX #3: proper context with timeout
				s.wg.Add(1)
				go func(cellID, lac, mcc, mnc uint32, lat, lon float64, rssi int32) {
					defer s.wg.Done()
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					s.recordCellPointFromEGTS(ctx, cellID, lac, mcc, mnc, lat, lon, rssi)
				}(cell.CellId, cell.Lac, cell.Mcc, cell.Mnc, req.Latitude, req.Longitude, rssi)
			}
		}
		if hasKnownCell {
			reasons = append(reasons, "known cell towers found")
		}
	}

	// Adjust confidence
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0 {
		confidence = 0
	}

	if result == pb.ValidationResult_INVALID {
		confidence = 0
	}

	if result == pb.ValidationResult_VALID && confidence < 0.5 {
		result = pb.ValidationResult_UNCERTAIN
		reasons = append(reasons, "low confidence")
	}

	// FIX #3: proper context with timeout for async save
	s.wg.Add(1)
	go func(req *pb.CoordinateRequest, result pb.ValidationResult, conf float32) {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.saveToHistory(ctx, req, result, conf)
	}(req, result, confidence)

	// FIX #3: proper context with timeout for last known location
	if result != pb.ValidationResult_INVALID {
		s.wg.Add(1)
		go func(deviceID string, lat, lon float64, t time.Time) {
			defer s.wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.cache.SetLastKnownLocation(ctx, deviceID, &cache.LastKnownLocation{
				Lat:  lat,
				Lon:  lon,
				Time: t,
			})
		}(req.DeviceId, req.Latitude, req.Longitude, reqTime)
	}

	response := &pb.CoordinateResponse{
		Result:            result,
		Confidence:       confidence,
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

		s.cacheMu.Lock()
		resp, err := s.Validate(stream.Context(), req)
		s.cacheMu.Unlock()

		if err != nil {
			return err
		}

		if err := stream.Send(resp); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown waits for all async goroutines to complete
func (s *ValidatorService) Shutdown(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ============ Helper functions ============

func calculateSpeedKmH(lat1, lon1 float64, time1 time.Time, lat2, lon2 float64, time2 time.Time) float64 {
	const R = 6371.0 // Earth's radius in km

	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)

	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := R * c
	hours := time2.Sub(time1).Hours()
	if hours <= 0 {
		hours = 0.001
	}

	return distance / hours
}

func toRad(deg float64) float64 {
	return deg * math.Pi / 180
}

// ============ EGTS data recording (self-learning) ============

func (s *ValidatorService) recordWifiPointFromEGTS(ctx context.Context, bssid, ssid string, lat, lon float64, accuracy float32, rssi int32) {
	point := &cache.WifiPoint{
		Lat:      lat,
		Lon:      lon,
		LastSeen: time.Now(),
		Count:    1,
		SSID:     ssid,
		EID:      rssi,
	}
	if err := s.cache.SetWifiPoint(ctx, bssid, point); err != nil {
		fmt.Printf("Warning: failed to save WiFi point: %v\n", err)
	}
	if err := s.storage.UpdatePointStats(ctx, "wifi", bssid, lat, lon, accuracy); err != nil {
		fmt.Printf("Warning: failed to update WiFi stats: %v\n", err)
	}
}

func (s *ValidatorService) recordCellPointFromEGTS(ctx context.Context, cellID, lac, mcc, mnc uint32, lat, lon float64, rssi int32) {
	point := &cache.CellPoint{
		Lat:      lat,
		Lon:      lon,
		LastSeen: time.Now(),
		LAC:      lac,
		MCC:      mcc,
		MNC:      mnc,
		EID:      rssi,
	}
	if err := s.cache.SetCellPoint(ctx, cellID, lac, point); err != nil {
		fmt.Printf("Warning: failed to save cell point: %v\n", err)
	}
	if err := s.storage.UpdatePointStats(ctx, fmt.Sprintf("cell_%d_%d", mcc, mnc), fmt.Sprintf("%d:%d", cellID, lac), lat, lon, 0); err != nil {
		fmt.Printf("Warning: failed to update cell stats: %v\n", err)
	}
}

// Save to ClickHouse
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
		HasCell:          len(req.CellTowers) > 0,
		ValidationResult: resultStr,
		Confidence:       confidence,
	}

	if err := s.storage.InsertCoordinate(ctx, record); err != nil {
		fmt.Printf("Warning: failed to save coordinate history: %v\n", err)
	}
}
