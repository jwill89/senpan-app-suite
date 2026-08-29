package store

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
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
	// doesn't matter - only that it exists to be dropped.
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

// TestEnsureSchemaRefusesNewerDatabase pins the guard that makes a rolled-back
// deploy safe. ensureSchema used to treat any version >= schemaVersion as "up to
// date", so a binary opening a database migrated by a NEWER build started
// silently - and the deploy script's automatic binary rollback does exactly that:
// it restores the previous binary but cannot un-migrate the database, so old code
// would run against a schema it has never seen, with nobody told. Refusing turns
// silent corruption into a startup error the operator can act on.
func TestEnsureSchemaRefusesNewerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Bring it up to date the normal way first, then stamp it as being from a
	// build newer than this one.
	if err := ensureSchema(db); err != nil {
		t.Fatalf("initial ensureSchema: %v", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion+1)); err != nil {
		t.Fatal(err)
	}

	err = ensureSchema(db)
	if err == nil {
		t.Fatal("ensureSchema accepted a database newer than the binary; a rolled-back deploy would run old code against a migrated schema")
	}
	if !strings.Contains(err.Error(), "newer than this binary") {
		t.Errorf("error = %v; want it to say the schema is newer than the binary", err)
	}

	// An exactly-current database is still the fast path.
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
		t.Fatal(err)
	}
	if err := ensureSchema(db); err != nil {
		t.Errorf("ensureSchema rejected an up-to-date database: %v", err)
	}
}

// TestMigrateTeaRoomNumberIndexToleratesLegacyData covers a migration that could
// stop the server booting. The release before v46 let a room be saved with no
// number and let several rooms share one; v46 then built a plain UNIQUE index over
// that column, which fails on exactly that data - and a migration that fails is a
// server that will not start. The index is now partial (blanks are not indexed) and
// genuine duplicates are blanked first.
func TestMigrateTeaRoomNumberIndexToleratesLegacyData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tearooms.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if _, err := db.Exec(teaRoomsTableSQL); err != nil {
		t.Fatalf("create tea_rooms: %v", err)
	}
	// The shapes v45 permitted: two blanks, and two rooms sharing "101".
	for _, num := range []string{"", "", "101", "101", "102"} {
		if _, err := db.Exec(`INSERT INTO tea_rooms (name, room_number) VALUES (?, ?)`, "Room "+num, num); err != nil {
			t.Fatalf("seed tea_rooms: %v", err)
		}
	}

	if err := migrateTeaRoomSubtitle(db); err != nil {
		t.Fatalf("migrateTeaRoomSubtitle refused legacy data: %v", err)
	}

	// Blanks survive untouched - a room is allowed to have no number.
	var blanks int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tea_rooms WHERE room_number = ''`).Scan(&blanks); err != nil {
		t.Fatal(err)
	}
	if blanks != 3 { // the two original blanks, plus the de-duplicated "101"
		t.Errorf("blank room numbers = %d; want 3 (2 original + 1 blanked duplicate)", blanks)
	}
	// Nothing was deleted: a duplicate number is a data-entry slip, not a reason to
	// destroy a room.
	var total int
	if err := db.QueryRow(`SELECT COUNT(*) FROM tea_rooms`).Scan(&total); err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("tea_rooms rows = %d; want all 5 kept", total)
	}
	// And the index really is in force for non-blank numbers.
	if _, err := db.Exec(`INSERT INTO tea_rooms (name, room_number) VALUES ('Dup', '102')`); err == nil {
		t.Error("a duplicate non-blank room number was accepted; the unique index is not doing its job")
	}
}

// TestMigrateFestivalMapsIsReRunSafeAfterRename pins the guard against a migration
// resurrecting a column a later one renamed. v64 renames
// stamp_rally_stamps.stall_id to occupant_id; v61 was guarded only on the old name,
// so re-running the chain re-added stall_id and its index, and v64 then early-
// returned on occupant_id and never cleaned either up.
func TestMigrateFestivalMapsIsReRunSafeAfterRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rerun.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// A fully migrated database: past v64, so the column is occupant_id.
	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	if exists, _, err := columnInfo(db, "stamp_rally_stamps", "occupant_id"); err != nil || !exists {
		t.Fatalf("precondition: occupant_id should exist after a full migrate (exists=%v err=%v)", exists, err)
	}

	// Re-run the v61 migration the way a repeated chain would.
	if err := migrateFestivalMaps(db); err != nil {
		t.Fatalf("migrateFestivalMaps re-run: %v", err)
	}

	if exists, _, err := columnInfo(db, "stamp_rally_stamps", "stall_id"); err != nil {
		t.Fatal(err)
	} else if exists {
		t.Error("re-running migrateFestivalMaps resurrected the renamed-away stall_id column")
	}
	var idx int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND name='idx_stamp_rally_stamps_stall'`).Scan(&idx); err != nil {
		t.Fatal(err)
	}
	if idx != 0 {
		t.Error("re-running migrateFestivalMaps re-created the dead stall_id index")
	}
}

