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
	pb "coordinate-validator/pkg/pb"
)

type gatewayServer struct {
	pb.UnimplementedCoordinateValidatorServer
	pb.UnimplementedLearningServiceServer

	refinementAddr string
	learningAddr   string
}

func main() {
	cfg := config.Load()

	refinementAddr := cfg.Server.RefinementAddr // e.g., "localhost:50051"
	learningAddr := cfg.Server.LearningAddr     // e.g., "localhost:50052"

	log.Printf("API Gateway started on port %s", cfg.Server.Port)
	log.Printf("  -> Refinement API: %s", refinementAddr)
	log.Printf("  -> Learning API: %s", learningAddr)

	lis, err := net.Listen("tcp", ":"+cfg.Server.Port)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterCoordinateValidatorServer(grpcServer, &gatewayServer{
		refinementAddr: refinementAddr,
		learningAddr:   learningAddr,
	})
	pb.RegisterLearningServiceServer(grpcServer, &gatewayServer{})

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down Gateway...")
		grpcServer.GracefulStop()
	}()

	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

// ============================================
// Refinement API Routing
// ============================================

func (s *gatewayServer) Validate(ctx context.Context, req *pb.CoordinateRequest) (*pb.CoordinateResponse, error) {
	// Connect to Refinement API
	conn, err := grpc.Dial(s.refinementAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewCoordinateValidatorClient(conn)
	return client.Validate(ctx, req)
}

func (s *gatewayServer) ValidateBatch(stream pb.CoordinateValidator_ValidateBatchServer) error {
	conn, err := grpc.Dial(s.refinementAddr, grpc.WithInsecure())
	if err != nil {
		return err
	}
	defer conn.Close()

	client := pb.NewCoordinateValidatorClient(conn)
	return client.ValidateBatch(stream.Context(), stream)
}

// ============================================
// Learning API Routing
// ============================================

func (s *gatewayServer) LearnFromCoordinates(ctx context.Context, req *pb.LearnRequest) (*pb.LearnResponse, error) {
	conn, err := grpc.Dial(s.learningAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewLearningServiceClient(conn)
	return client.LearnFromCoordinates(ctx, req)
}

func (s *gatewayServer) GetCompanionSources(ctx context.Context, req *pb.GetCompanionsRequest) (*pb.GetCompanionsResponse, error) {
	conn, err := grpc.Dial(s.learningAddr, grpc.WithInsecure())
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	client := pb.NewLearningServiceClient(conn)
	return client.GetCompanionSources(ctx, req)
}
