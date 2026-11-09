package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app-suite/internal/model"
)

// latestVersion is the newest migration's version.
func latestVersion(t *testing.T) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	p, err := provider(db)
	if err != nil {
		t.Fatal(err)
	}
	sources := p.ListSources()
	return sources[len(sources)-1].Version
}

// rawDB opens path with the write pool's settings, without migrating.
func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", writeDSN(path))
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

// A fresh database migrates to the newest version, where user_version and goose's
// version agree, with the bootstrap account and the first pattern category seeded.
func TestFreshDatabaseMigratesToLatest(t *testing.T) {
	s, err := New(t.Context(), filepath.Join(t.TempDir(), "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	v, err := s.SchemaVersion(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var goose int64
	if err := s.read.QueryRowContext(t.Context(), `SELECT MAX(version_id) FROM goose_db_version WHERE is_applied`).Scan(&goose); err != nil {
		t.Fatal(err)
	}
	if latest := latestVersion(t); int64(v) != latest || goose != latest {
		t.Fatalf("user_version %d and goose's version %d, want both %d", v, goose, latest)
	}
	var admins, categories int
	if err := s.read.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users WHERE username = 'admin' AND is_admin = 1 AND is_active = 1`).Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if err := s.read.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pattern_categories WHERE id = 1 AND name = 'Standard'`).Scan(&categories); err != nil {
		t.Fatal(err)
	}
	if admins != 1 || categories != 1 {
		t.Errorf("seeded %d bootstrap admins and %d Standard categories, want one of each", admins, categories)
	}
}

// A database the hand-written migrations brought to v71 - production's - is taken
// over as it is: the baseline doesn't run on it, so nothing it holds changes and
// no seed comes back that was deleted, and the migrations after it run.
func TestDatabaseAtTheBaselineIsTakenOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v71.db")
	db := rawDB(t, path)
	for _, q := range []string{
		baselineSQL,
		`INSERT INTO users (username, password_hash, is_admin, is_active) VALUES ('owner', 'x', 1, 1)`,
		`INSERT INTO settings (key, value) VALUES ('app_title', 'Senpan')`,
		fmt.Sprintf("PRAGMA user_version = %d", baselineVersion),
	} {
		if _, err := db.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	s, err := New(t.Context(), path)
	if err != nil {
		t.Fatalf("taking over a v%d database: %v", baselineVersion, err)
	}
	t.Cleanup(func() { s.Close() })

	var users, categories int
	var title string
	if err := s.read.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM users`).Scan(&users); err != nil {
		t.Fatal(err)
	}
	if err := s.read.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pattern_categories`).Scan(&categories); err != nil {
		t.Fatal(err)
	}
	if err := s.read.QueryRowContext(t.Context(), `SELECT value FROM settings WHERE key = 'app_title'`).Scan(&title); err != nil {
		t.Fatal(err)
	}
	if users != 1 || categories != 0 || title != "Senpan" {
		t.Errorf("after the takeover: %d users, %d categories, title %q; want the database as it was (1, 0, Senpan)", users, categories, title)
	}
	if v, _ := s.SchemaVersion(t.Context()); int64(v) != latestVersion(t) {
		t.Errorf("user_version %d after the takeover, want %d", v, latestVersion(t))
	}
}

// A database the hand-written migrations left short of v71 is refused, untouched:
// goose takes over only at the baseline.
func TestDatabaseShortOfTheBaselineIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "v50.db")
	db := rawDB(t, path)
	for _, q := range []string{`CREATE TABLE cards (id TEXT PRIMARY KEY)`, `PRAGMA user_version = 50`} {
		if _, err := db.ExecContext(t.Context(), q); err != nil {
			t.Fatal(err)
		}
	}
	_ = db.Close()

	if s, err := New(t.Context(), path); err == nil {
		s.Close()
		t.Fatal("a v50 database was migrated; it should be refused until a 4.0 release brings it to v71")
	} else if !strings.Contains(err.Error(), "predates goose") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
	if adopted, err := hasGoose(t.Context(), rawDB(t, path)); err != nil || adopted {
		t.Errorf("the refused database was changed: goose's table is there (%v, %v)", adopted, err)
	}
}

// A database migrated by a newer binary is refused rather than run, so a
// rolled-back deploy can't run old code against a schema it doesn't know.
func TestNewerDatabaseIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "newer.db")
	s, err := New(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserVersionForTest(t.Context(), int(latestVersion(t))+1); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()

	if s, err := New(t.Context(), path); err == nil {
		s.Close()
		t.Fatal("a database newer than the binary was accepted")
	} else if !strings.Contains(err.Error(), "newer than this binary") {
		t.Fatalf("refused for the wrong reason: %v", err)
	}
}

