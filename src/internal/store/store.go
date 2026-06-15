// Package store provides all database access through a single Store type.
package store

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// randInt returns a random int in [0, n).
func randInt(n int) int { return rand.IntN(n) }

// Store wraps the SQLite database connection and provides typed CRUD methods.
type Store struct {
	db *sql.DB
}

// New opens (or creates) the SQLite database at path, applies migrations,
// and returns a ready-to-use Store.
func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports a single writer; limit open connections to avoid
	// lock contention with the pure-Go driver.
	db.SetMaxOpenConns(4)

	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA cache_size = -8000",
		"PRAGMA temp_store = MEMORY",
		"PRAGMA mmap_size = 33554432",
		"PRAGMA foreign_keys = ON",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }
