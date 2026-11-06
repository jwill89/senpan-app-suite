package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// What the store relies on the SQLite driver for: the pragmas on every pooled
// connection, BEGIN IMMEDIATE for its writes, a canceled query reported as
// canceled, and timestamps read back as stored.

func openDriverTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := New(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

// Every connection the pool opens carries every connect pragma, not just the first
// one: the test holds the pool's whole allowance at once, so each is its own.
func TestConnectPragmasOnEveryConnection(t *testing.T) {
	s, _ := openDriverTestStore(t)
	want := map[string]string{
		"journal_mode": "wal",
		"synchronous":  "1", // NORMAL
		"busy_timeout": "5000",
		"cache_size":   "-8000",
		"temp_store":   "2", // MEMORY
		"mmap_size":    "33554432",
		"foreign_keys": "1",
	}

	n := s.db.Stats().MaxOpenConnections
	if n < 2 {
		t.Fatalf("the pool allows %d connections; the test needs several to mean anything", n)
	}
	for i := range n {
		conn, err := s.db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		for name, value := range want {
			var got string
			if err := conn.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&got); err != nil {
				t.Fatalf("connection %d: PRAGMA %s: %v", i, name, err)
			}
			if got != value {
				t.Errorf("connection %d: %s = %s, want %s", i, name, got, value)
			}
		}
	}
	if open := s.db.Stats().OpenConnections; open != n {
		t.Errorf("checked %d connections, but the pool has %d open", n, open)
	}
}

// beginImmediate takes the write lock when it begins, not at its first write: a
// second writer that will not wait is refused while the first transaction is
// open, and gets in once it ends. A deferred BEGIN would hold no lock yet.
func TestBeginImmediateTakesTheWriteLock(t *testing.T) {
	s, path := openDriverTestStore(t)
	tx, err := s.beginImmediate(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	other, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(0)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	second, err := other.BeginTx(t.Context(), nil)
	if err == nil {
		_ = second.Rollback()
		t.Fatal("a second writer began while the first transaction was open; beginImmediate did not take the write lock")
	}
	var e *sqlite.Error
	if !errors.As(err, &e) || e.Code()&0xff != sqlite3.SQLITE_BUSY {
		t.Fatalf("second writer's BEGIN = %v, want SQLITE_BUSY", err)
	}

	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	second, err = other.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatalf("second writer's BEGIN after the first ended: %v", err)
	}
	_ = second.Rollback()
}

// cancel_query(id) cancels the context stored under id. The driver's functions are
// process-wide, so it is registered once and finds each query's cancel by the id
// the query passes it.
var (
	registerCancelQuery sync.Once
	cancelQueries       sync.Map // int64 -> context.CancelFunc
	cancelQueryIDs      atomic.Int64
)

// countToAMillion runs long enough that a cancel from its first row lands while
// it is still going.
const countToAMillion = `WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x + 1 FROM c WHERE x < 1000000)
	SELECT count(cancel_query(?)) FROM c`

// A query whose context is canceled while it runs - a request whose client went
// away mid-query - fails as canceled, which is what lets the server log it at
// Debug rather than as an error (clientGone). The query cancels its own context
// from a SQL function on its first row, so the cancel lands mid-flight every time
// rather than depending on a race with a timer.
func TestQueryCanceledMidFlightIsCanceled(t *testing.T) {
	registerCancelQuery.Do(func() {
		sqlite.MustRegisterScalarFunction("cancel_query", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
			if cancel, ok := cancelQueries.Load(args[0]); ok {
				cancel.(context.CancelFunc)()
			}
			return nil, nil
		})
	})
	s, _ := openDriverTestStore(t)

	cases := []struct {
		name string
		run  func(ctx context.Context, id int64) error
	}{
		{"query", func(ctx context.Context, id int64) error {
			var n int
			return s.db.QueryRowContext(ctx, countToAMillion, id).Scan(&n)
		}},
		{"exec", func(ctx context.Context, id int64) error {
			_, err := s.db.ExecContext(ctx, countToAMillion, id)
			return err
		}},
		{"inside a write transaction", func(ctx context.Context, id int64) error {
			tx, err := s.beginImmediate(ctx)
			if err != nil {
				return fmt.Errorf("begin: %w", err)
			}
			defer func() { _ = tx.Rollback() }() // the context's end has rolled it back already
			_, err = tx.ExecContext(ctx, countToAMillion, id)
			return err
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			id := cancelQueryIDs.Add(1)
			cancelQueries.Store(id, cancel)
			defer cancelQueries.Delete(id)

			err := c.run(ctx, id)
			if err == nil {
				t.Fatal("the query ran to the end; it was meant to be interrupted")
			}
			if !errors.Is(err, context.Canceled) && !Interrupted(err) {
				t.Fatalf("err = %v, want context.Canceled or SQLite's interrupt", err)
			}
		})
	}
}