// TestMigrateRaffleEntryPaymentsBackfillIsReRunnable pins the backfill's
// idempotency. ensureSchema bumps user_version once, after everything has run, so a
// boot that died between this ALTER committing and that write came back with the
// column present - and the backfill, gated on the column being absent, was then
// skipped for good. Historically paid entries stayed at paid_entries=0, which reads
// as unpaid.
func TestMigrateRaffleEntryPaymentsBackfillIsReRunnable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "payments.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO raffles (title) VALUES ('R')`); err != nil {
		t.Fatal(err)
	}
	// A settled row whose backfill was missed: paid, but paid_entries left at 0.
	if _, err := db.Exec(`INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries, paid, paid_entries)
		VALUES (1, 'Aria', 'Gilgamesh', 4, 1, 0)`); err != nil {
		t.Fatal(err)
	}
	// A row settled AFTER the upgrade, partially. The re-run must not touch it.
	if _, err := db.Exec(`INSERT INTO raffle_entries (raffle_id, character_name, world, num_entries, paid, paid_entries)
		VALUES (1, 'Borin', 'Hades', 5, 1, 2)`); err != nil {
		t.Fatal(err)
	}

	if err := migrateRaffleEntryPayments(db); err != nil {
		t.Fatalf("migrateRaffleEntryPayments re-run: %v", err)
	}

	var aria, borin int
	if err := db.QueryRow(`SELECT paid_entries FROM raffle_entries WHERE character_name='Aria'`).Scan(&aria); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT paid_entries FROM raffle_entries WHERE character_name='Borin'`).Scan(&borin); err != nil {
		t.Fatal(err)
	}
	if aria != 4 {
		t.Errorf("missed backfill not repaired: paid_entries = %d; want 4", aria)
	}
	if borin != 2 {
		t.Errorf("re-run clobbered a partial settlement: paid_entries = %d; want 2 left alone", borin)
	}
}

// TestStampLookupIndexesExistOnBothPaths pins the indexes behind the public
// participant lookup. It filters on participant_name (case-insensitively) and
// joins garapon_players by stamp_card_id, and neither had an index - so an
// unauthenticated endpoint scanned every card ever issued and made SQLite build a
// transient index over garapon_players on every call. Checked on a FRESH database
// and on a migrated one, because an index that exists only on fresh installs is a
// bug this file has produced before.
func TestStampLookupIndexesExistOnBothPaths(t *testing.T) {
	want := []string{"idx_stamp_rally_cards_participant", "idx_garapon_players_stamp_card"}

	t.Run("fresh install", func(t *testing.T) {
		db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "fresh.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := ensureSchema(db); err != nil {
			t.Fatal(err)
		}
		assertIndexes(t, db, want)
	})

	t.Run("upgraded database", func(t *testing.T) {
		db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "upgraded.db"))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		// Build the tables the way an existing database has them, then run only the
		// owning migrations - the path an upgrade takes, which never calls
		// createIndexes.
		if err := ensureSchema(db); err != nil {
			t.Fatal(err)
		}
		for _, idx := range want {
			if _, err := db.Exec("DROP INDEX IF EXISTS " + idx); err != nil {
				t.Fatal(err)
			}
		}
		if err := migrateStampRally(db); err != nil {
			t.Fatalf("migrateStampRally: %v", err)
		}
		if err := migrateGarapons(db); err != nil {
			t.Fatalf("migrateGarapons: %v", err)
		}
		assertIndexes(t, db, want)
	})
}

func assertIndexes(t *testing.T, db *sql.DB, names []string) {
	t.Helper()
	for _, name := range names {
		var found int
		if err := db.QueryRow(
			`SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&found); err != nil {
			t.Fatal(err)
		}
		if found == 0 {
			t.Errorf("index %s is missing", name)
		}
	}
}

// The v66 backfill splits composed participant names into name + world. It runs
// once over live festival data, so the cases that matter are the ragged ones: a
// name with no world, a name that itself contains the separator, and rows written
// after the upgrade that must not be touched by a re-run.
func TestMigrateParticipantWorldsSplitsAndIsReRunnable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "worlds.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := ensureSchema(db); err != nil {
		t.Fatalf("ensureSchema: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO stamp_rallies (title) VALUES ('R')`); err != nil {
		t.Fatal(err)
	}
	// Rows as they looked before the column existed: everything in one blob.
	for _, seed := range []struct{ token, composed string }{
		{"t1", "Aria Ashwood @ Gilgamesh"},
		{"t2", "Solo Name"},              // never typed a world
		{"t3", "Odd @ Name @ Gilgamesh"}, // separator inside the name
		{"t4", "  Spacey  @  Balmung  "},
	} {
		if _, err := db.Exec(`INSERT INTO stamp_rally_cards (rally_id, token, participant_name, world)
			VALUES (1, ?, ?, '')`, seed.token, seed.composed); err != nil {
			t.Fatal(err)
		}
	}
	// A row written AFTER the upgrade, already split. A re-run must leave it alone
	// even though its name would otherwise look splittable.
	if _, err := db.Exec(`INSERT INTO stamp_rally_cards (rally_id, token, participant_name, world)
		VALUES (1, 't5', 'Later @ Person', 'Hades')`); err != nil {
		t.Fatal(err)
	}

	for pass := 1; pass <= 2; pass++ { // idempotent: the second pass changes nothing
		if err := migrateParticipantWorlds(db); err != nil {
			t.Fatalf("migrateParticipantWorlds pass %d: %v", pass, err)
		}
		for _, want := range []struct{ token, name, world string }{
			{"t1", "Aria Ashwood", "Gilgamesh"},
			// No separator: keep what the participant typed rather than inventing a
			// world for them.
			{"t2", "Solo Name", ""},
			// Split on the LAST separator - a world never contains one.
			{"t3", "Odd @ Name", "Gilgamesh"},
			{"t4", "Spacey", "Balmung"},
			{"t5", "Later @ Person", "Hades"},
		} {
			var name, world string
			if err := db.QueryRow(`SELECT participant_name, world FROM stamp_rally_cards WHERE token = ?`,
				want.token).Scan(&name, &world); err != nil {
				t.Fatal(err)
			}
			if name != want.name || world != want.world {
				t.Errorf("pass %d, %s: got (%q, %q); want (%q, %q)",
					pass, want.token, name, world, want.name, want.world)
			}
		}
	}
}
