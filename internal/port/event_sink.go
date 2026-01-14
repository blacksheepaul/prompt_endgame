package port

import (
	"context"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// EventSink defines operations for event storage and replay
type EventSink interface {
	// Append adds an event to the stream and returns assigned offset
	Append(ctx context.Context, event domain.Event) (domain.Offset, error)

	// ReadFromOffset returns events starting from the given offset
	// Returns a channel that streams events for SSE
	ReadFromOffset(ctx context.Context, roomID domain.RoomID, offset domain.Offset) (<-chan domain.Event, error)

	// Subscribe returns a channel for real-time events
	Subscribe(ctx context.Context, roomID domain.RoomID) (<-chan domain.Event, func())
}
