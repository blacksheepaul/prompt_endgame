package domain

import "time"

// EventType identifies the type of event
type EventType string

const (
	EventRoomCreated   EventType = "room_created"
	EventTurnStarted   EventType = "turn_started"
	EventTokenReceived EventType = "token_received"
	EventTurnCompleted EventType = "turn_completed"
	EventTurnCancelled EventType = "turn_cancelled"
	EventError         EventType = "error"
)

// Event represents a domain event in the system
type Event struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	RoomID    RoomID    `json:"room_id"`
	TurnID    TurnID    `json:"turn_id,omitempty"`
	Offset    Offset    `json:"offset"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload,omitempty"`
}

// TokenPayload contains streaming token data
type TokenPayload struct {
	AgentID string `json:"agent_id"`
	Token   string `json:"token"`
}

// ErrorPayload contains error information
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// NewEvent creates a new event with current timestamp
func NewEvent(eventType EventType, roomID RoomID, turnID TurnID, payload any) Event {
	return Event{
		ID:        NewTurnID().String(), // reuse UUID generator
		Type:      eventType,
		RoomID:    roomID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	}
}
