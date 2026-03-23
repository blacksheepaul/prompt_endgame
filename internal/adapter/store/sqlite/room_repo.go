package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/blacksheepaul/prompt_endgame/internal/config"
	"github.com/blacksheepaul/prompt_endgame/internal/domain"
)

// RoomRepo implements port.RoomRepository with SQLite persistence and memory caching.
type RoomRepo struct {
	db *sql.DB
	mu sync.RWMutex

	// Memory cache for hot rooms
	cache map[domain.RoomID]*cachedRoom

	// Configuration
	offloadEnabled bool
	maxCachedRooms int
	idleTimeout    time.Duration
}

// cachedRoom wraps a room with metadata for cache management.
type cachedRoom struct {
	room           *domain.Room
	lastAccessedAt time.Time
}

// NewRoomRepo creates a new SQLite-backed room repository.
func NewRoomRepo(db *sql.DB, cfg config.SQLiteConfig) *RoomRepo {
	return &RoomRepo{
		db:             db,
		cache:          make(map[domain.RoomID]*cachedRoom),
		offloadEnabled: cfg.OffloadEnabled,
		maxCachedRooms: cfg.MaxCachedRooms,
		idleTimeout:    cfg.IdleTimeout,
	}
}

// Save persists a room to both cache and database.
func (r *RoomRepo) Save(ctx context.Context, room *domain.Room) error {
	if room == nil {
		return fmt.Errorf("room is nil")
	}

	now := time.Now()
	room.UpdatedAt = now

	// Serialize current_turn to JSON
	var turnJSON []byte
	var err error
	if room.CurrentTurn != nil {
		turnJSON, err = json.Marshal(room.CurrentTurn)
		if err != nil {
			return fmt.Errorf("marshal current turn: %w", err)
		}
	}

	// Upsert to database
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO rooms (id, scenery_id, state, current_turn, created_at, updated_at, last_accessed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			scenery_id = excluded.scenery_id,
			state = excluded.state,
			current_turn = excluded.current_turn,
			updated_at = excluded.updated_at,
			last_accessed_at = excluded.last_accessed_at
	`, room.ID, room.SceneryID, string(room.State), string(turnJSON), room.CreatedAt, room.UpdatedAt, now)

	if err != nil {
		return fmt.Errorf("save room to db: %w", err)
	}

	// Update cache
	r.mu.Lock()
	r.cache[room.ID] = &cachedRoom{
		room:           room,
		lastAccessedAt: now,
	}
	r.mu.Unlock()

	return nil
}

// Get retrieves a room by ID. It returns from cache if available,
// otherwise loads from database.
func (r *RoomRepo) Get(ctx context.Context, id domain.RoomID) (domain.Room, error) {
	// Try cache first
	r.mu.RLock()
	cached, ok := r.cache[id]
	r.mu.RUnlock()

	if ok {
		r.mu.Lock()
		cached.lastAccessedAt = time.Now()
		r.mu.Unlock()
		return *cached.room, nil
	}

	// Load from database
	room, err := r.loadFromDB(ctx, id)
	if err != nil {
		return domain.Room{}, err
	}

	// Add to cache
	r.mu.Lock()
	r.cache[id] = &cachedRoom{
		room:           room,
		lastAccessedAt: time.Now(),
	}
	r.mu.Unlock()

	return *room, nil
}

// Update updates a room by applying fn within a lock.
// It loads from DB if not in cache, then persists changes.
func (r *RoomRepo) Update(ctx context.Context, id domain.RoomID, fn func(*domain.Room) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Get or load room
	cached, ok := r.cache[id]
	if !ok {
		room, err := r.loadFromDB(ctx, id)
		if err != nil {
			return err
		}
		cached = &cachedRoom{room: room}
		r.cache[id] = cached
	}

	// Apply update function
	if err := fn(cached.room); err != nil {
		return err
	}

	// Update metadata
	now := time.Now()
	cached.room.UpdatedAt = now
	cached.lastAccessedAt = now

	// Serialize current_turn
	var turnJSON []byte
	if cached.room.CurrentTurn != nil {
		var err error
		turnJSON, err = json.Marshal(cached.room.CurrentTurn)
		if err != nil {
			return fmt.Errorf("marshal current turn: %w", err)
		}
	}

	// Persist to database
	_, err := r.db.ExecContext(ctx, `
		UPDATE rooms SET
			scenery_id = ?,
			state = ?,
			current_turn = ?,
			updated_at = ?,
			last_accessed_at = ?
		WHERE id = ?
	`, cached.room.SceneryID, string(cached.room.State), string(turnJSON), cached.room.UpdatedAt, now, id)

	if err != nil {
		return fmt.Errorf("update room in db: %w", err)
	}

	return nil
}

// Delete removes a room from both cache and database.
func (r *RoomRepo) Delete(ctx context.Context, id domain.RoomID) error {
	// Delete from database first
	result, err := r.db.ExecContext(ctx, `DELETE FROM rooms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete room from db: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get rows affected: %w", err)
	}
	if rows == 0 {
		return domain.ErrRoomNotFound
	}

	// Remove from cache
	r.mu.Lock()
	delete(r.cache, id)
	r.mu.Unlock()

	return nil
}

