package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Open opens or creates a SQLite database at the given path.
// It ensures the parent directory exists and initializes the schema.
func Open(dbPath string) (*sql.DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database with WAL mode for better concurrency
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Apply schema migrations
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	return db, nil
}

// Close closes the database connection.
func Close(db *sql.DB) error {
	if db == nil {
		return nil
	}
	return db.Close()
}
