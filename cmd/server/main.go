package main

import (
	"log"

	"coordinate-validator/internal/config"
	"coordinate-validator/internal/handler"
)

func main() {
	// Load configuration
	cfg := config.Load()

	log.Printf("Starting coordinate-validator with config:")
	log.Printf("  Server port: %s", cfg.Server.Port)
	log.Printf("  Redis: %s", cfg.Redis.Addr)
	log.Printf("  ClickHouse: %s", cfg.ClickHouse.Addr)
	log.Printf("  Max speed: %.1f km/h", cfg.Validation.MaxSpeedKmH)
	log.Printf("  Max time diff: %v", cfg.Validation.MaxTimeDiff)

	// Create handler
	h, err := handler.NewGRPCHandler(cfg)
	if err != nil {
		log.Fatalf("Failed to create handler: %v", err)
	}

	// Start server (blocks until shutdown)
	// Resources are closed inside Start() after graceful shutdown
	if err := h.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server exited cleanly")
}
