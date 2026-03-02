package http

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/app"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	mux.HandleFunc("GET /supervisor/rooms", handlers.SupervisorRooms)

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Metrics endpoint for Prometheus (localhost only for security)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
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

// isLocalhost checks if the remote address is localhost
func isLocalhost(remoteAddr string) bool {
	// RemoteAddr format: "IP:port" or "[IPv6]:port"
	if remoteAddr == "" {
		return false
	}

	// Handle IPv4-mapped IPv6 addresses
	if strings.HasPrefix(remoteAddr, "[") {
		// IPv6 format: [::1]:port
		end := strings.LastIndex(remoteAddr, "]")
		if end == -1 {
			return false
		}
		host := remoteAddr[1:end]
		return host == "::1" || host == "::ffff:127.0.0.1"
	}

	// IPv4 format: 127.0.0.1:port
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// Try as plain IP (for testing)
		host = remoteAddr
	}

	return host == "127.0.0.1" || host == "localhost" || host == "::1"
}
