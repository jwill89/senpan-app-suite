// Package store provides all database access through a single Store type.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

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

// readConns is the read pool's size, all kept open: a reopened connection runs the
// pragmas again.
const readConns = 4

// writeDSN and readDSN are the data source names modernc opens path with: the
// path, then the settings the driver reads from its query. Neither has a "file:"
// prefix, so the path reaches SQLite as it is, without URI decoding of a '%' or
// '#' in it.
//
// The write pool begins every transaction IMMEDIATE (see beginImmediate). The read
// pool is query_only, so a write sent to it by mistake fails rather than racing the
// write pool's one connection.
func writeDSN(path string) string {
	return path + "?" + url.Values{
		"_pragma": connectPragmas,
		"_txlock": {"immediate"},
	}.Encode()
}

func readDSN(path string) string {
	return path + "?" + url.Values{
		"_pragma": append(append([]string{}, connectPragmas...), "query_only(1)"),
	}.Encode()
}

// Store holds two pools over one SQLite file: every write and every transaction goes through write, a pool of
// one connection, so writers queue in the app rather than on SQLite's lock; reads
// go through read. A write commits before its response, so a read after it sees
// it.
//
// Timestamps leave the store as SQLite stores them, in the one form times.go
// describes. The driver parses a DATETIME column into a time.Time, which
// database/sql formats back as RFC 3339 when it scans into a string - the same
// text, since that is the form stored.
type Store struct {
	write *sql.DB
	read  *sql.DB
}

// New opens (or creates) the database and brings its schema up to date (migrate),
// which is what a server wants. Anything that only READS should use OpenNoMigrate
// instead.
//
// ctx bounds the migrations. Canceling one stops it at the next statement, and goose
// records only the migrations that finished, so the next start runs the rest, just
// as it would after a crash.
func New(ctx context.Context, path string) (*Store, error) {
	st, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, st.write); err != nil {
		_ = st.Close() // the schema error is the one to report
		return nil, fmt.Errorf("migrate: %w", err)
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

// open opens the two pools. It runs no query: the driver connects lazily, and
// migrating is New's job alone. The DSNs carry the per-connection pragmas, so they
// apply to every connection either pool opens, not just the first.
func open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil { //nolint:gosec // made only when missing (a fresh dev database); the deploy owns production's directory
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	write, err := sql.Open("sqlite", writeDSN(path))
	if err != nil {
		return nil, fmt.Errorf("open write pool: %w", err)
	}
	write.SetMaxOpenConns(1)

	read, err := sql.Open("sqlite", readDSN(path))
	if err != nil {
		return nil, errors.Join(fmt.Errorf("open read pool: %w", err), write.Close())
	}
	read.SetMaxOpenConns(readConns)
	read.SetMaxIdleConns(readConns)

	return &Store{write: write, read: read}, nil
}

// DB is the write pool: every write and every transaction. The session store and
// the API's sqlc queries (internal/db) use it.
func (s *Store) DB() *sql.DB { return s.write }

// ReadDB is the read pool, for the API's sqlc queries (internal/db).
func (s *Store) ReadDB() *sql.DB { return s.read }

// Close closes both pools.
func (s *Store) Close() error { return errors.Join(s.read.Close(), s.write.Close()) }

// SchemaVersion reads the schema's version (PRAGMA user_version), which also shows
// the database answers: GET /healthz reports both.
func (s *Store) SchemaVersion(ctx context.Context) (int, error) {
	var v int
	if err := s.read.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
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
// takes the write lock when it first writes, so a read-modify-write transaction
// could read a snapshot that another writer (a second process such as themetool)
// then changes, and fail with a stale-snapshot error when it upgrades. Taking the
// write lock up front makes writers serialize on busy_timeout instead - SQLite
// allows only one writer anyway - which keeps count/cap checks (e.g. a garapon's
// remaining draws) race-free. Every multi-statement mutation in this package uses
// it.
//
// The driver ignores TxOptions.Isolation, so it is the write pool's _txlock that
// makes the BEGIN immediate, for every transaction on it (the migrations'
// included); this names the transactions that rely on it. Inside one, read and
// write through the transaction: the write pool's one connection is the
// transaction's until it ends, so a call on s.write would wait for it forever.
func (s *Store) beginImmediate(ctx context.Context) (*sql.Tx, error) {
	return s.write.BeginTx(ctx, nil)
}
