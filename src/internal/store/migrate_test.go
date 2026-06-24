package store

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrateBookClubEventsRetired verifies the merge-into-Announcements
// migrations on a legacy database: an old book_club_events table is dropped, and
// each per-club events-channel webhook (a discord_events_webhook_url_<slug>
// setting) becomes a dedicated announcement type while its setting row is removed.
func TestMigrateBookClubEventsRetired(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A minimal legacy DB at v15: the settings table (present since v3 in any real
	// DB) with an events webhook, plus a stray book_club_events table + row. The
	// remaining migrations never touch the table's columns, so its exact shape
	// doesn't matter — only that it exists to be dropped.
	const webhook = "https://discord.com/api/webhooks/123/abc"
	setup := []string{
		`CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE book_club_events (id INTEGER PRIMARY KEY AUTOINCREMENT, club_slug TEXT, title TEXT)`,
	}
	for _, stmt := range setup {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("setup legacy schema: %v", err)
		}
	}
	if _, err := db.Exec(`INSERT INTO book_club_events (club_slug, title) VALUES ('yaoi', 'Old Meeting')`); err != nil {
		t.Fatalf("seed legacy event: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO settings (key, value) VALUES ('discord_events_webhook_url_yaoi', ?)`, webhook,
	); err != nil {
		t.Fatalf("seed events webhook: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = 15"); err != nil {
		t.Fatal(err)
	}

	// Run migrations up to the current schema version.
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}

	// The retired events table is gone.
	if tableExists(db, "book_club_events") {
		t.Error("book_club_events table should have been dropped")
	}

	// The events webhook setting was migrated to an announcement type and removed.
	var leftover int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM settings WHERE key = 'discord_events_webhook_url_yaoi'`,
	).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Errorf("events webhook setting should have been removed; found %d", leftover)
	}

	var name, savedURL string
	if err := db.QueryRow(
		`SELECT name, webhook_url FROM announcement_types WHERE webhook_url = ?`, webhook,
	).Scan(&name, &savedURL); err != nil {
		t.Fatalf("expected a migrated announcement type: %v", err)
	}
	if name != "Yaoi Book Club Events" {
		t.Errorf("migrated type name = %q; want %q", name, "Yaoi Book Club Events")
	}

	// Idempotent: re-running the webhook migration adds nothing (no setting left).
	if err := migrateBookClubEventWebhooks(db); err != nil {
		t.Fatalf("second migrateBookClubEventWebhooks: %v", err)
	}
	var typeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM announcement_types WHERE webhook_url = ?`, webhook).
		Scan(&typeCount); err != nil {
		t.Fatal(err)
	}
	if typeCount != 1 {
		t.Errorf("announcement types with the webhook = %d; want 1", typeCount)
	}
}
