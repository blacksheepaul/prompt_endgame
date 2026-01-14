package http

import (
	"encoding/json"
	"net/http"

	"github.com/blacksheepaul/prompt_endgame/internal/app"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

// Handlers holds all HTTP handlers
type Handlers struct {
	roomService *app.RoomService
	eventSink   port.EventSink
}

// NewHandlers creates new handlers
func NewHandlers(roomService *app.RoomService, eventSink port.EventSink) *Handlers {
	return &Handlers{
		roomService: roomService,
		eventSink:   eventSink,
	}
}

// CreateRoomRequest is the request body for creating a room
type CreateRoomRequest struct {
	SceneryID string `json:"scenery_id"`
}

// CreateRoomResponse is the response for room creation
type CreateRoomResponse struct {
	ID        string `json:"id"`
	SceneryID string `json:"scenery_id"`
	State     string `json:"state"`
}

// AnswerRequest is the request body for submitting an answer
type AnswerRequest struct {
	UserInput string `json:"user_input"`
}

// AnswerResponse is the response for answer submission
type AnswerResponse struct {
	TurnID string `json:"turn_id"`
	Round  int    `json:"round"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error string `json:"error"`
}

// CreateRoom handles POST /rooms
func (h *Handlers) CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req CreateRoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body, use default scenery
		req.SceneryID = "default"
	}

	room, err := h.roomService.CreateRoom(r.Context(), req.SceneryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, CreateRoomResponse{
		ID:        room.ID.String(),
		SceneryID: room.SceneryID,
		State:     string(room.State),
	})
}

// SubmitAnswer handles POST /rooms/{id}/answer
func (h *Handlers) SubmitAnswer(w http.ResponseWriter, r *http.Request) {
	roomID := domain.RoomID(r.PathValue("id"))

	var req AnswerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.UserInput == "" {
		writeError(w, http.StatusBadRequest, "user_input is required")
		return
	}

	turn, err := h.roomService.SubmitAnswer(r.Context(), roomID, req.UserInput)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case app.ErrRoomNotFound:
			status = http.StatusNotFound
		case app.ErrRoomBusy:
			status = http.StatusConflict
		}
		writeError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, AnswerResponse{
		TurnID: turn.ID.String(),
		Round:  turn.Round,
	})
}

// StreamEvents handles GET /rooms/{id}/events (SSE)
func (h *Handlers) StreamEvents(w http.ResponseWriter, r *http.Request) {
	roomID := domain.RoomID(r.PathValue("id"))

	// Check room exists
	_, err := h.roomService.GetRoom(r.Context(), roomID)
	if err != nil {
		writeError(w, http.StatusNotFound, "room not found")
		return
	}

	// Parse offset from query or Last-Event-ID header
	offset := parseOffset(r)

	// Start SSE stream
	streamSSE(w, r, h.eventSink, roomID, offset)
}

// CancelTurn handles POST /rooms/{id}/cancel
func (h *Handlers) CancelTurn(w http.ResponseWriter, r *http.Request) {
	roomID := domain.RoomID(r.PathValue("id"))

	err := h.roomService.CancelTurn(r.Context(), roomID)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case app.ErrRoomNotFound:
			status = http.StatusNotFound
		case app.ErrNoActiveTurn:
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}
