package inmem

import (
	"context"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

func TestEventSink_AppendAndRead(t *testing.T) {
	sink := NewEventSink()
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

	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	if len(snapshot) != 5 {
		t.Fatalf("Expected 5 events in snapshot, got %d", len(snapshot))
	}

	select {
	case <-liveCh:
		t.Fatal("Did not expect live events")
	default:
	}
}

func TestEventSink_ReadFromOffsetAndSubscribe(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()

	// Append 3 events
	for i := 0; i < 3; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
		if _, err := sink.Append(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()

	if len(snapshot) != 2 {
		t.Fatalf("Expected 2 events in snapshot, got %d", len(snapshot))
	}

	// Append a live event after subscribe
	go func() {
		_, _ = sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil))
	}()

	select {
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Timeout waiting for live event")
	case <-liveCh:
	}
}

func TestEventSink_ReconnectNoGapNoDup(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()

	for i := 0; i < 3; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			t.Fatal(err)
		}
	}

	snapshot, liveCh, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}

	seen := make([]domain.Event, 0, 5)
	seen = append(seen, snapshot...)

	for i := 0; i < 2; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			unsubscribe()
			t.Fatal(err)
		}
	}

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

	lastOffset := seen[len(seen)-1].Offset
	if lastOffset != 4 {
		t.Fatalf("Expected last offset 4, got %d", lastOffset)
	}

	for i := 0; i < 2; i++ {
		if _, err := sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)); err != nil {
			t.Fatal(err)
		}
	}

	snapshot2, liveCh2, unsubscribe2, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, lastOffset+1)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe2()

	if len(snapshot2) != 2 {
		t.Fatalf("Expected 2 events in snapshot, got %d", len(snapshot2))
	}

	select {
	case <-liveCh2:
		t.Fatal("Did not expect live events")
	default:
	}

	for i := 0; i < len(seen)-1; i++ {
		if seen[i].Offset+1 != seen[i+1].Offset {
			t.Fatalf("Offsets not sequential at %d", i)
		}
	}
}

func TestEventSink_Subscribe(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()

	// Subscribe before events
	ch, unsubscribe := sink.Subscribe(ctx, roomID)
	defer unsubscribe()

	// Append events in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		for i := 0; i < 3; i++ {
			event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
			sink.Append(ctx, event)
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
			t.Fatal("Timeout waiting for events")
		}
	}

	if count != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

func TestEventSink_MultipleRooms(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	room1 := domain.NewRoomID()
	room2 := domain.NewRoomID()

	// Append events to different rooms
	for i := 0; i < 3; i++ {
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, room1, "", nil))
		sink.Append(ctx, domain.NewEvent(domain.EventTokenReceived, room2, "", nil))
	}

	// Read room1 events
	snapshot1, ch1, unsubscribe1, err := sink.ReadFromOffsetAndSubscribe(ctx, room1, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe1()
	count1 := len(snapshot1)
	select {
	case <-ch1:
		t.Fatal("Did not expect live events")
	default:
	}

	// Read room2 events
	snapshot2, ch2, unsubscribe2, err := sink.ReadFromOffsetAndSubscribe(ctx, room2, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe2()
	count2 := len(snapshot2)
	select {
	case <-ch2:
		t.Fatal("Did not expect live events")
	default:
	}

	if count1 != 3 {
		t.Errorf("Expected 3 events for room1, got %d", count1)
	}
	if count2 != 3 {
		t.Errorf("Expected 3 events for room2, got %d", count2)
	}
}

func TestEventSink_OffsetSequence(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()

	// Verify offsets are sequential
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
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()
	const numGoroutines = 50

	done := make(chan bool)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
			sink.Append(ctx, event)
			done <- true
		}()
	}

	// Wait for all appends
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// Verify all events are stored
	snapshot, ch, unsubscribe, err := sink.ReadFromOffsetAndSubscribe(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	count := len(snapshot)
	select {
	case <-ch:
		t.Fatal("Did not expect live events")
	default:
	}

	if count != numGoroutines {
		t.Errorf("Expected %d events, got %d", numGoroutines, count)
	}
}
