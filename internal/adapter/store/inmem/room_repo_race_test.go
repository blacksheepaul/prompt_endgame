//go:build race
// +build race

package inmem

import (
	"context"
	"testing"

	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

func TestRoomRepo_GetPointerIsDangerous(t *testing.T) {
	repo := NewRoomRepo()
	ctx := context.Background()
	room := domain.NewRoom("test")
	repo.Save(ctx, room)

	go func() {
		for i := 0; i < 1000; i++ {
			r, _ := repo.Get(ctx, room.ID)
			r.State = domain.RoomStateStreaming // This should not affects stored data
		}
	}()

	for i := 0; i < 1000; i++ {
		_ = repo.Update(ctx, room.ID, func(r *domain.Room) error {
			r.State = domain.RoomStateIdle
			return nil
		})
	}
}
