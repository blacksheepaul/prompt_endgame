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

	// Read from offset 0
	ch, err := sink.ReadFromOffset(ctx, roomID, 0)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for range ch {
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 events, got %d", count)
	}
}

func TestEventSink_ReadFromOffset(t *testing.T) {
	sink := NewEventSink()
	ctx := context.Background()

	roomID := domain.NewRoomID()

	// Append 10 events
	for i := 0; i < 10; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, roomID, "", nil)
		sink.Append(ctx, event)
	}

	// Read from offset 5
	ch, err := sink.ReadFromOffset(ctx, roomID, 5)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for range ch {
		count++
	}

	if count != 5 {
		t.Errorf("Expected 5 events from offset 5, got %d", count)
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
	ch1, _ := sink.ReadFromOffset(ctx, room1, 0)
	count1 := 0
	for range ch1 {
		count1++
	}

	// Read room2 events
	ch2, _ := sink.ReadFromOffset(ctx, room2, 0)
	count2 := 0
	for range ch2 {
		count2++
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
	ch, _ := sink.ReadFromOffset(ctx, roomID, 0)
	count := 0
	for range ch {
		count++
	}

	if count != numGoroutines {
		t.Errorf("Expected %d events, got %d", numGoroutines, count)
	}
}