// Interrupted answers only for SQLite's interrupt: a real failure must not be
// taken for a departed client, or it would be logged at Debug and missed.
func TestInterruptedIgnoresOtherErrors(t *testing.T) {
	s, _ := openDriverTestStore(t)
	insert := `INSERT INTO settings (key, value) VALUES ('interrupt_test', 'x')`
	if _, err := s.db.ExecContext(t.Context(), insert); err != nil {
		t.Fatal(err)
	}
	_, constraint := s.db.ExecContext(t.Context(), insert)
	if constraint == nil {
		t.Fatal("a duplicate key was accepted")
	}

	for _, err := range []error{nil, errors.New("disk I/O error"), context.Canceled, context.DeadlineExceeded, constraint} {
		if Interrupted(err) {
			t.Errorf("Interrupted(%v) = true, want false", err)
		}
	}
}

// The driver parses a column declared DATETIME into a time.Time, which database/sql
// then formats as RFC 3339 when it scans into a string, so reading one directly
// would hand the API "2026-01-02T03:04:05Z" where the table holds SQLite's
// "2026-01-02 03:04:05", and the frontend parses the two differently. The store
// reads every one as CAST(... AS TEXT), which comes back as stored; these are the
// paths the frontend and the plugin parse.
func TestDatetimeColumnsReadAsStored(t *testing.T) {
	ctx := t.Context()
	s, _ := openDriverTestStore(t)
	exec := func(query string, args ...any) sql.Result {
		t.Helper()
		res, err := s.db.ExecContext(ctx, query, args...)
		if err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		return res
	}
	const stored = "2026-01-02 03:04:05"
	// What Go writes (a completed card's time), which the driver would also have
	// reformatted, to "2026-01-02T03:04:05Z".
	const storedRFC3339 = "2026-01-02T03:04:05.000Z"

	u, err := s.CreateUser(ctx, "timestamps", "hash")
	if err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE users SET created_at = ? WHERE id = ?`, stored, u.ID)
	got, err := s.GetUserByID(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt != stored {
		t.Errorf("user created_at = %q, want %q", got.CreatedAt, stored)
	}

	if err := s.UpsertUserToken(ctx, u.ID, "token-hash", "pat_test"); err != nil {
		t.Fatal(err)
	}
	exec(`UPDATE user_tokens SET created_at = ? WHERE user_id = ?`, stored, u.ID)
	info, err := s.GetUserTokenInfo(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.CreatedAt != stored {
		t.Errorf("token created_at = %q, want %q", info.CreatedAt, stored)
	}

	exec(`INSERT INTO winners_log (logged_at, card_id) VALUES (?, 'ABC234')`, stored)
	entries, _, err := s.ListWinnersLog(ctx, 10, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LoggedAt != stored {
		t.Errorf("winners log = %+v, want one entry logged at %q", entries, stored)
	}

	rally, err := exec(`INSERT INTO stamp_rallies (title) VALUES ('Rally')`).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	card, err := exec(`INSERT INTO stamp_rally_cards (rally_id, token, completed, completed_at, created_at)
		VALUES (?, 'card-token', 1, ?, ?)`, rally, storedRFC3339, stored).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.GetRallyCardByID(ctx, card)
	if err != nil {
		t.Fatal(err)
	}
	if c == nil || c.CompletedAt != storedRFC3339 || c.CreatedAt != stored {
		t.Errorf("rally card = %+v, want completed_at %q and created_at %q", c, storedRFC3339, stored)
	}

	// A new game's start time, as CURRENT_TIMESTAMP wrote it.
	_, createdAt, err := s.StartGame(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$`).MatchString(createdAt) {
		t.Errorf("game created_at = %q, want SQLite's YYYY-MM-DD HH:MM:SS", createdAt)
	}
	g, err := s.GetActiveGame(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if g == nil || g.CreatedAt != createdAt {
		t.Errorf("active game = %+v, want created_at %q", g, createdAt)
	}
}
