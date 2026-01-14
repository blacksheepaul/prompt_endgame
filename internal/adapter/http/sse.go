package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// parseOffset extracts offset from request (query param or Last-Event-ID header)
func parseOffset(r *http.Request) domain.Offset {
	// Try query parameter first
	if offsetStr := r.URL.Query().Get("fromOffset"); offsetStr != "" {
		if offset, err := strconv.ParseInt(offsetStr, 10, 64); err == nil {
			return domain.Offset(offset)
		}
	}

	// Fall back to Last-Event-ID header
	if lastEventID := r.Header.Get("Last-Event-ID"); lastEventID != "" {
		if offset, err := strconv.ParseInt(lastEventID, 10, 64); err == nil {
			return domain.Offset(offset + 1) // Resume from next event
		}
	}

	return 0
}

// streamSSE handles the SSE streaming
func streamSSE(w http.ResponseWriter, r *http.Request, eventSink port.EventSink, roomID domain.RoomID, offset domain.Offset) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Get flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	// Flush headers
	flusher.Flush()

	ctx := r.Context()

	// Read historical events first
	historyCh, err := eventSink.ReadFromOffset(ctx, roomID, offset)
	if err != nil {
		writeSSEError(w, flusher, "failed to read history")
		return
	}

	// Stream historical events
	for event := range historyCh {
		if err := writeSSEEvent(w, flusher, event); err != nil {
			return
		}
	}

	// Subscribe to live events
	liveCh, unsubscribe := eventSink.Subscribe(ctx, roomID)
	defer unsubscribe()

	// Stream live events
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-liveCh:
			if !ok {
				return
			}
			if err := writeSSEEvent(w, flusher, event); err != nil {
				return
			}
		}
	}
}

func writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event domain.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}

	// Write SSE formatted event
	fmt.Fprintf(w, "id: %d\n", event.Offset)
	fmt.Fprintf(w, "event: %s\n", event.Type)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	return nil
}

func writeSSEError(w http.ResponseWriter, flusher http.Flusher, message string) {
	fmt.Fprintf(w, "event: error\n")
	fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", message)
	flusher.Flush()
}

// KeepAlive sends periodic keep-alive comments (optional enhancement)
func keepAlive(ctx context.Context, w http.ResponseWriter, flusher http.Flusher) {
	// Can be used to prevent connection timeout
	// fmt.Fprintf(w, ": keep-alive\n\n")
	// flusher.Flush()
}
