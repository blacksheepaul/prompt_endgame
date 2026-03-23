package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// EventSink implements port.EventSink with SQLite persistence and in-memory subscriptions.
type EventSink struct {
	db *sql.DB
	mu sync.RWMutex

	// In-memory subscribers for real-time events
	subscribers map[domain.RoomID][]chan domain.Event
}

// NewEventSink creates a new SQLite-backed event sink.
func NewEventSink(db *sql.DB) *EventSink {
	return &EventSink{
		db:          db,
		subscribers: make(map[domain.RoomID][]chan domain.Event),
	}
}

// Append adds an event to the stream and returns assigned offset.
// It persists to database and notifies in-memory subscribers.
func (s *EventSink) Append(ctx context.Context, event domain.Event) (domain.Offset, error) {
	const maxRetries = 10
	var err error

	// Retry loop for handling SQLITE_BUSY with exponential backoff
	for attempt := 0; attempt < maxRetries; attempt++ {
		err = s.appendWithTx(ctx, &event)
		if err == nil {
			break
		}
		// Check if it's a busy error
		if isBusyError(err) && attempt < maxRetries-1 {
			// Exponential backoff: 5ms, 10ms, 20ms, 40ms, ...
			delay := time.Duration(5*(1<<attempt)) * time.Millisecond
			if delay > 100*time.Millisecond {
				delay = 100 * time.Millisecond
			}
			time.Sleep(delay)
			continue
		}
		return 0, err
	}

	if err != nil {
		return 0, err
	}

	// Notify subscribers (outside lock to avoid blocking)
	s.mu.RLock()
	subs := s.subscribers[event.RoomID]
	s.mu.RUnlock()

	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Channel full, skip to avoid blocking
		}
	}

	return event.Offset, nil
}

// appendWithTx performs a single append attempt with transaction.
func (s *EventSink) appendWithTx(ctx context.Context, event *domain.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	var nextOffset int64
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(MAX(offset), -1) + 1 FROM events WHERE room_id = ?
	`, event.RoomID).Scan(&nextOffset)
	if err != nil {
		return fmt.Errorf("get next offset: %w", err)
	}

	event.Offset = domain.Offset(nextOffset)
	event.Timestamp = time.Now()

	// Serialize payload
	var payloadJSON []byte
	if event.Payload != nil {
		payloadJSON, err = json.Marshal(event.Payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
	}

	// Insert event
	_, err = tx.ExecContext(ctx, `
		INSERT INTO events (id, type, room_id, turn_id, offset, timestamp, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.ID, string(event.Type), event.RoomID, event.TurnID, event.Offset, event.Timestamp, string(payloadJSON))
	if err != nil {
		return fmt.Errorf("insert event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// isBusyError checks if the error is a SQLite busy/locked error.
func isBusyError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return contains(errStr, "database is locked") ||
		contains(errStr, "SQLITE_BUSY") ||
		contains(errStr, "SQLITE_LOCKED")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ReadFromOffsetAndSubscribe returns historical events from offset plus a channel for live updates.
// It atomically subscribes to avoid gaps between snapshot and live events.
func (s *EventSink) ReadFromOffsetAndSubscribe(ctx context.Context, roomID domain.RoomID, offset domain.Offset) ([]domain.Event, <-chan domain.Event, func(), error) {
	// Create subscription channel first (under lock to ensure atomicity)
	s.mu.Lock()
	ch := make(chan domain.Event, 100)
	s.subscribers[roomID] = append(s.subscribers[roomID], ch)
	s.mu.Unlock()

	// Read historical events from database
	snapshot, err := s.readFromDB(ctx, roomID, offset)
	if err != nil {
		// Clean up subscription on error
		s.removeSubscriber(roomID, ch)
		return nil, nil, nil, err
	}

	unsubscribe := func() {
		s.removeSubscriber(roomID, ch)
	}

	return snapshot, ch, unsubscribe, nil
}

// Subscribe returns a channel for real-time events only (no historical data).
func (s *EventSink) Subscribe(ctx context.Context, roomID domain.RoomID) (<-chan domain.Event, func()) {
	s.mu.Lock()
	ch := make(chan domain.Event, 100)
	s.subscribers[roomID] = append(s.subscribers[roomID], ch)
	s.mu.Unlock()

	unsubscribe := func() {
		s.removeSubscriber(roomID, ch)
	}

	return ch, unsubscribe
}

// readFromDB reads historical events from database.
func (s *EventSink) readFromDB(ctx context.Context, roomID domain.RoomID, offset domain.Offset) ([]domain.Event, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, type, room_id, turn_id, offset, timestamp, payload
		FROM events
		WHERE room_id = ? AND offset >= ?
		ORDER BY offset ASC
	`, roomID, offset)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	var events []domain.Event
	for rows.Next() {
		var event domain.Event
		var typeStr string
		var payloadJSON sql.NullString

		err := rows.Scan(&event.ID, &typeStr, &event.RoomID, &event.TurnID, &event.Offset, &event.Timestamp, &payloadJSON)
		if err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}

		event.Type = domain.EventType(typeStr)

		if payloadJSON.Valid && payloadJSON.String != "" {
			// Store raw JSON for now - full deserialization would need type info
			var rawPayload map[string]interface{}
			if err := json.Unmarshal([]byte(payloadJSON.String), &rawPayload); err != nil {
				return nil, fmt.Errorf("unmarshal payload: %w", err)
			}
			event.Payload = rawPayload
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	return events, nil
}

// removeSubscriber removes a subscriber channel.
func (s *EventSink) removeSubscriber(roomID domain.RoomID, ch chan domain.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	subs := s.subscribers[roomID]
	for i, sub := range subs {
		if sub == ch {
			// Remove from slice
			s.subscribers[roomID] = append(subs[:i], subs[i+1:]...)
			close(ch)
			break
		}
	}

	// Clean up empty subscriber lists
	if len(s.subscribers[roomID]) == 0 {
		delete(s.subscribers, roomID)
	}
}

// GetEventsForRoom returns all events for a room (for testing/debugging).
func (s *EventSink) GetEventsForRoom(ctx context.Context, roomID domain.RoomID) ([]domain.Event, error) {
	return s.readFromDB(ctx, roomID, 0)
}

// GetLatestOffset returns the latest event offset for a room.
func (s *EventSink) GetLatestOffset(ctx context.Context, roomID domain.RoomID) (domain.Offset, error) {
	var offset sql.NullInt64
	err := s.db.QueryRowContext(ctx, `
		SELECT MAX(offset) FROM events WHERE room_id = ?
	`, roomID).Scan(&offset)
	if err != nil {
		return 0, fmt.Errorf("get latest offset: %w", err)
	}
	if !offset.Valid {
		return 0, nil
	}
	return domain.Offset(offset.Int64), nil
}
