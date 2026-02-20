package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	"coordinate-validator/internal/config"
	"coordinate-validator/internal/model"
	"coordinate-validator/internal/queue"
	"coordinate-validator/internal/storage"
	pb "coordinate-validator/pkg/pb"
)

type storageServer struct {
	pb.UnimplementedStorageServiceServer
	clickhouse *storage.ClickHouseStorage
	kafka      *queue.KafkaProducer
}

func main() {
	cfg := config.Load()

	// Initialize ClickHouse
	ch, err := storage.NewClickHouseStorage(&cfg.ClickHouse)
	if err != nil {
		log.Fatalf("Failed to connect to ClickHouse: %v", err)
	}
	defer ch.Close()

	// Initialize Kafka
	kafka := queue.NewKafkaProducer(&cfg.Kafka)
	defer kafka.Close()

	log.Printf("Storage Service started on port %s", cfg.Server.Port)

	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterStorageServiceServer(grpcServer, &storageServer{
		clickhouse: ch,
		kafka:      kafka,
	})

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down Storage Service...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// ============================================
// gRPC Handlers
// ============================================

func (s *storageServer) SaveValidation(ctx context.Context, req *pb.SaveValidationRequest) (*pb.SaveValidationResponse, error) {
	record := model.ValidationRecord{
		DeviceID:    req.DeviceId,
		Latitude:    req.Latitude,
		Longitude:   req.Longitude,
		Accuracy:    req.Accuracy,
		Timestamp:   req.Timestamp,
		HasWifi:     req.HasWifi,
		HasBT:       req.HasBt,
		HasCell:     req.HasCell,
		Result:      model.ValidationResult(req.Result),
		Confidence:  req.Confidence,
		FlowType:    req.FlowType,
		InsertTime:  now(),
	}

	// Queue for async write
	s.clickhouse.QueueValidation(record)

	// Send to Kafka for other consumers
	s.kafka.SendRefinementEvent(ctx, &model.RefinementEvent{
		DeviceID:   req.DeviceId,
		Latitude:   req.Latitude,
		Longitude: req.Longitude,
		Timestamp:  req.Timestamp,
		Result:     model.ValidationResult(req.Result),
		Confidence: req.Confidence,
		HasWifi:    req.HasWifi,
		HasBT:      req.HasBt,
		HasCell:    req.HasCell,
	})

	return &pb.SaveValidationResponse{Success: true}, nil
}

func (s *storageServer) SaveLearning(ctx context.Context, req *pb.SaveLearningRequest) (*pb.SaveLearningResponse, error) {
	s.kafka.SendLearningEvent(ctx, &model.LearningEvent{
		ObjectID:     req.ObjectId,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		Timestamp:    req.Timestamp,
		IsCompanion:  req.IsCompanion,
	})

	return &pb.SaveLearningResponse{Success: true}, nil
}

func now() interface{} {
	// Placeholder - implement proper time
	return nil
}
