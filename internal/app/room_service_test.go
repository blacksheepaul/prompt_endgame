package app

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/adapter/provider/mock"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/scenery/fs"
	"github.com/blacksheepaul/prompt_endgame/internal/adapter/store/inmem"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
	"github.com/blacksheepaul/prompt_endgame/internal/port"
)

type testEnv struct {
	service   *RoomService
	eventSink port.EventSink
}

func setupTestService(tokenDelay time.Duration) testEnv {
	roomRepo := inmem.NewRoomRepo()
	eventSink := inmem.NewEventSink()
	llmProvider := mock.NewProvider(tokenDelay)
	sceneryRepo := fs.NewRepo("./testdata", true)

	turnRuntime := NewTurnRuntime(llmProvider, eventSink, roomRepo, sceneryRepo)
	return testEnv{
		service:   NewRoomService(roomRepo, eventSink, sceneryRepo, turnRuntime),
		eventSink: eventSink,
	}
}

func subscribeRoomEvents(t *testing.T, sink port.EventSink, roomID domain.RoomID) (<-chan domain.Event, func()) {
	t.Helper()
	ch, unsubscribe := sink.Subscribe(context.Background(), roomID)
	return ch, unsubscribe
}

func waitForEvent(t *testing.T, ch <-chan domain.Event, eventType domain.EventType, timeout time.Duration) domain.Event {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			t.Fatalf("timeout waiting for event %s", eventType)
			return domain.Event{}
		case event, ok := <-ch:
			if !ok {
				continue
			}
			if event.Type == eventType {
				return event
			}
		}
	}
}

func TestRoomService_CreateRoom(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, err := service.CreateRoom(ctx, "default")
	if err != nil {
		t.Fatal(err)
	}

	if room.ID == "" {
		t.Error("Expected room ID to be set")
	}
	if room.State != domain.RoomStateIdle {
		t.Errorf("Expected state %v, got %v", domain.RoomStateIdle, room.State)
	}
	if room.SceneryID != "default" {
		t.Errorf("Expected scenery 'default', got %v", room.SceneryID)
	}
}

func TestRoomService_SubmitAnswer(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	turn, err := service.SubmitAnswer(ctx, room.ID, "Hello")
	if err != nil {
		t.Fatal(err)
	}

	if turn.Round != 1 {
		t.Errorf("Expected round 1, got %d", turn.Round)
	}
	if turn.UserInput != "Hello" {
		t.Errorf("Expected input 'Hello', got %s", turn.UserInput)
	}

}

func TestRoomService_ConcurrentAnswers(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	// Try to submit multiple answers concurrently
	const numConcurrent = 10
	var wg sync.WaitGroup
	successCount := 0
	busyCount := 0
	var mu sync.Mutex

	wg.Add(numConcurrent)
	for i := 0; i < numConcurrent; i++ {
		go func(n int) {
			defer wg.Done()
			_, err := service.SubmitAnswer(ctx, room.ID, "test")
			mu.Lock()
			switch err {
			case nil:
				successCount++
			case ErrRoomBusy:
				busyCount++
			default:
				t.Errorf("Unexpected error: %v", err)
			}
			mu.Unlock()
		}(i)
	}

	wg.Wait()

	// Should have at least one success and some busy errors
	if successCount == 0 {
		t.Error("Expected at least one successful submission")
	}
	t.Logf("Success: %d, Busy: %d", successCount, busyCount)
}

func TestRoomService_CancelTurn(t *testing.T) {
	env := setupTestService(50 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")
	eventCh, unsubscribe := subscribeRoomEvents(t, env.eventSink, room.ID)
	defer unsubscribe()

	service.SubmitAnswer(ctx, room.ID, "Hello")

	// Wait for actual streaming to start (first token proves ExecuteTurn is running)
	waitForEvent(t, eventCh, domain.EventTokenReceived, 500*time.Millisecond)

	err := service.CancelTurn(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}

	waitForEvent(t, eventCh, domain.EventTurnCancelled, 500*time.Millisecond)

	// Wait for ExecuteTurn defer to complete and reset room to Idle
	time.Sleep(200 * time.Millisecond)
	got, _ := service.GetRoom(ctx, room.ID)
	if got.State != domain.RoomStateIdle {
		t.Errorf("Expected room state %v, got %v", domain.RoomStateIdle, got.State)
	}
	if got.CurrentTurn == nil {
		t.Fatal("Expected current turn to be preserved")
	}
	if got.CurrentTurn.State != domain.TurnStateCancelled {
		t.Errorf("Expected turn state %v, got %v", domain.TurnStateCancelled, got.CurrentTurn.State)
	}
}

func TestRoomService_CancelStopsStreaming(t *testing.T) {
	env := setupTestService(50 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	eventCh, unsubscribe := subscribeRoomEvents(t, env.eventSink, room.ID)
	defer unsubscribe()

	_, err := service.SubmitAnswer(ctx, room.ID, "Hello")
	if err != nil {
		t.Fatal(err)
	}

	waitForEvent(t, eventCh, domain.EventTurnStarted, 200*time.Millisecond)

	if err := service.CancelTurn(ctx, room.ID); err != nil {
		t.Fatal(err)
	}

	waitForEvent(t, eventCh, domain.EventTurnCancelled, 200*time.Millisecond)

	select {
	case event := <-eventCh:
		if event.Type == domain.EventTurnCompleted {
			t.Fatal("Unexpected TurnCompleted after cancel")
		}
	case <-time.After(150 * time.Millisecond):
		// no completion expected
	}
}

func TestRoomService_GetRoom(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	created, _ := service.CreateRoom(ctx, "default")

	retrieved, err := service.GetRoom(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("Expected ID %v, got %v", created.ID, retrieved.ID)
	}
}

func TestRoomService_GetNonExistentRoom(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	_, err := service.GetRoom(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent room")
	}
}

func TestRoomService_CancelWithoutActiveTurn(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	err := service.CancelTurn(ctx, room.ID)
	if err != ErrNoActiveTurn {
		t.Errorf("Expected ErrNoActiveTurn, got %v", err)
	}
}

func TestRoomService_MultipleRounds(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")
	eventCh, unsubscribe := subscribeRoomEvents(t, env.eventSink, room.ID)
	defer unsubscribe()

	// First turn
	turn1, _ := service.SubmitAnswer(ctx, room.ID, "First")
	if turn1.Round != 1 {
		t.Errorf("Expected round 1, got %d", turn1.Round)
	}

	// Wait for completion
	waitForEvent(t, eventCh, domain.EventTurnCompleted, 500*time.Millisecond)

	// Wait for ExecuteTurn defer to reset room to Idle
	time.Sleep(20 * time.Millisecond)

	// Second turn
	turn2, err := service.SubmitAnswer(ctx, room.ID, "Second")
	if err != nil {
		t.Fatal(err)
	}
	if turn2.Round != 2 {
		t.Errorf("Expected round 2, got %d", turn2.Round)
	}
}

func TestRoomService_CreateRoomWithInvalidScenery(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	_, err := service.CreateRoom(ctx, "nonexistent-scenery")
	if err != ErrInvalidScenery {
		t.Errorf("Expected ErrInvalidScenery, got %v", err)
	}
}

func TestRoomService_SubmitAnswerToNonExistentRoom(t *testing.T) {
	env := setupTestService(1 * time.Millisecond)
	service := env.service
	ctx := context.Background()

	_, err := service.SubmitAnswer(ctx, "nonexistent", "test")
	if err != domain.ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}
