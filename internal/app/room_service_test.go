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
)

func setupTestService() *RoomService {
	roomRepo := inmem.NewRoomRepo()
	eventSink := inmem.NewEventSink()
	llmProvider := mock.NewProvider(50 * time.Millisecond)
	sceneryRepo := fs.NewRepo("./testdata", true)

	turnRuntime := NewTurnRuntime(llmProvider, eventSink, roomRepo, sceneryRepo)
	return NewRoomService(roomRepo, eventSink, sceneryRepo, turnRuntime)
}

func TestRoomService_CreateRoom(t *testing.T) {
	service := setupTestService()
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
	service := setupTestService()
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

	// Give runtime time to update room state
	time.Sleep(100 * time.Millisecond)
}

func TestRoomService_ConcurrentAnswers(t *testing.T) {
	service := setupTestService()
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
	service := setupTestService()
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")
	service.SubmitAnswer(ctx, room.ID, "Hello")

	// Wait a bit for streaming to start
	time.Sleep(50 * time.Millisecond)

	err := service.CancelTurn(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Verify room state
	time.Sleep(20 * time.Millisecond)
	got, _ := service.GetRoom(ctx, room.ID)
	if got.State != domain.RoomStateCancelled {
		t.Errorf("Expected state %v, got %v", domain.RoomStateCancelled, got.State)
	}
}

func TestRoomService_GetRoom(t *testing.T) {
	service := setupTestService()
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
	service := setupTestService()
	ctx := context.Background()

	_, err := service.GetRoom(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent room")
	}
}

func TestRoomService_CancelWithoutActiveTurn(t *testing.T) {
	service := setupTestService()
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	err := service.CancelTurn(ctx, room.ID)
	if err != ErrNoActiveTurn {
		t.Errorf("Expected ErrNoActiveTurn, got %v", err)
	}
}

func TestRoomService_MultipleRounds(t *testing.T) {
	service := setupTestService()
	ctx := context.Background()

	room, _ := service.CreateRoom(ctx, "default")

	// First turn
	turn1, _ := service.SubmitAnswer(ctx, room.ID, "First")
	if turn1.Round != 1 {
		t.Errorf("Expected round 1, got %d", turn1.Round)
	}

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

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
	service := setupTestService()
	ctx := context.Background()

	_, err := service.CreateRoom(ctx, "nonexistent-scenery")
	if err == nil {
		t.Error("Expected error for nonexistent scenery")
	}
}

func TestRoomService_SubmitAnswerToNonExistentRoom(t *testing.T) {
	service := setupTestService()
	ctx := context.Background()

	_, err := service.SubmitAnswer(ctx, "nonexistent", "test")
	if err != ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}
