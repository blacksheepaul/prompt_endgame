package domain

import (
	"github.com/google/uuid"
)

// RoomID uniquely identifies a room
type RoomID string

// TurnID uniquely identifies a turn within a room
type TurnID string

// Offset represents position in event stream for replay
type Offset int64

// RoomState represents the current state of a room
type RoomState string

const (
	RoomStateIdle      RoomState = "idle"
	RoomStateStreaming RoomState = "streaming"
	RoomStateCancelled RoomState = "cancelled"
	RoomStateEndgame   RoomState = "endgame" // the game has ended but the room is still active for replay or review
)

// TurnState represents the current state of a turn
type TurnState string

const (
	TurnStatePending   TurnState = "pending" // turn is created but not started yet
	TurnStateStreaming TurnState = "streaming"
	TurnStateDone      TurnState = "done" // turn has completed
	TurnStateCancelled TurnState = "cancelled"
)

// NewRoomID generates a new unique RoomID
func NewRoomID() RoomID {
	return RoomID(uuid.New().String())
}

// NewTurnID generates a new unique TurnID
func NewTurnID() TurnID {
	return TurnID(uuid.New().String())
}

// String returns the string representation
func (id RoomID) String() string { return string(id) }
func (id TurnID) String() string { return string(id) }
