package sqlite

import (
	"database/sql"
	"fmt"
)

// migrate applies database schema migrations.
func migrate(db *sql.DB) error {
	// Create rooms table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS rooms (
			id TEXT PRIMARY KEY,
			scenery_id TEXT NOT NULL,
			state TEXT NOT NULL,
			current_turn TEXT,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_accessed_at DATETIME NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("create rooms table: %w", err)
	}

	// Create events table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			room_id TEXT NOT NULL,
			turn_id TEXT,
			offset INTEGER NOT NULL,
			timestamp DATETIME NOT NULL,
			payload TEXT,
			FOREIGN KEY (room_id) REFERENCES rooms(id) ON DELETE CASCADE,
			UNIQUE(room_id, offset)
		)
	`)
	if err != nil {
		return fmt.Errorf("create events table: %w", err)
	}

	// Create indexes for better query performance
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_events_room_offset ON events(room_id, offset)`,
		`CREATE INDEX IF NOT EXISTS idx_events_room_time ON events(room_id, timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_rooms_state ON rooms(state)`,
		`CREATE INDEX IF NOT EXISTS idx_rooms_last_accessed ON rooms(last_accessed_at)`,
	}

	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}
