package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/core"
	"coordinate-validator/internal/model"
	"coordinate-validator/internal/storage"
	pb "coordinate-validator/pkg/pb"
)

type learningServer struct {
	pb.UnimplementedLearningServiceServer
	learningCore *core.LearningCore
	cache        *cache.RedisCache
	storage      *storage.ClickHouseStorage
}

func main() {
	cfg := config.Load()

	// Initialize Redis
	redisCache, err := cache.NewRedisCache(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()

	// Initialize ClickHouse storage
	chStorage, err := storage.NewClickHouseStorage(&cfg.ClickHouse)
	if err != nil {
		log.Fatalf("Failed to connect to ClickHouse: %v", err)
	}
	defer chStorage.Close()

	log.Printf("Learning API started on port %s", cfg.Server.Port)

	// Create learning core
	learningCore := core.NewLearningCore(redisCache, &cfg.Validation)

	// Create gRPC server
	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterLearningServiceServer(grpcServer, &learningServer{
		learningCore: learningCore,
		cache:        redisCache,
		storage:      chStorage,
	})

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down Learning API...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// ============================================
// gRPC Handlers
// ============================================

func (s *learningServer) LearnFromCoordinates(ctx context.Context, req *pb.LearnRequest) (*pb.LearnResponse, error) {
	modelReq := &model.LearnRequest{
		ObjectID:   req.ObjectId,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
		Accuracy:  req.Accuracy,
		Timestamp: req.Timestamp,
		// TODO: convert Wifi, Bluetooth, CellTowers
	}

	resp, err := s.learningCore.Learn(ctx, modelReq)
	if err != nil {
		return nil, err
	}

	return &pb.LearnResponse{
		Result:            convertLearningResult(resp.Result),
		StationarySources: resp.StationarySources,
		RandomSources:     resp.RandomSources,
	}, nil
}

func (s *learningServer) GetCompanionSources(ctx context.Context, req *pb.GetCompanionsRequest) (*pb.GetCompanionsResponse, error) {
	companions, err := s.cache.GetCompanions(ctx, req.ObjectId)
	if err != nil {
		return nil, err
	}

	pbCompanions := make([]*pb.CompanionSource, len(companions))
	for i, c := range companions {
		pbCompanions[i] = &pb.CompanionSource{
			PointId:       c.PointID,
			PointType:     convertPointType(c.PointType),
			Observations:  c.Observations,
			Stability:     c.Stability,
			IsStationary:  c.IsStationary,
			FirstSeen:     c.FirstSeen,
			LastSeen:      c.LastSeen,
		}
	}

	return &pb.GetCompanionsResponse{Companions: pbCompanions}, nil
}

// ============================================
// Converters
// ============================================

func convertLearningResult(r model.LearningResult) pb.LearningResult {
	switch r {
	case model.LearningResultLeared:
		return pb.LearningResult_LEARNED
	case model.LearningResultNeedMoreData:
		return pb.LearningResult_NEED_MORE_DATA
	case model.LearningResultStationary:
		return pb.LearningResult_STATIONARY_DETECTED
	case model.LearningResultRandomExcluded:
		return pb.LearningResult_RANDOM_EXCLUDED
	default:
		return pb.LearningResult_LEARNED
	}
}

func convertPointType(pt model.PointType) pb.PointType {
	switch pt {
	case model.PointTypeWifi:
		return pb.PointType_WIFI
	case model.PointTypeCell:
		return pb.PointType_CELL
	case model.PointTypeBT:
		return pb.PointType_BLE
	default:
		return pb.PointType_POINT_TYPE_UNSPECIFIED
	}
}
