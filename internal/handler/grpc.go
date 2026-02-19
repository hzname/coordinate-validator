package handler

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"coordinate-validator/internal/cache"
	"coordinate-validator/internal/config"
	"coordinate-validator/internal/service"
	"coordinate-validator/internal/storage"
	pb "coordinate-validator/pkg/pb"
)

type GRPCHandler struct {
	server   *grpc.Server
	cache    *cache.RedisCache
	storage  *storage.ClickHouseStorage
	service  *service.ValidatorService
	cfg      config.ServerConfig
}

func NewGRPCHandler(cfg *config.Config) (*GRPCHandler, error) {
	// Initialize Redis
	redisCache, err := cache.NewRedisCache(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	// Initialize ClickHouse
	chStorage, err := storage.NewClickHouseStorage(cfg.ClickHouse)
	if err != nil {
		redisCache.Close()
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	// Create service
	validatorService := service.NewValidatorService(redisCache, chStorage, cfg.Validation)

	// Create gRPC server
	server := grpc.NewServer(
		grpc.MaxConcurrentStreams(1000),
	)

	// Register service
	pb.RegisterCoordinateValidatorServer(server, validatorService)

	// Health check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.Health_SERVING)

	// Reflection (for grpcurl)
	reflection.Register(server)

	return &GRPCHandler{
		server:   server,
		cache:    redisCache,
		storage:  chStorage,
		service:  validatorService,
		cfg:      cfg.Server,
	}, nil
}

func (h *GRPCHandler) Start() error {
	addr := fmt.Sprintf(":%s", h.cfg.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.Printf("gRPC server listening on %s", addr)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down gRPC server...")
		h.server.GracefulStop()
	}()

	if err := h.server.Serve(lis); err != nil {
		return fmt.Errorf("gRPC server error: %w", err)
	}

	return nil
}

func (h *GRPCHandler) Stop() {
	h.server.GracefulStop()
	h.cache.Close()
	h.storage.Close()
	log.Println("Server stopped")
}
