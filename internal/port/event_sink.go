package port

import (
	"context"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// EventSink defines operations for event storage and replay
type EventSink interface {
	// Append adds an event to the stream and returns assigned offset
	Append(ctx context.Context, event domain.Event) (domain.Offset, error)

	// ReadFromOffsetAndSubscribe returns a stable snapshot plus live updates.
	// It must subscribe atomically with the snapshot to avoid gaps.
	ReadFromOffsetAndSubscribe(ctx context.Context, roomID domain.RoomID, offset domain.Offset) ([]domain.Event, <-chan domain.Event, func(), error)

	// Subscribe returns a channel for real-time events
	Subscribe(ctx context.Context, roomID domain.RoomID) (<-chan domain.Event, func())
}