// List returns all rooms from database.
func (r *RoomRepo) List(ctx context.Context) ([]domain.Room, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, scenery_id, state, current_turn, created_at, updated_at
		FROM rooms
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query rooms: %w", err)
	}
	defer rows.Close()

	var rooms []domain.Room
	for rows.Next() {
		var room domain.Room
		var stateStr string
		var turnJSON sql.NullString

		err := rows.Scan(&room.ID, &room.SceneryID, &stateStr, &turnJSON, &room.CreatedAt, &room.UpdatedAt)
		if err != nil {
			return nil, fmt.Errorf("scan room: %w", err)
		}

		room.State = domain.RoomState(stateStr)

		if turnJSON.Valid && turnJSON.String != "" {
			if err := json.Unmarshal([]byte(turnJSON.String), &room.CurrentTurn); err != nil {
				return nil, fmt.Errorf("unmarshal current turn: %w", err)
			}
		}

		rooms = append(rooms, room)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rooms: %w", err)
	}

	return rooms, nil
}

// loadFromDB loads a room from the database.
func (r *RoomRepo) loadFromDB(ctx context.Context, id domain.RoomID) (*domain.Room, error) {
	var room domain.Room
	var stateStr string
	var turnJSON sql.NullString

	err := r.db.QueryRowContext(ctx, `
		SELECT id, scenery_id, state, current_turn, created_at, updated_at
		FROM rooms
		WHERE id = ?
	`, id).Scan(&room.ID, &room.SceneryID, &stateStr, &turnJSON, &room.CreatedAt, &room.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, domain.ErrRoomNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("load room from db: %w", err)
	}

	room.State = domain.RoomState(stateStr)

	if turnJSON.Valid && turnJSON.String != "" {
		if err := json.Unmarshal([]byte(turnJSON.String), &room.CurrentTurn); err != nil {
			return nil, fmt.Errorf("unmarshal current turn: %w", err)
		}
	}

	return &room, nil
}

// Offload removes a room from memory cache while keeping it in database.
// Only idle rooms can be offloaded.
func (r *RoomRepo) Offload(ctx context.Context, id domain.RoomID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cached, ok := r.cache[id]
	if !ok {
		return nil // Already not in cache
	}

	// Only offload idle rooms
	if cached.room.State != domain.RoomStateIdle {
		return fmt.Errorf("cannot offload non-idle room: state=%s", cached.room.State)
	}

	// Update last_accessed_at in database
	_, err := r.db.ExecContext(ctx, `
		UPDATE rooms SET last_accessed_at = ? WHERE id = ?
	`, time.Now(), id)
	if err != nil {
		return fmt.Errorf("update last_accessed_at: %w", err)
	}

	// Remove from cache
	delete(r.cache, id)
	return nil
}

// StartOffloader starts a background goroutine that periodically offloads
// idle rooms from memory to free up space.
func (r *RoomRepo) StartOffloader(ctx context.Context, checkInterval time.Duration) {
	if !r.offloadEnabled || r.idleTimeout <= 0 {
		return
	}

	go func() {
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.offloadIdleRooms(ctx)
			}
		}
	}()
}

// offloadIdleRooms offloads rooms that have been idle for too long
// or when cache size exceeds limit.
func (r *RoomRepo) offloadIdleRooms(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	var toOffload []domain.RoomID

	for id, cached := range r.cache {
		// Skip non-idle rooms
		if cached.room.State != domain.RoomStateIdle {
			continue
		}

		// Offload if idle for too long
		if now.Sub(cached.lastAccessedAt) > r.idleTimeout {
			toOffload = append(toOffload, id)
		}
	}

	// If cache still too large, offload oldest idle rooms
	if len(r.cache)-len(toOffload) > r.maxCachedRooms && r.maxCachedRooms > 0 {
		// Find oldest idle rooms
		type item struct {
			id           domain.RoomID
			lastAccessed time.Time
		}
		var idleRooms []item
		for id, cached := range r.cache {
			if cached.room.State == domain.RoomStateIdle {
				idleRooms = append(idleRooms, item{id, cached.lastAccessedAt})
			}
		}

		// Simple sort to find oldest (only if we need to)
		if len(idleRooms) > 0 {
			// Just remove enough to get under limit
			needed := len(r.cache) - len(toOffload) - r.maxCachedRooms
			for i := 0; i < len(idleRooms) && needed > 0; i++ {
				id := idleRooms[i].id
				// Check not already marked
				found := false
				for _, existing := range toOffload {
					if existing == id {
						found = true
						break
					}
				}
				if !found {
					toOffload = append(toOffload, id)
					needed--
				}
			}
		}
	}

	// Perform offload (remove from cache only, data is already in DB)
	for _, id := range toOffload {
		delete(r.cache, id)
	}
}

// CacheStats returns current cache statistics.
func (r *RoomRepo) CacheStats() (cachedCount, maxCache int, offloadEnabled bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.cache), r.maxCachedRooms, r.offloadEnabled
}
