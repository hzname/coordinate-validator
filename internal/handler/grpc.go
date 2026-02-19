package handler

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

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
	server       *grpc.Server
	cache        *cache.RedisCache
	storage      *storage.ClickHouseStorage
	service      *service.ValidatorService
	cfg          config.ServerConfig
	shutdownChan chan struct{}
}

func NewGRPCHandler(cfg *config.Config) (*GRPCHandler, error) {
	redisCache, err := cache.NewRedisCache(cfg.Redis)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	chStorage, err := storage.NewClickHouseStorage(cfg.ClickHouse)
	if err != nil {
		redisCache.Close()
		return nil, fmt.Errorf("failed to connect to ClickHouse: %w", err)
	}

	validatorService := service.NewValidatorService(redisCache, chStorage, cfg.Validation)

	server := grpc.NewServer(
		grpc.MaxConcurrentStreams(1000),
	)

	// Register all 6 services
	pb.RegisterCoordinateValidatorServer(server, validatorService)
	pb.RegisterLearningServiceServer(server, validatorService)
	pb.RegisterAbsoluteCoordinatesServer(server, validatorService)
	pb.RegisterAdminServiceServer(server, validatorService)
	pb.RegisterMetricsServiceServer(server, validatorService)
	pb.RegisterStatisticsServiceServer(server, validatorService)

	// Health check
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthServer.SetServingStatus("", grpc_health_v1.Health_SERVING)

	reflection.Register(server)

	return &GRPCHandler{
		server:       server,
		cache:        redisCache,
		storage:      chStorage,
		service:      validatorService,
		cfg:          cfg.Server,
		shutdownChan: make(chan struct{}),
	}, nil
}

func (h *GRPCHandler) Start() error {
	addr := fmt.Sprintf(":%s", h.cfg.Port)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	log.Printf("gRPC server listening on %s", addr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Received shutdown signal, stopping server...")
		h.server.GracefulStop()

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := h.service.Shutdown(ctx); err != nil {
			log.Printf("Warning: service shutdown timed out: %v", err)
		}

		h.cache.Close()
		h.storage.Close()

		close(h.shutdownChan)
		log.Println("Server stopped gracefully")
	}()

	if err := h.server.Serve(lis); err != nil && err != grpc.ErrServerStopped {
		return fmt.Errorf("gRPC server error: %w", err)
	}

	<-h.shutdownChan
	return nil
}

func (h *GRPCHandler) Stop() {
	h.server.GracefulStop()
	h.cache.Close()
	h.storage.Close()
	log.Println("Server stopped")
}
