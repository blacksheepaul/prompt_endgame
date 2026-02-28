package inmem

import (
	"context"
	"sync"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// EventSink implements port.EventSink with in-memory storage
type EventSink struct {
	mu          sync.RWMutex
	events      map[domain.RoomID][]domain.Event
	subscribers map[domain.RoomID][]chan domain.Event
}

// NewEventSink creates a new in-memory event sink
func NewEventSink() *EventSink {
	return &EventSink{
		events:      make(map[domain.RoomID][]domain.Event),
		subscribers: make(map[domain.RoomID][]chan domain.Event),
	}
}

func (s *EventSink) Append(ctx context.Context, event domain.Event) (domain.Offset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	roomEvents := s.events[event.RoomID]
	offset := domain.Offset(len(roomEvents))
	event.Offset = offset
	s.events[event.RoomID] = append(roomEvents, event)

	// Notify subscribers
	for _, ch := range s.subscribers[event.RoomID] {
		select {
		case ch <- event:
		default:
			// Skip if channel is full
		}
	}

	return offset, nil
}

func (s *EventSink) ReadFromOffsetAndSubscribe(ctx context.Context, roomID domain.RoomID, offset domain.Offset) ([]domain.Event, <-chan domain.Event, func(), error) {
	s.mu.Lock()
	roomEvents := s.events[roomID]
	snapshot := make([]domain.Event, 0, len(roomEvents))
	if int(offset) < len(roomEvents) {
		snapshot = append(snapshot, roomEvents[int(offset):]...)
	}
	ch := make(chan domain.Event, 100)
	s.subscribers[roomID] = append(s.subscribers[roomID], ch)
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.subscribers[roomID]
		for i, sub := range subs {
			if sub == ch {
				s.subscribers[roomID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return snapshot, ch, unsubscribe, nil
}

func (s *EventSink) Subscribe(ctx context.Context, roomID domain.RoomID) (<-chan domain.Event, func()) {
	s.mu.Lock()
	ch := make(chan domain.Event, 100)
	s.subscribers[roomID] = append(s.subscribers[roomID], ch)
	s.mu.Unlock()

	unsubscribe := func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		subs := s.subscribers[roomID]
		for i, sub := range subs {
			if sub == ch {
				s.subscribers[roomID] = append(subs[:i], subs[i+1:]...)
				close(ch)
				break
			}
		}
	}

	return ch, unsubscribe
}
