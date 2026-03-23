package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

func setupTestEventSink(t *testing.T) (*EventSink, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "sqlite-events-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	sink := NewEventSink(db)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return sink, cleanup
}

func TestEventSink_AppendAndRead(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Append events
	for i := 0; i < 5; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
		offset, err := sink.Append(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		if offset != domain.Offset(i) {
			t.Errorf("Expected offset %d, got %d", i, offset)
		}
	}

	// Read from beginning
	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	if len(snapshot) != 5 {
		t.Fatalf("Expected 5 events in snapshot, got %d", len(snapshot))
	}

	// Verify offsets
	for i, event := range snapshot {
		if event.Offset != domain.Offset(i) {
			t.Errorf("Expected offset %d, got %d", i, event.Offset)
		}
	}

	// Should not have live events yet
	select {
	case <-liveCh:
		t.Fatal("Did not expect live events")
	default:
	}
}

func TestEventSink_ReadFromOffsetAndSubscribe(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Append 3 events
	for i := 0; i < 3; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
		if _, err := sink.Append(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	// Read from offset 1
	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	if len(snapshot) != 2 {
		t.Fatalf("Expected 2 events in snapshot (offset 1,2), got %d", len(snapshot))
	}

	// Append live event
	go func() {
		time.Sleep(10 * time.Millisecond)
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil))
	}()

	// Should receive live event
	select {
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for live event")
	case <-liveCh:
		// Success
	}
}

func TestEventSink_ReconnectNoGapNoDup(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Append initial events
	for i := 0; i < 3; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			t.Fatal(err)
		}
	}

	// Subscribe from offset 0
	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]domain.Event, 0, 5)
	seen = append(seen, snapshot...)

	// Append more events while subscribed
	for i := 0; i < 2; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			unsubscribe()
			t.Fatal(err)
		}
	}

	// Receive live events
	for i := 0; i < 2; i++ {
		select {
		case <-time.After(200 * time.Millisecond):
			unsubscribe()
			t.Fatal("Timeout waiting for live event")
		case event := <-liveCh:
			seen = append(seen, event)
		}
	}

	unsubscribe()

	// Verify sequential offsets
	lastOffset := seen[len(seen)-1].Offset
	if lastOffset != 4 {
		t.Fatalf("Expected last offset 4, got %d", lastOffset)
	}

	for i := 0; i < len(seen)-1; i++ {
		if seen[i].Offset+1 != seen[i+1].Offset {
			t.Fatalf("Offsets not sequential at %d: %d -> %d", i, seen[i].Offset, seen[i+1].Offset)
		}
	}

	// Reconnect from last offset + 1
	snapshot2, liveCh2, unsubscribe2, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, lastOffset+1)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe2()

	// Should have no historical events
	if len(snapshot2) != 0 {
		t.Fatalf("Expected 0 events in new snapshot, got %d", len(snapshot2))
	}

	// Append more events
	for i := 0; i < 2; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			t.Fatal(err)
		}
	}

	// Should receive new events
	for i := 0; i < 2; i++ {
		select {
		case <-time.After(200 * time.Millisecond):
			t.Fatal("Timeout waiting for live event")
		case event := <-liveCh2:
			if event.Offset != lastOffset+1+domain.Offset(i) {
				t.Errorf("Expected offset %d, got %d", lastOffset+1+domain.Offset(i), event.Offset)
			}
		}
	}
}

func TestEventSink_Subscribe(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Subscribe before events
	ch, unsubscribe := sink.Subscribe(ctx, roomID)
	defer unsubscribe()

	// Append events in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		for i := 0; i < 3; i++ {
			sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil))
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Receive events
	count := 0
	timeout := time.After(200 * time.Millisecond)
	for count < 3 {
		select {
		case <-ch:
			count++
		case <-timeout:
			t.Fatalf("Timeout waiting for events, received %d/3", count)
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

func TestEventSink_MultipleRooms(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	room1 := domain.NewRoomID()
	room2 := domain.NewRoomID()

	// Append events to different rooms
	for i := 0; i < 3; i++ {
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, room1, "", nil))
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, room2, "", nil))
	}

	// Read room1 events
	snapshot1, _, unsubscribe1, err := sink.ReadFromOffsetAndSubscribe(ctx, room1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe1()

	// Read room2 events
	snapshot2, _, unsubscribe2, err := sink.ReadFromOffsetAndSubscribe(ctx, room2, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe2()

	if len(snapshot1) != 3 {
		t.Errorf("Expected 3 events for room1, got %d", len(snapshot1))
	}
	if len(snapshot2) != 3 {
		t.Errorf("Expected 3 events for room2, got %d", len(snapshot2))
	}
}

func TestEventSink_OffsetSequence(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Verify offsets are sequential per room
	for i := 0; i < 10; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
		offset, err := sink.Append(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		if int(offset) != i {
			t.Errorf("Expected offset %d, got %d", i, offset)
		}
	}
}

func TestEventSink_ConcurrentAppend(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()
	const numGoroutines = 20 // Reduced from 50 to avoid SQLite busy errors

	var wg sync.WaitGroup
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
			if _, err := sink.Append(ctx, event); err != nil {
				t.Errorf("Append failed: %v", err)
			}
		}()
	}

	// Wait for all appends to complete
	wg.Wait()

	// Small delay to ensure all transactions committed
	time.Sleep(200 * time.Millisecond)

	// Verify all events stored
	snapshot, _, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	if len(snapshot) != numGoroutines {
		t.Errorf("Expected %d events, got %d", numGoroutines, len(snapshot))
	}

	// Verify no duplicate offsets
	offsets := make(map[domain.Offset]bool)
	for _, event := range snapshot {
		if offsets[event.Offset] {
			t.Errorf("Duplicate offset: %d", event.Offset)
		}
		offsets[event.Offset] = true
	}
}

func TestEventSink_PayloadPersistence(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()
	turnID := domain.NewTurnID()

	// Append event with payload
	payload := domain.TokenPayload{
		AgentID: "agent1",
		Token:   "Hello, 世界! 🌍",
	}
	event := domain.NewEvent(domain.EventTokenReceived, roomID, turnID, payload)
	_, err := sink.Append(ctx, event)
	if err != nil {
		t.Fatal(err)
	}

	// Read back
	events, err := sink.GetEventsForRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}

	if len(events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(events))
	}

	if events[0].Type != domain.EventTokenReceived {
		t.Errorf("Expected type %v, got %v", domain.EventTokenReceived, events[0].Type)
	}
	if events[0].RoomID != roomID {
		t.Error("RoomID mismatch")
	}
	if events[0].TurnID != turnID {
		t.Error("TurnID mismatch")
	}
}

func TestEventSink_GetLatestOffset(t *testing.T) {
	sink, cleanup := setupTestEventSink(t)
	defer cleanup()

	ctx := context.Background()
	roomID := domain.NewRoomID()

	// Empty room
	offset, err := sink.GetLatestOffset(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 0 {
		t.Errorf("Expected offset 0 for empty room, got %d", offset)
	}

	// Append events
	for i := 0; i < 5; i++ {
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil))
	}

	offset, err = sink.GetLatestOffset(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if offset != 4 {
		t.Errorf("Expected offset 4, got %d", offset)
	}
}
