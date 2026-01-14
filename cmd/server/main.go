package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"github.com/blacksheepaul/prompt_endgame/internal/wiring"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Wire dependencies
	container := wiring.Wire(cfg)

	// Start server in goroutine
	go func() {
		if err := container.HTTPServer.Start(); err != nil {
			log.Printf("Server error: %v\n", err)
		}
	}()

	fmt.Printf("Server running at http://localhost%s\n", cfg.Server.Addr)
	fmt.Println("Endpoints:")
	fmt.Println("  POST   /rooms           - Create a room")
	fmt.Println("  POST   /rooms/:id/answer - Submit an answer")
	fmt.Println("  GET    /rooms/:id/events - SSE event stream")
	fmt.Println("  POST   /rooms/:id/cancel - Cancel current turn")
	fmt.Println("  GET    /health          - Health check")

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\nShutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := container.HTTPServer.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	fmt.Println("Server exited")
}
