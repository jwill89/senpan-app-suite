// Package store provides all database access through a single Store type.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// randInt returns a random int in [0, n).
func randInt(n int) int { return rand.IntN(n) } //nolint:gosec // raffle and garapon draws, not secrets: math/rand/v2, seeded per process

// connectPragmas are applied to EVERY pooled connection: the driver runs each
// _pragma in the DSN (see dsn) as it opens a connection, busy_timeout first and
// the rest in name order. SQLite applies most pragmas per-connection rather than
// per-database, so setting them once after open would leave the pool's other
// connections without them. Two of these are correctness-critical, not just
// tuning: foreign_keys (defaults OFF) is what makes the ON DELETE CASCADE rules
// actually fire - a delete that lands on a connection without it would silently
// orphan child rows - and busy_timeout prevents spurious "database is locked"
// errors under WAL write contention. journal_mode persists in the database file,
// but asserting it here too is harmless and keeps it explicit.
var connectPragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"busy_timeout(5000)",
	"cache_size(-8000)",
	"temp_store(MEMORY)",
	"mmap_size(33554432)",
	"foreign_keys(1)",
}

// dsn is the data source name modernc opens path with: the path, then the
// settings the driver reads from its query. It has no "file:" prefix, so the path
// reaches SQLite as it is, without URI decoding of a '%' or '#' in it.
func dsn(path string) string {
	return path + "?" + url.Values{
		"_pragma": connectPragmas,
		// Every read-write transaction begins IMMEDIATE; see beginImmediate.
		"_txlock": {"immediate"},
	}.Encode()
}

// Store wraps the SQLite database connection and provides typed CRUD methods.
//
// Timestamps leave the store as SQLite stores them. The driver parses a column
// declared DATE, DATETIME or TIMESTAMP into a time.Time, which database/sql then
// formats as RFC 3339 when it scans into a string ("2026-01-02T03:04:05Z" for a
// stored "2026-01-02 03:04:05"), so every query reads such a column as
// CAST(col AS TEXT), which the driver leaves as it is. A COALESCE over one is
// already an expression and needs no cast.
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

	// The DSN carries the per-connection pragmas, so they apply to every
	// connection the pool opens - not just the first.
	db, err := sql.Open("sqlite", dsn(path))
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

// SchemaVersion reads the schema's version (PRAGMA user_version), which also shows
// the database answers: GET /healthz reports both.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return v, nil
}

// Interrupted reports whether err is SQLite's interrupt (SQLITE_INTERRUPT): a
// query stopped part-way because its context was done, reported as the driver's
// own error. database/sql answers context.Canceled for a query whose context was
// done before it started, and modernc answers the same for one it interrupts
// while running - the usual shape of a request whose client disconnected
// mid-query - so callers check for context.Canceled first (see the server's
// clientGone). This catches an interrupt that reaches them unconverted.
func Interrupted(err error) bool {
	var e *sqlite.Error
	return errors.As(err, &e) && e.Code()&0xff == sqlite3.SQLITE_INTERRUPT
}

// beginImmediate starts a write transaction with BEGIN IMMEDIATE rather than the
// default deferred transaction. A deferred transaction starts read-only and only
// takes the write lock when it first writes; under WAL with multiple pooled
// connections that lets two read-modify-write transactions both read the same
// snapshot and then collide when the second tries to upgrade its read to a write,
// failing with a stale-snapshot error. Taking the write lock up front instead
// makes concurrent writers serialize on busy_timeout - SQLite allows only one
// writer anyway, so there is no throughput cost - which keeps count/cap checks
// (e.g. a garapon's remaining draws) race-free. Every multi-statement mutation in
// this package uses it.
//
// The driver ignores TxOptions.Isolation, so it is the DSN's _txlock that makes
// the BEGIN immediate, for every read-write transaction on the pool (the
// migrations' included); this names the transactions that rely on it.
func (s *Store) beginImmediate(ctx context.Context) (*sql.Tx, error) {
	return s.db.BeginTx(ctx, nil)
}
