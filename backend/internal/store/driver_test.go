package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"

	"app-suite/internal/db"
)

// What the store relies on the SQLite driver for: the pragmas on every pooled
// connection, BEGIN IMMEDIATE for its writes, a canceled query reported as
// canceled, and timestamps read back as stored.

func openDriverTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := migratedPath(t)
	s, err := New(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, path
}

// Every connection either pool opens carries every connect pragma, not just the
// first one: the test holds each pool's whole allowance at once, so each is its
// own. The read pool's are query_only as well, and its connections refuse a write.
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
	for _, pool := range []struct {
		name      string
		db        *sql.DB
		size      int
		queryOnly string
	}{
		{"write", s.write, 1, "0"},
		{"read", s.read, readConns, "1"},
	} {
		if n := pool.db.Stats().MaxOpenConnections; n != pool.size {
			t.Fatalf("the %s pool allows %d connections, want %d", pool.name, n, pool.size)
		}
		conns := make([]*sql.Conn, 0, pool.size)
		for range pool.size {
			conn, err := pool.db.Conn(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			conns = append(conns, conn)
		}
		for i, conn := range conns {
			for name, value := range map[string]string{"query_only": pool.queryOnly} {
				want[name] = value
			}
			for name, value := range want {
				var got string
				if err := conn.QueryRowContext(t.Context(), "PRAGMA "+name).Scan(&got); err != nil {
					t.Fatalf("%s connection %d: PRAGMA %s: %v", pool.name, i, name, err)
				}
				if got != value {
					t.Errorf("%s connection %d: %s = %s, want %s", pool.name, i, name, got, value)
				}
			}
		}
		if open := pool.db.Stats().OpenConnections; open != pool.size {
			t.Errorf("checked %d %s connections, but the pool has %d open", pool.size, pool.name, open)
		}
		for _, conn := range conns {
			_ = conn.Close()
		}
	}
	if _, err := s.read.ExecContext(t.Context(), `INSERT INTO settings (key, value) VALUES ('read-pool', 'x')`); err == nil {
		t.Error("the read pool took a write")
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
			return s.read.QueryRowContext(ctx, countToAMillion, id).Scan(&n)
		}},
		{"exec", func(ctx context.Context, id int64) error {
			_, err := s.write.ExecContext(ctx, countToAMillion, id)
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
	if _, err := s.write.ExecContext(t.Context(), insert); err != nil {
		t.Fatal(err)
	}
	_, constraint := s.write.ExecContext(t.Context(), insert)
	if constraint == nil {
		t.Fatal("a duplicate key was accepted")
	}

	for _, err := range []error{nil, errors.New("disk I/O error"), context.Canceled, context.DeadlineExceeded, constraint} {
		if Interrupted(err) {
			t.Errorf("Interrupted(%v) = true, want false", err)
		}
	}
}

// Timestamps in the one stored form: RFC 3339 in UTC to the second.
// The driver parses a DATETIME column into a time.Time, which database/sql formats
// back as RFC 3339 when it scans into a string, so a value in the stored form reads
// back as the same text - these are the paths the frontend and the plugin parse.
// What SQL writes for "now" is that form, and every DATETIME column's CHECK refuses
// any other: SQLite's space form, an offset, fractional seconds.
func TestTimestampsInTheStoredForm(t *testing.T) {
	ctx := t.Context()
	s, _ := openDriverTestStore(t)
	exec := func(query string, args ...any) (sql.Result, error) {
		t.Helper()
		return s.write.ExecContext(ctx, query, args...)
	}
	must := func(res sql.Result, err error) sql.Result {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	const stored = "2026-01-02T03:04:05Z"
	form := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

	q := db.New(s.write)
	if err := q.CreateUser(ctx, db.CreateUserParams{Username: "timestamps", PasswordHash: "hash"}); err != nil {
		t.Fatal(err)
	}
	made, err := q.GetUserByUsername(ctx, "timestamps")
	if err != nil {
		t.Fatal(err)
	}
	if made.CreatedAt == nil || !form.MatchString(*made.CreatedAt) {
		t.Errorf("a new user's created_at = %v, want the stored form", made.CreatedAt)
	}
	id := made.ID
	must(exec(`UPDATE users SET created_at = ? WHERE id = ?`, stored, id))
	got, err := q.GetUser(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.CreatedAt == nil || *got.CreatedAt != stored || got.LastLoginAt != nil {
		t.Errorf("user created_at %v, last_login_at %v; want %q and none", got.CreatedAt, got.LastLoginAt, stored)
	}
	if err := q.TouchLastLogin(ctx, id); err != nil {
		t.Fatal(err)
	}
	if got, _ = q.GetUser(ctx, id); got.LastLoginAt == nil || !form.MatchString(*got.LastLoginAt) {
		t.Errorf("last_login_at = %v after a sign-in, want the stored form", got.LastLoginAt)
	}

	for _, bad := range []string{"2026-01-02 03:04:05", "2026-01-02T03:04:05.000Z", "2026-01-02T04:04:05+01:00", "2026-02-30T03:04:05Z"} {
		if _, err := exec(`UPDATE users SET created_at = ? WHERE id = ?`, bad, id); err == nil {
			t.Errorf("created_at took %q; the CHECK should refuse all but the stored form", bad)
		}
	}

	must(exec(`INSERT INTO winners_log (logged_at, card_id) VALUES (?, 'ABC234')`, stored))
	entries, err := q.ListWinners(ctx, db.ListWinnersParams{Sort: "loggedAt", Dir: "desc", PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].LoggedAt == nil || *entries[0].LoggedAt != stored {
		t.Errorf("winners log = %+v, want one entry logged at %q", entries, stored)
	}

	rally, err := must(exec(`INSERT INTO stamp_rallies (title) VALUES ('Rally')`)).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	card, err := must(exec(`INSERT INTO stamp_rally_cards (rally_id, token, created_at) VALUES (?, 'card-token', ?)`, rally, stored)).LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	completedAt := stored
	if err := q.SetRallyCardCompleted(ctx, db.SetRallyCardCompletedParams{CompletedAt: &completedAt, ID: card}); err != nil {
		t.Fatal(err)
	}
	c, err := q.GetRallyCard(ctx, card)
	if err != nil {
		t.Fatal(err)
	}
	if c.CompletedAt == nil || *c.CompletedAt != stored || c.CreatedAt == nil || *c.CreatedAt != stored {
		t.Errorf("rally card = %+v, want completed_at and created_at %q", c, stored)
	}

	// A new game's start time, as SQL writes "now".
	created, err := q.CreateGame(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if created.CreatedAt == nil || !form.MatchString(*created.CreatedAt) {
		t.Errorf("game created_at = %v, want the stored form", created.CreatedAt)
	}
	g, err := q.GetActiveGame(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if g.CreatedAt == nil || *g.CreatedAt != *created.CreatedAt {
		t.Errorf("active game = %+v, want created_at %v", g, created.CreatedAt)
	}
}

// NormalizeTime takes what clients send and returns the stored form, or "" for
// none, and refuses anything that isn't a time.
func TestNormalizeTime(t *testing.T) {
	for in, want := range map[string]string{
		"":                          "",
		"  ":                        "",
		"2026-06-13T20:00:00Z":      "2026-06-13T20:00:00Z",
		"2026-06-13T20:00:00.000Z":  "2026-06-13T20:00:00Z",
		"2026-06-13T22:00:00+02:00": "2026-06-13T20:00:00Z",
		"2026-06-13T20:00":          "2026-06-13T20:00:00Z",
	} {
		if got, err := NormalizeTime(in); err != nil || got != want {
			t.Errorf("NormalizeTime(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"tomorrow", "2026-06-13 20:00:00", "2026-13-01T00:00:00Z"} {
		if _, err := NormalizeTime(bad); err == nil {
			t.Errorf("NormalizeTime(%q) took it", bad)
		}
	}
}
