package inmem

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

func TestRoomRepo_ConcurrentAccess(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()

	// Create initial room
	room := domain.NewRoom("test")
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Concurrent updates using Update function
	const numGoroutines = 100
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

	// Verify room still exists and is in valid state
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != room.ID {
		t.Errorf("Expected room ID %v, got %v", room.ID, got.ID)
	}
}

func TestRoomRepo_UpdateFunc(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()

	room := domain.NewRoom("test")
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Test successful update
	err := repo.Update(ctx, room.ID, func(r *domain.Room) error {
		r.State = domain.RoomStateStreaming
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

func TestRoomRepo_UpdateNonExistent(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()

	err := repo.Update(ctx, "nonexistent", func(r *domain.Room) error {
		return nil
	})
	if err != ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound, got %v", err)
	}
}

func TestRoomRepo_SaveAndGet(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()

	room := domain.NewRoom("test-scenery")

	// Save room
	if err := repo.Save(ctx, room); err != nil {
		t.Fatal(err)
	}

	// Get room
	got, err := repo.Get(ctx, room.ID)
	if err != nil {
		t.Fatal(err)
	}

	if got.ID != room.ID {
		t.Errorf("Expected ID %v, got %v", room.ID, got.ID)
	}
	if got.SceneryID != "test-scenery" {
		t.Errorf("Expected scenery 'test-scenery', got %v", got.SceneryID)
	}
}

func TestRoomRepo_Delete(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()

	room := domain.NewRoom("test")
	repo.Save(ctx, room)

	// Delete room
	if err := repo.Delete(ctx, room.ID); err != nil {
		t.Fatal(err)
	}

	// Verify it's gone
	_, err := repo.Get(ctx, room.ID)
	if err != ErrRoomNotFound {
		t.Errorf("Expected ErrRoomNotFound after delete, got %v", err)
	}
}
