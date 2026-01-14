package port

import (
	"context"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// RoomRepository defines operations for room persistence
type RoomRepository interface {
	// Save persists a room
	Save(ctx context.Context, room *domain.Room) error

	// Get retrieves a room by ID
	Get(ctx context.Context, id domain.RoomID) (*domain.Room, error)

	// Update updates an existing room
	// Update(ctx context.Context, room *domain.Room) error

	// Update updates a room by applying fn within a lock (thread-safe)
	Update(ctx context.Context, id domain.RoomID, fn func(*domain.Room) error) error

	// Delete removes a room
	Delete(ctx context.Context, id domain.RoomID) error
}
