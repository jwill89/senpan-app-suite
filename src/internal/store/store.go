// Package store provides all database access through a single Store type.
package store

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// randInt returns a random int in [0, n).
func randInt(n int) int { return rand.IntN(n) }

// connectPragmas are applied to EVERY pooled connection via the driver's
// connect hook (see New). SQLite applies most pragmas per-connection rather than
// per-database, so setting them once after open would leave the pool's other
// connections without them. Two of these are correctness-critical, not just
// tuning: foreign_keys (defaults OFF) is what makes the ON DELETE CASCADE rules
// actually fire — a delete that lands on a connection without it would silently
// orphan child rows — and busy_timeout prevents spurious "database is locked"
// errors under WAL write contention. journal_mode/mmap_size persist in the
// database file, but asserting them here too is harmless and keeps it explicit.
const connectPragmas = `
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA busy_timeout = 5000;
PRAGMA cache_size = -8000;
PRAGMA temp_store = MEMORY;
PRAGMA mmap_size = 33554432;
PRAGMA foreign_keys = ON;
`

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

	// driver.Open's init callback runs for every connection the pool opens, so
	// the per-connection pragmas apply to all of them — not just the first.
	db, err := driver.Open(path, func(conn *sqlite3.Conn) error {
		return conn.Exec(connectPragmas)
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports a single writer; limit open connections to avoid
	// lock contention with the pure-Go driver.
	db.SetMaxOpenConns(4)

	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }
