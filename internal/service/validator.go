package service

import (
	"context"
	"fmt"
	"math"
	"sort"
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

// ============ CoordinateValidator Service ============

func (s *ValidatorService) Validate(ctx context.Context, req *pb.CoordinateRequest) (*pb.CoordinateResponse, error) {
	result := pb.ValidationResult_VALID
	confidence := float32(1.0)
	reasons := []string{}

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

	lastLoc, err := s.cache.GetLastKnownLocation(ctx, req.DeviceId)
	if err == nil && lastLoc != nil && result != pb.ValidationResult_INVALID {
		speed := calculateSpeedKmH(lastLoc.Lat, lastLoc.Lon, lastLoc.Time, req.Latitude, req.Longitude, reqTime)
		if speed > s.cfg.MaxSpeedKmH {
			result = pb.ValidationResult_INVALID
			reasons = append(reasons, fmt.Sprintf("impossible speed: %.1f km/h", speed))
		}
	}

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

	if len(req.Bluetooth) > 0 && result != pb.ValidationResult_INVALID {
		for _, bt := range req.Bluetooth {
			btPoint, err := s.cache.GetBluetoothPoint(ctx, bt.Mac)
			if err == nil && btPoint != nil {
				confidence += s.cfg.BluetoothWeight * 0.3
				reasons = append(reasons, fmt.Sprintf("known BLE: %s", bt.Mac))
			}
		}
	}

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

	s.wg.Add(1)
	go func(req *pb.CoordinateRequest, result pb.ValidationResult, conf float32) {
		defer s.wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.saveToHistory(ctx, req, result, conf)
	}(req, result, confidence)

	if result != pb.ValidationResult_INVALID {
		s.wg.Add(1)
		go func(deviceID string, lat, lon float64, t time.Time) {
			defer s.wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.cache.SetLastKnownLocation(ctx, deviceID, &cache.LastKnownLocation{Lat: lat, Lon: lon, Time: t})
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

// ============ LearningService ============

func (s *ValidatorService) LearnFromCoordinates(ctx context.Context, req *pb.LearnRequest) (*pb.LearnResponse, error) {
	stationarySources := []string{}
	randomSources := []string{}

	for _, wifi := range req.Wifi {
		obs, err := s.cache.AddObservation(ctx, req.ObjectId, "wifi", wifi.Bssid, req.Latitude, req.Longitude)
		if err == nil && obs != nil {
			if obs.Status == "STATIONARY" {
				stationarySources = append(stationarySources, wifi.Bssid)
			} else if obs.Status == "RANDOM" {
				randomSources = append(randomSources, wifi.Bssid)
			}
		}
	}

	for _, bt := range req.Bluetooth {
		obs, err := s.cache.AddObservation(ctx, req.ObjectId, "ble", bt.Mac, req.Latitude, req.Longitude)
		if err == nil && obs != nil {
			if obs.Status == "STATIONARY" {
				stationarySources = append(stationarySources, bt.Mac)
			} else if obs.Status == "RANDOM" {
				randomSources = append(randomSources, bt.Mac)
			}
		}
	}

	for _, cell := range req.CellTowers {
		cellID := fmt.Sprintf("%d:%d", cell.CellId, cell.Lac)
		obs, err := s.cache.AddObservation(ctx, req.ObjectId, "cell", cellID, req.Latitude, req.Longitude)
		if err == nil && obs != nil {
			if obs.Status == "STATIONARY" {
				stationarySources = append(stationarySources, cellID)
			} else if obs.Status == "RANDOM" {
				randomSources = append(randomSources, cellID)
			}
		}
	}

	result := pb.LearningResult_LEARNED
	if len(stationarySources) > 0 {
		result = pb.LearningResult_STATIONARY_DETECTED
	} else if len(randomSources) > 0 {
		result = pb.LearningResult_RANDOM_EXCLUDED
	} else if len(req.Wifi)+len(req.Bluetooth)+len(req.CellTowers) == 0 {
		result = pb.LearningResult_NEED_MORE_DATA
	}

	return &pb.LearnResponse{
		Result:            result,
		StationarySources: stationarySources,
		RandomSources:     randomSources,
	}, nil
}

func (s *ValidatorService) GetCompanionSources(ctx context.Context, req *pb.GetCompanionsRequest) (*pb.GetCompanionsResponse, error) {
	return &pb.GetCompanionsResponse{Companions: []*pb.CompanionSource{}}, nil
}

// ============ AbsoluteCoordinates Service ============

func (s *ValidatorService) SetAbsoluteCoordinates(ctx context.Context, req *pb.AbsoluteRequest) (*pb.AbsoluteResponse, error) {
	pointType := req.PointType.String()
	abs := &cache.AbsoluteCoordinates{
		Lat:       req.Latitude,
		Lon:       req.Longitude,
		Accuracy:  req.Accuracy,
		Source:    req.Source,
		Timestamp: time.Now(),
		ExpiresAt: time.Unix(req.ExpiresAt, 0),
	}
	err := s.cache.SetAbsolute(ctx, pointType, req.PointId, abs)
	return &pb.AbsoluteResponse{Success: err == nil}, err
}

func (s *ValidatorService) RemoveAbsoluteCoordinates(ctx context.Context, req *pb.RemoveRequest) (*pb.RemoveResponse, error) {
	pointType := req.PointType.String()
	err := s.cache.DeleteAbsolute(ctx, pointType, req.PointId)
	return &pb.RemoveResponse{Success: err == nil}, err
}

func (s *ValidatorService) GetPointInfo(ctx context.Context, req *pb.PointRequest) (*pb.PointInfoResponse, error) {
	pointType := req.PointType.String()
	abs, _ := s.cache.GetAbsolute(ctx, pointType, req.PointId)
	calc, _ := s.cache.GetCalculated(ctx, pointType, req.PointId)
	obs, _ := s.cache.GetObservation(ctx, "", pointType, req.PointId)

	resp := &pb.PointInfoResponse{}
	if abs != nil {
		resp.Absolute = &pb.AbsoluteCoordinates{
			Latitude:  abs.Lat,
			Longitude: abs.Lon,
			Accuracy:  abs.Accuracy,
			Source:    abs.Source,
			Timestamp: abs.Timestamp.Unix(),
			ExpiresAt: abs.ExpiresAt.Unix(),
		}
	}
	if calc != nil {
		resp.Calculated = &pb.CalculatedCoordinates{
			Latitude:     calc.Lat,
			Longitude:    calc.Lon,
			Confidence:   calc.Confidence,
			Observations: int32(calc.Observations),
			CalculatedAt: calc.CalculatedAt.Unix(),
		}
	}
	if obs != nil {
		resp.IsStationary = obs.Status == "STATIONARY"
	}
	return resp, nil
}

func (s *ValidatorService) GetExcludedPoints(ctx context.Context, req *pb.ExcludedRequest) (*pb.ExcludedResponse, error) {
	return &pb.ExcludedResponse{Sources: []*pb.ExcludedSource{}}, nil
}

// ============ AdminService ============

func (s *ValidatorService) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	cfg := s.cfg
	params := map[string]*pb.ConfigParameter{
		"maxSpeedKmH":   {Key: "maxSpeedKmH", Value: fmt.Sprintf("%.1f", cfg.MaxSpeedKmH), Description: "Max speed km/h", Category: "validation"},
		"maxTimeDiff":   {Key: "maxTimeDiff", Value: cfg.MaxTimeDiff.String(), Description: "Max time diff", Category: "validation"},
		"wifiWeight":    {Key: "wifiWeight", Value: fmt.Sprintf("%.2f", cfg.WifiWeight), Description: "WiFi weight", Category: "validation"},
		"cellWeight":    {Key: "cellWeight", Value: fmt.Sprintf("%.2f", cfg.CellWeight), Description: "Cell weight", Category: "validation"},
	}
	return &pb.GetConfigResponse{Parameters: params}, nil
}

func (s *ValidatorService) UpdateConfig(ctx context.Context, req *pb.UpdateConfigRequest) (*pb.UpdateConfigResponse, error) {
	return &pb.UpdateConfigResponse{Success: false, OldValue: "", NewValue: req.Value, ChangedAt: time.Now().Unix()}, fmt.Errorf("runtime config not implemented")
}

func (s *ValidatorService) ResetConfig(ctx context.Context, req *pb.ResetConfigRequest) (*pb.ResetConfigResponse, error) {
	return &pb.ResetConfigResponse{Success: false}, fmt.Errorf("reset not implemented")
}

func (s *ValidatorService) GetConfigHistory(ctx context.Context, req *pb.HistoryRequest) (*pb.HistoryResponse, error) {
	return &pb.HistoryResponse{Changes: []*pb.ConfigChange{}}, nil
}

// ============ MetricsService ============

func (s *ValidatorService) GetOverview(ctx context.Context, req *pb.OverviewRequest) (*pb.OverviewResponse, error) {
	return &pb.OverviewResponse{TotalRequests: 0, ValidCount: 0, InvalidCount: 0, UncertainCount: 0}, nil
}

func (s *ValidatorService) GetTimeSeries(ctx context.Context, req *pb.TimeSeriesRequest) (*pb.TimeSeriesResponse, error) {
	return &pb.TimeSeriesResponse{Points: []*pb.TimeSeriesPoint{}}, nil
}

func (s *ValidatorService) GetLatencyStats(ctx context.Context, req *pb.LatencyRequest) (*pb.LatencyResponse, error) {
	return &pb.LatencyResponse{AvgMs: 0, P50Ms: 0, P95Ms: 0, P99Ms: 0}, nil
}

// ============ StatisticsService ============

func (s *ValidatorService) GetSourceStats(ctx context.Context, req *pb.SourceStatsRequest) (*pb.SourceStatsResponse, error) {
	return &pb.SourceStatsResponse{Stats: []*pb.SourceTypeStats{}}, nil
}

func (s *ValidatorService) GetTopSources(ctx context.Context, req *pb.TopSourcesRequest) (*pb.TopSourcesResponse, error) {
	return &pb.TopSourcesResponse{Sources: []*pb.SourceInfo{}}, nil
}

func (s *ValidatorService) GetLearningProgress(ctx context.Context, req *pb.ProgressRequest) (*pb.ProgressResponse, error) {
	return &pb.ProgressResponse{TotalObservations: 0, StationarySources: 0, CoveragePercent: 0}, nil
}

func (s *ValidatorService) GetObjectStats(ctx context.Context, req *pb.ObjectStatsRequest) (*pb.ObjectStatsResponse, error) {
	return &pb.ObjectStatsResponse{TotalObjects: 0, ActiveObjects: 0, IdleObjects: 0, OfflineObjects: 0}, nil
}

// ============ Shutdown ============

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

// ============ Helpers ============

func calculateSpeedKmH(lat1, lon1 float64, time1 time.Time, lat2, lon2 float64, time2 time.Time) float64 {
	const R = 6371.0
	dLat := toRad(lat2 - lat1)
	dLon := toRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(toRad(lat1))*math.Cos(toRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
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

func (s *ValidatorService) recordWifiPointFromEGTS(ctx context.Context, bssid, ssid string, lat, lon float64, accuracy float32, rssi int32) {
	point := &cache.WifiPoint{Lat: lat, Lon: lon, LastSeen: time.Now(), Count: 1, SSID: ssid, EID: rssi}
	if err := s.cache.SetWifiPoint(ctx, bssid, point); err != nil {
		fmt.Printf("Warning: failed to save WiFi point: %v\n", err)
	}
	if err := s.storage.UpdatePointStats(ctx, "wifi", bssid, lat, lon, accuracy); err != nil {
		fmt.Printf("Warning: failed to update WiFi stats: %v\n", err)
	}
}

func (s *ValidatorService) recordCellPointFromEGTS(ctx context.Context, cellID, lac, mcc, mnc uint32, lat, lon float64, rssi int32) {
	point := &cache.CellPoint{Lat: lat, Lon: lon, LastSeen: time.Now(), LAC: lac, MCC: mcc, MNC: mnc, EID: rssi}
	if err := s.cache.SetCellPoint(ctx, cellID, lac, point); err != nil {
		fmt.Printf("Warning: failed to save cell point: %v\n", err)
	}
	if err := s.storage.UpdatePointStats(ctx, fmt.Sprintf("cell_%d_%d", mcc, mnc), fmt.Sprintf("%d:%d", cellID, lac), lat, lon, 0); err != nil {
		fmt.Printf("Warning: failed to update cell stats: %v\n", err)
	}
}

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
