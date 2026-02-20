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
	pb "coordinate-validator/pkg/pb"
)

type refinementServer struct {
	pb.UnimplementedCoordinateValidatorServer
	validator *core.ValidationCore
	cache     *cache.RedisCache
}

func main() {
	cfg := config.Load()

	// Initialize Redis cache
	redisCache, err := cache.NewRedisCache(&cfg.Redis)
	if err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}
	defer redisCache.Close()

	log.Printf("Refinement API started on port %s", cfg.Server.Port)

	// Create validation core
	validationCore := core.NewValidationCore(redisCache, &cfg.Validation)

	// Create gRPC server
	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterCoordinateValidatorServer(grpcServer, &refinementServer{
		validator: validationCore,
		cache:     redisCache,
	})

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down Refinement API...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// ============================================
// gRPC Handlers
// ============================================

func (s *refinementServer) Validate(ctx context.Context, req *pb.CoordinateRequest) (*pb.CoordinateResponse, error) {
	// Convert from proto
	modelReq := &model.CoordinateRequest{
		DeviceID:   req.DeviceId,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		Accuracy:   req.Accuracy,
		Timestamp:  req.Timestamp,
		Wifi:       convertWifi(req.Wifi),
		Bluetooth:  convertBT(req.Bluetooth),
		CellTowers: convertCell(req.CellTowers),
	}

	// Validate
	resp, err := s.validator.Validate(ctx, modelReq)
	if err != nil {
		return nil, err
	}

	// Update device position for speed check
	s.validator.UpdateDevicePosition(ctx, modelReq)

	// Convert to proto
	return &pb.CoordinateResponse{
		Result:            convertValidationResult(resp.Result),
		Confidence:        resp.Confidence,
		EstimatedAccuracy: resp.EstimatedAccuracy,
		Reason:            resp.Reason,
	}, nil
}

func (s *refinementServer) ValidateBatch(stream pb.CoordinateValidator_ValidateBatchServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			break
		}

		modelReq := &model.CoordinateRequest{
			DeviceID:   req.DeviceId,
			Latitude:   req.Latitude,
			Longitude:  req.Longitude,
			Accuracy:   req.Accuracy,
			Timestamp:  req.Timestamp,
			Wifi:       convertWifi(req.Wifi),
			Bluetooth:  convertBT(req.Bluetooth),
			CellTowers: convertCell(req.CellTowers),
		}

		resp, err := s.validator.Validate(stream.Context(), modelReq)
		if err != nil {
			return err
		}

		s.validator.UpdateDevicePosition(stream.Context(), modelReq)

		pbResp := &pb.CoordinateResponse{
			Result:            convertValidationResult(resp.Result),
			Confidence:        resp.Confidence,
			EstimatedAccuracy: resp.EstimatedAccuracy,
			Reason:            resp.Reason,
		}

		if err := stream.Send(pbResp); err != nil {
			return err
		}
	}

	return nil
}

// ============================================
// Converters (placeholder - implement properly)
// ============================================

func convertWifi(wifi []*pb.WifiAccessPoint) []model.WifiAP {
	// TODO: implement
	return nil
}

func convertBT(bt []*pb.BluetoothDevice) []model.BluetoothDev {
	// TODO: implement
	return nil
}

func convertCell(cells []*pb.CellTower) []model.CellTower {
	// TODO: implement
	return nil
}

func convertValidationResult(r model.ValidationResult) pb.ValidationResult {
	// TODO: implement
	return pb.ValidationResult_VALID
}