// v71Database builds a database the way production's was before goose: the
// baseline's schema at v71, then the given rows.
func v71Database(t *testing.T, rows ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v71.db")
	db := rawDB(t, path)
	for _, q := range append([]string{baselineSQL}, append(rows, fmt.Sprintf("PRAGMA user_version = %d", baselineVersion))...) {
		if _, err := db.ExecContext(t.Context(), q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	_ = db.Close()
	return path
}

// Migration 73 moves every timestamp to the one stored form: SQLite's space form,
// the browser's milliseconds, an offset and the naive form older raffles stored
// (read as UTC) all convert, ” becomes NULL, and AUTOINCREMENT keeps its
// high-water mark, so a deleted id doesn't come back.
func TestTimestampMigrationConvertsEveryForm(t *testing.T) {
	path := v71Database(t,
		`INSERT INTO users (id, username, password_hash, created_at, last_login_at) VALUES (1, 'a', 'x', '2026-01-02 03:04:05', NULL), (2, 'b', 'x', '2026-01-02 03:04:05', '2026-03-04 05:06:07')`,
		`INSERT INTO user_tokens (user_id, token_hash, created_at, last_used_at) VALUES (2, 'h', '2026-01-02 03:04:05', NULL)`,
		`INSERT INTO user_passkeys (user_id, credential_id, credential) VALUES (1, 'c', 'x')`,
		`INSERT INTO raffles (id, title, available_from, available_to) VALUES (5, 'gone', '', '')`,
		`DELETE FROM raffles WHERE id = 5`,
		`INSERT INTO raffles (id, title, available_from, available_to) VALUES (3, 'r', '2026-06-13T20:00:00.000Z', '2026-06-14T20:00')`,
		`INSERT INTO raffle_entries (raffle_id, character_name, world) VALUES (3, 'Aria Ashwood', 'Gilgamesh')`,
		`INSERT INTO tea_rooms (name, locked_until) VALUES ('open', ''), ('locked', '2026-06-13T22:00:00+02:00')`,
		`INSERT INTO festival_maps (id, title) VALUES (1, 'map')`,
		`INSERT INTO ema_boards (id, festival_map_id) VALUES (1, 1)`,
		`INSERT INTO ema_windows (board_id, start_at, end_at) VALUES (1, '2026-06-13T20:00:00Z', '2026-06-13T21:00:00Z')`,
	)
	s, err := New(t.Context(), path)
	if err != nil {
		t.Fatalf("migrating a v71 database: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	for query, want := range map[string]string{
		`SELECT created_at FROM users WHERE id = 1`:                        "2026-01-02T03:04:05Z",
		`SELECT last_login_at IS NULL FROM users WHERE id = 1`:             "1",
		`SELECT last_login_at FROM users WHERE id = 2`:                     "2026-03-04T05:06:07Z",
		`SELECT last_used_at IS NULL FROM user_tokens`:                     "1",
		`SELECT created_at GLOB '????-??-??T??:??:??Z' FROM user_passkeys`: "1",
		`SELECT last_used_at IS NULL FROM user_passkeys`:                   "1",
		`SELECT available_from FROM raffles WHERE id = 3`:                  "2026-06-13T20:00:00Z",
		`SELECT available_to FROM raffles WHERE id = 3`:                    "2026-06-14T20:00:00Z",
		`SELECT locked_until IS NULL FROM tea_rooms WHERE name = 'open'`:   "1",
		`SELECT locked_until FROM tea_rooms WHERE name = 'locked'`:         "2026-06-13T20:00:00Z",
		`SELECT start_at FROM ema_windows`:                                 "2026-06-13T20:00:00Z",
		`SELECT COUNT(*) FROM raffle_entries WHERE raffle_id = 3`:          "1",
		`SELECT seq FROM sqlite_sequence WHERE name = 'raffles'`:           "5",
	} {
		var got string
		if err := s.read.QueryRowContext(t.Context(), query).Scan(&got); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
		if got != want {
			t.Errorf("%s = %q, want %q", query, got, want)
		}
	}
	id, err := s.CreateRaffle(t.Context(), &model.Raffle{Title: "next"})
	if err != nil {
		t.Fatal(err)
	}
	if id != 6 {
		t.Errorf("the next raffle got id %d, want 6: AUTOINCREMENT lost its mark and could reuse a deleted id", id)
	}
}

// A time the migration can't read stops it, and every table stays as it was: the
// rebuild is one transaction, so a deploy rolls back to a database the previous
// binary still runs on.
func TestTimestampMigrationRefusesAnUnreadableTime(t *testing.T) {
	path := v71Database(t, `INSERT INTO raffles (title, available_from) VALUES ('r', 'next tuesday')`)
	if s, err := New(t.Context(), path); err == nil {
		s.Close()
		t.Fatal("a raffle open from 'next tuesday' migrated; the time would have been lost")
	}
	db := rawDB(t, path)
	var from, sql string
	if err := db.QueryRowContext(t.Context(), `SELECT available_from FROM raffles`).Scan(&from); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT sql FROM sqlite_master WHERE name = 'raffles'`).Scan(&sql); err != nil {
		t.Fatal(err)
	}
	if from != "next tuesday" || strings.Contains(sql, "CHECK") {
		t.Errorf("after the refused migration: available_from %q, table %q; want both as they were", from, sql)
	}
}

// The committed backend/schema.sql is what the migrations build now.
func TestSchemaIsCurrent(t *testing.T) {
	want, err := Schema(t.Context())
	if err != nil {
		t.Fatalf("build the schema: %v", err)
	}
	got, err := os.ReadFile("../../schema.sql")
	if err != nil {
		t.Fatalf("read the committed schema.sql: %v", err)
	}
	if string(got) != want {
		t.Fatal("backend/schema.sql is out of date - run `go run ./cmd/schema-gen` in backend/ and commit the result")
	}
}
