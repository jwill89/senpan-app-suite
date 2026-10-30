// Package store provides all database access through a single Store type.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"

	"github.com/ncruces/go-sqlite3"
	"github.com/ncruces/go-sqlite3/driver"
)

// randInt returns a random int in [0, n).
func randInt(n int) int { return rand.IntN(n) } //nolint:gosec // raffle and garapon draws, not secrets: math/rand/v2, seeded per process

// connectPragmas are applied to EVERY pooled connection via the driver's
// connect hook (see New). SQLite applies most pragmas per-connection rather than
// per-database, so setting them once after open would leave the pool's other
// connections without them. Two of these are correctness-critical, not just
// tuning: foreign_keys (defaults OFF) is what makes the ON DELETE CASCADE rules
// actually fire - a delete that lands on a connection without it would silently
// orphan child rows - and busy_timeout prevents spurious "database is locked"
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
// New opens the database and brings its schema up to date, which is what a server
// wants. Anything that only READS should use OpenNoMigrate instead.
//
// ctx bounds the migrations. Canceling it stops them at the next statement; the
// schema version is written only once every migration has run, so the next start
// runs them again from the version stored, just as it would after a crash.
func New(ctx context.Context, path string) (*Store, error) {
	st, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := ensureSchema(ctx, st.db); err != nil {
		_ = st.db.Close() // the schema error is the one to report
		return nil, fmt.Errorf("ensure schema: %w", err)
	}
	return st, nil
}

// OpenNoMigrate opens an existing database WITHOUT running migrations, for tools
// that only read. Migrating is irreversible, so a read-only command should never
// do it as a side effect of being pointed at a file - a stray `themetool dump`
// against a production copy would otherwise silently upgrade that copy's schema.
// The file must already exist; a missing one is an error rather than a fresh,
// empty database.
func OpenNoMigrate(path string) (*Store, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	return open(path)
}

// open opens the database and configures its connection pool. It runs no query:
// the driver connects lazily, and migrating is New's job alone.
func open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // made only when missing (a fresh dev database); the deploy owns production's directory
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	// driver.Open's init callback runs for every connection the pool opens, so
	// the per-connection pragmas apply to all of them - not just the first.
	db, err := driver.Open(path, func(conn *sqlite3.Conn) error {
		return conn.Exec(connectPragmas)
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite only supports a single writer; limit open connections to avoid
	// lock contention with the pure-Go driver.
	db.SetMaxOpenConns(4)

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

// Interrupted reports whether err is a query SQLite stopped part-way because its
// context was done. database/sql answers context.Canceled for a query whose
// context was already done before it started, but one canceled while running
// comes back from the driver as SQLITE_INTERRUPT, which errors.Is(err,
// context.Canceled) does not see - and that is the usual shape of a request whose
// client disconnected mid-query.
func Interrupted(err error) bool { return errors.Is(err, sqlite3.INTERRUPT) }

// beginImmediate starts a write transaction (issuing BEGIN IMMEDIATE via the
// driver's serializable isolation level) rather than the default deferred
// transaction. A deferred transaction starts read-only and only takes the write
// lock when it first writes; under WAL with multiple pooled connections that lets
// two read-modify-write transactions both read the same snapshot and then collide
// when the second tries to upgrade its read to a write, failing with a stale-
// snapshot error. Taking the write lock up front instead makes concurrent writers
// serialize on busy_timeout - SQLite allows only one writer anyway, so there is no
// throughput cost - which keeps count/cap checks (e.g. a garapon's remaining
// draws) race-free. Every multi-statement mutation in this package uses it.
func (s *Store) beginImmediate(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
}
