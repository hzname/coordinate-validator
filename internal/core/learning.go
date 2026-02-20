package core

import (
	"context"
	"log"
	"math"
	"time"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
)

type LearningCore struct {
	cache *cache.RedisCache
	cfg   *config.ValidationConfig
}

func NewLearningCore(cache *cache.RedisCache, cfg *config.ValidationConfig) *LearningCore {
	return &LearningCore{
		cache: cache,
		cfg:   cfg,
	}
}

// ============================================
// Main Learning Flow
// ============================================

func (l *LearningCore) Learn(ctx context.Context, req *model.LearnRequest) (*model.LearnResponse, error) {
	// Get existing companions for this object
	companions, err := l.cache.GetCompanions(ctx, req.ObjectID)
	if err != nil {
		return nil, err
	}

	// Detect companions from current data
	detectedCompanions := l.detectCompanions(req)

	// Add new companions to cache
	for _, c := range detectedCompanions {
		err := l.cache.AddCompanion(ctx, req.ObjectID, c.PointID, c.PointType)
		if err != nil {
			log.Printf("Warning: failed to add companion: %v", err)
		}
	}

	// Update coordinates for detected companions
	var stationarySources []string
	var randomSources []string

	// Process WiFi
	for _, w := range req.Wifi {
		isCompanion := l.isCompanion(detectedCompanions, w.BSSID, model.PointTypeWifi)
		l.updateWifiCoordinates(ctx, req, &w, isCompanion)
		if isCompanion {
			stationarySources = append(stationarySources, w.BSSID)
		} else {
			randomSources = append(randomSources, w.BSSID)
		}
	}

	// Process Cell Towers
	for _, c := range req.CellTowers {
		key := keyFromCell(c.CellID, c.LAC)
		isCompanion := l.isCompanion(detectedCompanions, key, model.PointTypeCell)
		l.updateCellCoordinates(ctx, req, &c, isCompanion)
		if isCompanion {
			stationarySources = append(stationarySources, key)
		} else {
			randomSources = append(randomSources, key)
		}
	}

	// Process Bluetooth
	for _, b := range req.Bluetooth {
		isCompanion := l.isCompanion(detectedCompanions, b.MAC, model.PointTypeBT)
		l.updateBTCoordinates(ctx, req, &b, isCompanion)
		if isCompanion {
			stationarySources = append(stationarySources, b.MAC)
		} else {
			randomSources = append(randomSources, b.MAC)
		}
	}

	// Determine result
	result := l.determineLearningResult(len(stationarySources), len(randomSources))

	return &model.LearnResponse{
		Result:            result,
		StationarySources: stationarySources,
		RandomSources:    randomSources,
	}, nil
}

// ============================================
// Companion Detection (Co-occurrence Analysis)
// ============================================

func (l *LearningCore) detectCompanions(req *model.LearnRequest) []model.CompanionSource {
	var sources []model.CompanionSource

	// WiFi sources
	for _, w := range req.Wifi {
		sources = append(sources, model.CompanionSource{
			PointID:   w.BSSID,
			PointType: model.PointTypeWifi,
		})
	}

	// Cell towers
	for _, c := range req.CellTowers {
		sources = append(sources, model.CompanionSource{
			PointID:   keyFromCell(c.CellID, c.LAC),
			PointType: model.PointTypeCell,
		})
	}

	// Bluetooth
	for _, b := range req.Bluetooth {
		sources = append(sources, model.CompanionSource{
			PointID:   b.MAC,
			PointType: model.PointTypeBT,
		})
	}

	// Simple heuristic: devices seen together frequently are companions
	// In production, this would use more sophisticated ML
	return sources
}

func (l *LearningCore) isCompanion(companions []model.CompanionSource, pointID string, pointType model.PointType) bool {
	for _, c := range companions {
		if c.PointID == pointID && c.PointType == pointType {
			return true
		}
	}
	return false
}

// ============================================
// Update Cached Coordinates
// ============================================

func (l *LearningCore) updateWifiCoordinates(ctx context.Context, req *model.LearnRequest, wifi *model.WifiAP, isCompanion bool) {
	existing, _ := l.cache.GetWifi(ctx, wifi.BSSID)

	if existing == nil {
		// New WiFi - create entry
		l.cache.SetWifi(ctx, &model.CachedWifi{
			BSSID:      wifi.BSSID,
			Latitude:   req.Latitude,
			Longitude:  req.Longitude,
			LastSeen:   time.Now(),
			Version:    1,
			ObsCount:   1,
			Confidence: 0.3,
		})
	} else {
		// Update existing - weighted average
		weight := 0.1 // decay factor
		if isCompanion {
			weight = 0.2 // companion updates faster
		}

		newLat := existing.Latitude*(1-weight) + req.Latitude*weight
		newLon := existing.Longitude*(1-weight) + req.Longitude*weight
		obsCount := existing.ObsCount + 1
		confidence := calculateConfidence(obsCount)

		l.cache.SetWifi(ctx, &model.CachedWifi{
			BSSID:      wifi.BSSID,
			Latitude:   newLat,
			Longitude:  newLon,
			LastSeen:   time.Now(),
			Version:    existing.Version + 1,
			ObsCount:   obsCount,
			Confidence: confidence,
		})
	}
}

func (l *LearningCore) updateCellCoordinates(ctx context.Context, req *model.LearnRequest, cell *model.CellTower, isCompanion bool) {
	key := keyFromCell(cell.CellID, cell.LAC)
	existing, _ := l.cache.GetCell(ctx, cell.CellID, cell.LAC)

	if existing == nil {
		l.cache.SetCell(ctx, &model.CachedCell{
			CellID:    cell.CellID,
			LAC:       cell.LAC,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			Version:   1,
			ObsCount:  1,
			Confidence: 0.3,
		})
	} else {
		weight := 0.1
		if isCompanion {
			weight = 0.2
		}

		newLat := existing.Latitude*(1-weight) + req.Latitude*weight
		newLon := existing.Longitude*(1-weight) + req.Longitude*weight
		obsCount := existing.ObsCount + 1
		confidence := calculateConfidence(obsCount)

		l.cache.SetCell(ctx, &model.CachedCell{
			CellID:    cell.CellID,
			LAC:       cell.LAC,
			Latitude:  newLat,
			Longitude: newLon,
			Version:   existing.Version + 1,
			ObsCount:  obsCount,
			Confidence: confidence,
		})
	}
}

func (l *LearningCore) updateBTCoordinates(ctx context.Context, req *model.LearnRequest, bt *model.BluetoothDev, isCompanion bool) {
	existing, _ := l.cache.GetBT(ctx, bt.MAC)

	if existing == nil {
		l.cache.SetBT(ctx, &model.CachedBT{
			MAC:       bt.MAC,
			Latitude:  req.Latitude,
			Longitude: req.Longitude,
			LastSeen:  time.Now(),
			Version:   1,
			ObsCount:  1,
			Confidence: 0.3,
		})
	} else {
		weight := 0.1
		if isCompanion {
			weight = 0.2
		}

		newLat := existing.Latitude*(1-weight) + req.Latitude*weight
		newLon := existing.Longitude*(1-weight) + req.Longitude*weight
		obsCount := existing.ObsCount + 1
		confidence := calculateConfidence(obsCount)

		l.cache.SetBT(ctx, &model.CachedBT{
			MAC:       bt.MAC,
			Latitude:  newLat,
			Longitude: newLon,
			LastSeen:  time.Now(),
			Version:   existing.Version + 1,
			ObsCount:  obsCount,
			Confidence: confidence,
		})
	}
}

// ============================================
// Helpers
// ============================================

func calculateConfidence(obsCount int64) float64 {
	// Confidence grows logarithmically with observations
	// Max confidence ~0.95 at 1000 observations
	const maxObs = 1000.0
	const maxConf = 0.95

	if obsCount <= 1 {
		return 0.3
	}

	conf := maxConf * (1 - math.Exp(-float64(obsCount)/maxObs*5))
	return conf
}

func keyFromCell(cellID uint32, lac uint32) string {
	return string(rune(cellID)) + ":" + string(rune(lac))
}

func (l *LearningCore) determineLearningResult(stationaryCount, randomCount int) model.LearningResult {
	if stationaryCount == 0 && randomCount == 0 {
		return model.LearningResultNeedMoreData
	}
	if randomCount > stationaryCount*2 {
		return model.LearningResultRandomExcluded
	}
	if stationaryCount > 0 {
		return model.LearningResultStationary
	}
	return model.LearningResultLeared
}
