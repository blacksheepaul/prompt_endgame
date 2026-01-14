package inmem

import (
	"context"
	"errors"
	"sync"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

var ErrRoomNotFound = errors.New("room not found")

// RoomRepo implements port.RoomRepository with in-memory storage
type RoomRepo struct {
	mu    sync.RWMutex
	rooms map[domain.RoomID]*domain.Room
}

// NewRoomRepo creates a new in-memory room repository
func NewRoomRepo() *RoomRepo {
	return &RoomRepo{
		rooms: make(map[domain.RoomID]*domain.Room),
	}
}

func (r *RoomRepo) Save(ctx context.Context, room *domain.Room) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rooms[room.ID] = room
	return nil
}

func (r *RoomRepo) Get(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	room, ok := r.rooms[id]
	if !ok {
		return domain.Room{}, ErrRoomNotFound
	}
	return *room, nil
}

// Update updates a room by applying fn within the write lock
// This ensures thread-safe modifications
func (r *RoomRepo) Update(ctx context.Context, id domain.RoomID, fn func(*domain.Room) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	room, ok := r.rooms[id]
	if !ok {
		return ErrRoomNotFound
	}
	return fn(room)
}

func (r *RoomRepo) Delete(ctx context.Context, id domain.RoomID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.rooms, id)
	return nil
}
