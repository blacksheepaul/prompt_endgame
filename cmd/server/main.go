package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"github.com/blacksheepaul/prompt_endgame/internal/wiring"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	logger, err := wiring.NewLogger(cfg)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Wire dependencies
	container := wiring.Wire(cfg, logger)

	// Start server in goroutine
	go func() {
		if err := container.HTTPServer.Start(); err != nil {
			logger.Error("Server error", zap.Error(err))
		}
	}()

	logger.Info("Server running",
		zap.String("url", "http://localhost"+cfg.Server.Addr),
	)
	logger.Info("Endpoints",
		zap.String("create_room", "POST   /rooms"),
		zap.String("submit_answer", "POST   /rooms/:id/answer"),
		zap.String("stream_events", "GET    /rooms/:id/events"),
		zap.String("cancel_turn", "POST   /rooms/:id/cancel"),
		zap.String("list_rooms", "GET    /supervisor/rooms"),
		zap.String("health_check", "GET    /health"),
		zap.String("metrics", "GET    /metrics"),
	)

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := container.HTTPServer.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exited")
}
