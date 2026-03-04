package http

import (
	"context"
	"net"
	"net/http"
	"net/http/pprof"
	"strings"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/app"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// Server represents the HTTP server
type Server struct {
	server   *http.Server
	handlers *Handlers
	logger   *zap.Logger
}

// NewServer creates a new HTTP server
func NewServer(addr string, roomService *app.RoomService, eventSink port.EventSink, logger *zap.Logger) *Server {
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

	// Metrics endpoint for Prometheus (private networks only for security)
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		if !isPrivateNetwork(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		promhttp.Handler().ServeHTTP(w, r)
	})

	// pprof endpoints for profiling (localhost only for security)
	mux.HandleFunc("GET /debug/pprof/", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Index(w, r)
	})
	mux.HandleFunc("GET /debug/pprof/cmdline", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Cmdline(w, r)
	})
	mux.HandleFunc("GET /debug/pprof/profile", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Profile(w, r)
	})
	mux.HandleFunc("GET /debug/pprof/symbol", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Symbol(w, r)
	})
	mux.HandleFunc("GET /debug/pprof/trace", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Trace(w, r)
	})
	mux.HandleFunc("GET /debug/pprof/{name}", func(w http.ResponseWriter, r *http.Request) {
		if !isLocalhost(r.RemoteAddr) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		pprof.Handler(r.PathValue("name")).ServeHTTP(w, r)
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
		logger:   logger,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	s.logger.Info("Server starting", zap.String("addr", s.server.Addr))
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

// isPrivateNetwork checks if the remote address is from a private network
// Allows localhost and private IPs (for Docker/internal networks)
func isPrivateNetwork(remoteAddr string) bool {
	if isLocalhost(remoteAddr) {
		return true
	}

	// Handle IPv4-mapped IPv6 addresses like [::ffff:172.17.0.1]:port
	if strings.HasPrefix(remoteAddr, "[") {
		end := strings.LastIndex(remoteAddr, "]")
		if end != -1 {
			host := remoteAddr[1:end]
			// Check for IPv4-mapped IPv6 format
			if strings.HasPrefix(host, "::ffff:") {
				ipv4 := strings.TrimPrefix(host, "::ffff:")
				if isPrivateIPv4(ipv4) {
					return true
				}
			}
		}
	}

	// Extract host from "IP:port" format
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	// Check if it's a private IPv4 address
	if isPrivateIPv4(host) {
		return true
	}

	// Check if it's a private IPv6 address
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}

	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	return false
}

// isPrivateIPv4 checks if an IPv4 address is in a private range
func isPrivateIPv4(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil || ip.To4() == nil {
		return false
	}

	// Use the standard library's IsPrivate method (Go 1.17+)
	return ip.IsPrivate()
}
