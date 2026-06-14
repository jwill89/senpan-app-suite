package store

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestMigrateBookClubEventTimes verifies the v16 migration converts the legacy
// unix-seconds columns into readable UTC RFC-3339 strings, backfills existing
// rows, and drops the old columns — using the real SQLite driver.
func TestMigrateBookClubEventTimes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Build the *legacy* (pre-v16) book_club_events table with unix columns and
	// the old due index, then stamp the DB at version 15 so ensureSchema runs
	// only the v16 migration.
	legacy := []string{
		`CREATE TABLE book_club_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			club_slug TEXT NOT NULL DEFAULT 'yaoi',
			title TEXT NOT NULL,
			start_local TEXT NOT NULL DEFAULT '',
			timezone TEXT NOT NULL DEFAULT '',
			length_hours INTEGER NOT NULL DEFAULT 1,
			location TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			image TEXT NOT NULL DEFAULT '',
			post_at_local TEXT NOT NULL DEFAULT '',
			start_at_unix INTEGER NOT NULL DEFAULT 0,
			post_at_unix INTEGER NOT NULL DEFAULT 0,
			posted INTEGER NOT NULL DEFAULT 0,
			posted_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX idx_book_club_events_due ON book_club_events(posted, post_at_unix)`,
	}
	for _, stmt := range legacy {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup legacy schema: %v", err)
		}
	}
	const startUnix, postUnix int64 = 1_780_000_000, 1_779_000_000
	if _, err := db.Exec(
		`INSERT INTO book_club_events (club_slug, title, start_at_unix, post_at_unix) VALUES (?, ?, ?, ?)`,
		"yaoi", "Legacy", startUnix, postUnix,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 15"); err != nil {
		t.Fatal(err)
	}

	// Run migrations.
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}

	// Legacy columns are gone; new readable columns exist.
	if hasColumn(db, "book_club_events", "start_at_unix") || hasColumn(db, "book_club_events", "post_at_unix") {
		t.Error("legacy unix columns should have been dropped")
	}
	if !hasColumn(db, "book_club_events", "start_at") || !hasColumn(db, "book_club_events", "post_at") {
		t.Fatal("new start_at/post_at columns missing")
	}

	// The unix values were converted to UTC RFC-3339 strings.
	var startAt, postAt string
	if err := db.QueryRow(`SELECT start_at, post_at FROM book_club_events WHERE title = 'Legacy'`).
		Scan(&startAt, &postAt); err != nil {
		t.Fatal(err)
	}
	wantStart := time.Unix(startUnix, 0).UTC().Format("2006-01-02T15:04:05Z")
	wantPost := time.Unix(postUnix, 0).UTC().Format("2006-01-02T15:04:05Z")
	if startAt != wantStart {
		t.Errorf("start_at = %q; want %q", startAt, wantStart)
	}
	if postAt != wantPost {
		t.Errorf("post_at = %q; want %q", postAt, wantPost)
	}

	// Migration is idempotent — a second run is a clean no-op.
	if err := migrateBookClubEventTimes(db); err != nil {
		t.Fatalf("second migrateBookClubEventTimes: %v", err)
	}
}
