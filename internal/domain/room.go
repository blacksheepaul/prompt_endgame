package domain

import "time"

// Room represents a conversation room
type Room struct {
	ID          RoomID    `json:"id"`
	SceneryID   string    `json:"scenery_id"`
	State       RoomState `json:"state"`
	CurrentTurn *Turn     `json:"current_turn,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Turn represents a single conversation turn
type Turn struct {
	ID        TurnID     `json:"id"`
	RoomID    RoomID     `json:"room_id"`
	Round     int        `json:"round"`
	UserInput string     `json:"user_input"`
	Responses []Response `json:"responses,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// Response holds an agent's response
type Response struct {
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
}

// NewRoom creates a new room with the given scenery
func NewRoom(sceneryID string) *Room {
	now := time.Now()
	return &Room{
		ID:        NewRoomID(),
		SceneryID: sceneryID,
		State:     RoomStateIdle,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewTurn creates a new turn for the room
func NewTurn(roomID RoomID, round int, userInput string) *Turn {
	return &Turn{
		ID:        NewTurnID(),
		RoomID:    roomID,
		Round:     round,
		UserInput: userInput,
		CreatedAt: time.Now(),
	}
}

// IsStreaming checks if the room is currently streaming
func (r *Room) IsStreaming() bool {
	return r.State == RoomStateStreaming
}

// CanStartTurn checks if a new turn can be started
func (r *Room) CanStartTurn() bool {
	return r.State == RoomStateIdle || r.State == RoomStateDone
}
