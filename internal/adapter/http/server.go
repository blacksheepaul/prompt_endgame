package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/app"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// Server represents the HTTP server
type Server struct {
	server   *http.Server
	handlers *Handlers
}

// NewServer creates a new HTTP server
func NewServer(addr string, roomService *app.RoomService, eventSink port.EventSink) *Server {
	handlers := NewHandlers(roomService, eventSink)

	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("POST /rooms", handlers.CreateRoom)
	mux.HandleFunc("POST /rooms/{id}/answer", handlers.SubmitAnswer)
	mux.HandleFunc("GET /rooms/{id}/events", handlers.StreamEvents)
	mux.HandleFunc("POST /rooms/{id}/cancel", handlers.CancelTurn)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	return &Server{
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 0, // No timeout for SSE
			IdleTimeout:  60 * time.Second,
		},
		handlers: handlers,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	fmt.Printf("Server starting on %s\n", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
