package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

func setupTestDB(t *testing.T) (*RoomRepo, func()) {
	t.Helper()

	// Create temp directory for test database
	tmpDir, err := os.MkdirTemp("", "sqlite-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := Open(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("Failed to open database: %v", err)
	}

	cfg := config.SQLiteConfig{
		Path:           dbPath,
		OffloadEnabled: true,
		MaxCachedRooms: 10,
		IdleTimeout:    time.Second,
	}

	repo := NewRoomRepo(db, cfg)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return repo, cleanup
}

func TestRoomRepo_SaveAndGet(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	room := domain.NewRoom("test-scenery")

	// Save room
	if err := repo.Save(ctx, room); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Get room
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != room.ID {
		t.Errorf("Expected ID %v, got %v", room.ID, got.ID)
	}
	if got.SceneryID != "test-scenery" {
		t.Errorf("Expected scenery 'test-scenery', got %v", got.SceneryID)
	}
}

func TestRoomRepo_Get_NotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	_, err := repo.Get(ctx, "nonexistent")
	if err != domain.ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}

func TestRoomRepo_Update(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	room := domain.NewRoom("test")
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Update room
	err := repo.Update(ctx, room.ID, func(r *domain.Room) error {
		r.State = domain.RoomStateStreaming
		return nil
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify update
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != domain.RoomStateStreaming {
		t.Errorf("Expected state %v, got %v", domain.RoomStateStreaming, got.State)
	}
}

func TestRoomRepo_Update_NotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Update(ctx, "nonexistent", func(r *domain.Room) error {
		return nil
	})
	if err != domain.ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}

func TestRoomRepo_Delete(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	room := domain.NewRoom("test")
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Delete room
	if err := repo.Delete(ctx, room.ID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deletion
	_, err := repo.Get(ctx, room.ID)
	if err != domain.ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound after delete, got %v", err)
	}
}

func TestRoomRepo_Delete_NotFound(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()
	err := repo.Delete(ctx, "nonexistent")
	if err != domain.ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}

func TestRoomRepo_List(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// List empty
	rooms, err := repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 0 {
		t.Errorf("Expected 0 rooms, got %d", len(rooms))
	}

	// Create rooms
	room1 := domain.NewRoom("test-1")
	room2 := domain.NewRoom("test-2")
	if err := repo.Save(ctx, room1); err != nil {
		t.Fatal(err)
	}
	if err := repo.Save(ctx, room2); err != nil {
		t.Fatal(err)
	}

	// List again
	rooms, err = repo.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 2 {
		t.Fatalf("Expected 2 rooms, got %d", len(rooms))
	}
}

func TestRoomRepo_ConcurrentAccess(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create initial room
	room := domain.NewRoom("test")
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Concurrent updates
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(n int) {
			defer wg.Done()
			err := repo.Update(ctx, room.ID, func(r *domain.Room) error {
				r.State = domain.RoomStateStreaming
				time.Sleep(time.Microsecond) // Simulate some work
				r.State = domain.RoomStateIdle
				return nil
			})
			if err != nil {
				t.Errorf("Update failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	// Verify room still exists
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != room.ID {
		t.Errorf("Expected room ID %v, got %v", room.ID, got.ID)
	}
}

func TestRoomRepo_OffloadAndReload(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a room with current turn
	room := domain.NewRoom("test")
	room.State = domain.RoomStateIdle
	room.CurrentTurn = &domain.Turn{
		ID:        domain.NewTurnID(),
		RoomID:    room.ID,
		Round:     1,
		State:     domain.TurnStateDone,
		UserInput: "test input",
		Responses: []domain.Response{
			{AgentID: "agent1", Content: "response1"},
		},
	}

	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Offload the room from memory
	if err := repo.Offload(ctx, room.ID); err != nil {
		t.Fatalf("Offload failed: %v", err)
	}

	// Verify cache is empty
	cached, _, _ := repo.CacheStats()
	if cached != 0 {
		t.Errorf("Expected 0 cached rooms after offload, got %d", cached)
	}

	// Get should reload from database
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatalf("Get after offload failed: %v", err)
	}

	if got.ID != room.ID {
		t.Errorf("Expected ID %v, got %v", room.ID, got.ID)
	}
	if got.CurrentTurn == nil {
		t.Fatal("Expected CurrentTurn to be preserved")
	}
	if got.CurrentTurn.Round != 1 {
		t.Errorf("Expected round 1, got %d", got.CurrentTurn.Round)
	}
	if len(got.CurrentTurn.Responses) != 1 {
		t.Errorf("Expected 1 response, got %d", len(got.CurrentTurn.Responses))
	}
}

func TestRoomRepo_Offload_NonIdle(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a streaming room
	room := domain.NewRoom("test")
	room.State = domain.RoomStateStreaming

	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Try to offload - should fail
	err := repo.Offload(ctx, room.ID)
	if err == nil {
		t.Error("Expected error when offloading non-idle room")
	}
}

func TestRoomRepo_CurrentTurnPersistence(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create room with complex turn data
	room := domain.NewRoom("test")
	room.CurrentTurn = &domain.Turn{
		ID:        domain.NewTurnID(),
		RoomID:    room.ID,
		Round:     5,
		State:     domain.TurnStateDone,
		UserInput: "complex input with unicode: 你好世界 🌍",
		Responses: []domain.Response{
			{AgentID: "agent1", Content: "Response 1"},
			{AgentID: "agent2", Content: "Response 2 with \"quotes\""},
			{AgentID: "agent3", Content: "Response 3\nwith newlines"},
		},
	}

	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Reload and verify
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.CurrentTurn == nil {
		t.Fatal("CurrentTurn should not be nil")
	}
	if got.CurrentTurn.Round != 5 {
		t.Errorf("Expected round 5, got %d", got.CurrentTurn.Round)
	}
	if got.CurrentTurn.UserInput != room.CurrentTurn.UserInput {
		t.Errorf("UserInput mismatch: expected %q, got %q", room.CurrentTurn.UserInput, got.CurrentTurn.UserInput)
	}
	if len(got.CurrentTurn.Responses) != 3 {
		t.Fatalf("Expected 3 responses, got %d", len(got.CurrentTurn.Responses))
	}
	for i, resp := range got.CurrentTurn.Responses {
		if resp.AgentID != room.CurrentTurn.Responses[i].AgentID {
			t.Errorf("Response %d AgentID mismatch", i)
		}
		if resp.Content != room.CurrentTurn.Responses[i].Content {
			t.Errorf("Response %d Content mismatch: expected %q, got %q", i, room.CurrentTurn.Responses[i].Content, resp.Content)
		}
	}
}

func TestRoomRepo_Offload_KeepsEvents(t *testing.T) {
	repo, cleanup := setupTestDB(t)
	defer cleanup()

	ctx := context.Background()

	// Create a room with current turn
	room := domain.NewRoom("test")
	room.State = domain.RoomStateIdle
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Create an event sink and add some events for this room
	sink := NewEventSink(repo.db)
	for i := 0; i < 10; i++ {
		event := domain.NewEvent(domain.EventTokenReceived, room.ID, "", map[string]int{"index": i})
		if _, err := sink.Append(ctx, event); err != nil {
			t.Fatal(err)
		}
	}

	// Verify events exist
	events, err := sink.GetEventsForRoom(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 10 {
		t.Fatalf("Expected 10 events before offload, got %d", len(events))
	}

	// Offload the room (only removes from memory cache)
	if err := repo.Offload(ctx, room.ID); err != nil {
		t.Fatalf("Offload failed: %v", err)
	}

	// Verify events are still in database (offload only clears memory cache)
	events, err = sink.GetEventsForRoom(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 10 {
		t.Errorf("Expected 10 events still in database after offload, got %d", len(events))
	}

	// Verify room can be reloaded from database
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatalf("Get after offload failed: %v", err)
	}
	if got.ID != room.ID {
		t.Errorf("Expected room to be reloadable, got ID %v", got.ID)
	}
}
