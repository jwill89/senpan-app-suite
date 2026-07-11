package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"app-suite/internal/auth"
)

// schemaVersion is the target database schema version. Each migration
// increments this by one. On startup, ensureSchema compares the stored
// PRAGMA user_version against this constant and runs only the migrations
// needed to bring the database up to date. Bump this when adding a new
// migration block.
const schemaVersion = 45

// ensureSchema reads the current PRAGMA user_version from the database and
// applies any outstanding migrations to bring it up to schemaVersion.
// On the hot path (version == schemaVersion), zero migration queries execute.
// Each migration is guarded by `if version < N` so they run incrementally.
func ensureSchema(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= schemaVersion {
		return nil
	}

	if version < 1 {
		var tableCount int
		if err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='cards'").Scan(&tableCount); err != nil {
			return err
		}
		if tableCount == 0 {
			if err := createTables(db); err != nil {
				return err
			}
		} else {
			if err := migrateSortOrder(db); err != nil {
				return err
			}
			if err := createIndexes(db); err != nil {
				return err
			}
		}
	}

	if version < 2 {
		if err := migrateWinnersCache(db); err != nil {
			return err
		}
	}

	if version < 3 {
		if err := migrateSettings(db); err != nil {
			return err
		}
	}

	if version < 4 {
		if err := migrateStyles(db); err != nil {
			return err
		}
	}

	if version < 5 {
		if err := migratePatternCategories(db); err != nil {
			return err
		}
	}

	if version < 6 {
		if err := migrateRaffles(db); err != nil {
			return err
		}
	}

	if version < 7 {
		if err := migrateWinnersLog(db); err != nil {
			return err
		}
	}

	if version < 8 {
		if err := migrateWinnersLogIndex(db); err != nil {
			return err
		}
	}

	if version < 9 {
		if err := migrateCardPlayerNameIndex(db); err != nil {
			return err
		}
	}

	if version < 10 {
		if err := migrateRaffleImagePaths(db); err != nil {
			return err
		}
	}

	if version < 11 {
		if err := migrateBookClubs(db); err != nil {
			return err
		}
	}

	if version < 12 {
		if err := migrateBookClubTropes(db); err != nil {
			return err
		}
	}

	// Versions 13, 14, and 16 created and evolved the book_club_events table.
	// That feature was merged into the Announcements tool and the table is dropped
	// in version 28, so those create/alter migrations were removed (a fresh DB no
	// longer creates the table; an existing one has it dropped below).

	if version < 15 {
		if err := migrateGamePresets(db); err != nil {
			return err
		}
	}

	if version < 17 {
		if err := migrateAnnouncements(db); err != nil {
			return err
		}
	}

	if version < 18 {
		if err := migrateAnnouncementTimezone(db); err != nil {
			return err
		}
	}

	if version < 19 {
		if err := migrateAnnouncementLocalTimes(db); err != nil {
			return err
		}
	}

	if version < 20 {
		if err := migrateAnnouncementColor(db); err != nil {
			return err
		}
	}

	if version < 21 {
		if err := migrateAnnouncementButtons(db); err != nil {
			return err
		}
	}

	if version < 22 {
		if err := migrateUsers(db); err != nil {
			return err
		}
	}

	if version < 23 {
		if err := migrateUserLastLogin(db); err != nil {
			return err
		}
	}

	if version < 24 {
		if err := migrateAnnouncementRoles(db); err != nil {
			return err
		}
	}

	if version < 25 {
		if err := migrateAnnouncementMention(db); err != nil {
			return err
		}
	}

	if version < 26 {
		if err := migrateAnnouncementLocation(db); err != nil {
			return err
		}
	}

	if version < 27 {
		if err := migrateBookClubEventWebhooks(db); err != nil {
			return err
		}
	}

	if version < 28 {
		if err := migrateDropBookClubEvents(db); err != nil {
			return err
		}
	}

	if version < 29 {
		if err := migrateAnnouncementTimeFormats(db); err != nil {
			return err
		}
	}

	if version < 30 {
		if err := migrateCardCreatedAt(db); err != nil {
			return err
		}
	}

	if version < 31 {
		if err := migrateAnnouncementThumbnail(db); err != nil {
			return err
		}
	}

	if version < 32 {
		if err := migrateAnnouncementDynamicDates(db); err != nil {
			return err
		}
	}

	if version < 33 {
		if err := migrateAnnouncementSortOrder(db); err != nil {
			return err
		}
	}

	if version < 34 {
		if err := migrateStyleFlourishes(db); err != nil {
			return err
		}
	}

	if version < 35 {
		if err := migrateGarapons(db); err != nil {
			return err
		}
	}

	if version < 36 {
		if err := migrateGaraponDrawKeepLogs(db); err != nil {
			return err
		}
	}

	if version < 37 {
		if err := migrateStyleTokens(db); err != nil {
			return err
		}
	}

	if version < 38 {
		if err := migrateAffiliates(db); err != nil {
			return err
		}
	}

	if version < 39 {
		if err := migrateStampRally(db); err != nil {
			return err
		}
	}

	if version < 40 {
		if err := migrateStampRallyGaraponLink(db); err != nil {
			return err
		}
	}

	if version < 41 {
		if err := migrateStampRallyKeepLogs(db); err != nil {
			return err
		}
	}

	if version < 42 {
		if err := migrateUserTokens(db); err != nil {
			return err
		}
	}

	if version < 43 {
		if err := migrateCalledNumbersUnique(db); err != nil {
			return err
		}
	}

	if version < 44 {
		if _, err := db.Exec(userPasskeysTableSQL); err != nil {
			return fmt.Errorf("create user_passkeys: %w", err)
		}
		if _, err := db.Exec(userPasskeysIndexSQL); err != nil {
			return fmt.Errorf("create user_passkeys index: %w", err)
		}
	}

	if version < 45 {
		if err := migrateTeaRooms(db); err != nil {
			return err
		}
	}

	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

// userPasskeysTableSQL defines the WebAuthn passkey credentials table. The full
// go-webauthn Credential is stored as JSON in `credential` (it's designed to be
// serialized); `credential_id` is the base64url of Credential.ID for lookup +
// uniqueness. Rows cascade-delete with the owning user.
const userPasskeysTableSQL = `CREATE TABLE IF NOT EXISTS user_passkeys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL,
	credential_id TEXT NOT NULL UNIQUE,
	credential TEXT NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	last_used_at TEXT NOT NULL DEFAULT '',
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)`

const userPasskeysIndexSQL = `CREATE INDEX IF NOT EXISTS idx_user_passkeys_user ON user_passkeys(user_id)`

// migrateCalledNumbersUnique adds a UNIQUE(game_id, number) index to
// called_numbers: a database-level backstop so a number can't be recorded twice
// in one game even if the draw path's in-process serialization is ever bypassed
// (a second writer / process). Any pre-existing duplicates are collapsed to their
// first occurrence so the unique index can be built.
func migrateCalledNumbersUnique(db *sql.DB) error {
	if !tableExists(db, "called_numbers") {
		return nil
	}
	if _, err := db.Exec(`DELETE FROM called_numbers WHERE rowid NOT IN (
		SELECT MIN(rowid) FROM called_numbers GROUP BY game_id, number)`); err != nil {
		return fmt.Errorf("dedupe called_numbers: %w", err)
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_called_numbers_game_number
		ON called_numbers(game_id, number)`); err != nil {
		return fmt.Errorf("create called_numbers unique index: %w", err)
	}
	return nil
}

// createTables builds all tables from scratch for a fresh database.
// Called only when no tables exist (version 0, no prior data).
func createTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cards (
			id TEXT PRIMARY KEY,
			board_data TEXT NOT NULL,
			player_name TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS pattern_categories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0
		)`,
		// Seed the default category for fresh installs.
		`INSERT OR IGNORE INTO pattern_categories (id, name, sort_order) VALUES (1, 'Standard', 0)`,
		`CREATE TABLE IF NOT EXISTS patterns (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			pattern_data TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			category_id INTEGER NOT NULL DEFAULT 1,
			FOREIGN KEY (category_id) REFERENCES pattern_categories(id)
		)`,
		`CREATE TABLE IF NOT EXISTS games (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			status TEXT NOT NULL DEFAULT 'active',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			winners_cache TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS game_patterns (
			game_id INTEGER NOT NULL,
			pattern_id INTEGER NOT NULL,
			pattern_name TEXT NOT NULL,
			pattern_data TEXT NOT NULL,
			PRIMARY KEY (game_id, pattern_id),
			FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS called_numbers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			game_id INTEGER NOT NULL,
			number INTEGER NOT NULL,
			call_order INTEGER NOT NULL,
			FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS styles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			css_content TEXT NOT NULL DEFAULT '',
			board_flourish TEXT NOT NULL DEFAULT '',
			number_flourish TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS raffles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			rules TEXT NOT NULL DEFAULT '',
			max_entries INTEGER NOT NULL DEFAULT 1,
			signup_instructions TEXT NOT NULL DEFAULT '',
			cost_per_entry REAL NOT NULL DEFAULT 0,
			available_from TEXT NOT NULL DEFAULT '',
			available_to TEXT NOT NULL DEFAULT '',
			prize_image TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			winner_entry_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS raffle_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			raffle_id INTEGER NOT NULL,
			character_name TEXT NOT NULL,
			world TEXT NOT NULL,
			num_entries INTEGER NOT NULL DEFAULT 1,
			paid INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (raffle_id) REFERENCES raffles(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS winners_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			logged_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			card_id TEXT NOT NULL,
			player_name TEXT NOT NULL DEFAULT '',
			game_details TEXT NOT NULL DEFAULT '',
			winning_patterns TEXT NOT NULL DEFAULT '[]'
		)`,
		`CREATE TABLE IF NOT EXISTS reading_lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			club_slug TEXT NOT NULL DEFAULT 'yaoi',
			title TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS reading_list_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER NOT NULL,
			cover_image TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			format TEXT NOT NULL DEFAULT '',
			genres TEXT NOT NULL DEFAULT '',
			tropes TEXT NOT NULL DEFAULT '',
			chapters TEXT NOT NULL DEFAULT '',
			comments TEXT NOT NULL DEFAULT '',
			sources TEXT NOT NULL DEFAULT '[]',
			sort_order INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (list_id) REFERENCES reading_lists(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS game_presets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			pattern_ids TEXT NOT NULL DEFAULT '[]',
			game_details TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		announcementTypesTableSQL,
		announcementsTableSQL,
		announcementRolesTableSQL,
		garaponsTableSQL,
		garaponPrizesTableSQL,
		garaponPlayersTableSQL,
		garaponDrawsTableSQL,
		affiliatesTableSQL,
		stampRalliesTableSQL,
		stampRallyStampsTableSQL,
		stampRallyPrizesTableSQL,
		stampRallyCardsTableSQL,
		stampRallyCollectedTableSQL,
		userTokensTableSQL,
		teaRoomsTableSQL,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("create tables: %w", err)
		}
	}
	return createIndexes(db)
}

// createIndexes creates all performance indexes. Called both from
// createTables (fresh install) and from the version < 1 migration
// path (upgrading an existing database that predates versioning).
func createIndexes(db *sql.DB) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_games_status ON games(status)",
		"CREATE INDEX IF NOT EXISTS idx_called_numbers_game ON called_numbers(game_id, call_order)",
		// A number can be drawn at most once per game (backstops the draw mutex).
		"CREATE UNIQUE INDEX IF NOT EXISTS idx_called_numbers_game_number ON called_numbers(game_id, number)",
		"CREATE INDEX IF NOT EXISTS idx_game_patterns_game ON game_patterns(game_id)",
		"CREATE INDEX IF NOT EXISTS idx_raffle_entries_raffle ON raffle_entries(raffle_id)",
		"CREATE INDEX IF NOT EXISTS idx_winners_log_logged_at ON winners_log(logged_at)",
		"CREATE INDEX IF NOT EXISTS idx_winners_log_player_date ON winners_log(player_name, logged_at)",
		"CREATE INDEX IF NOT EXISTS idx_cards_player_name ON cards(player_name)",
		"CREATE INDEX IF NOT EXISTS idx_reading_list_items_list ON reading_list_items(list_id)",
		"CREATE INDEX IF NOT EXISTS idx_announcements_type ON announcements(type_id)",
		"CREATE INDEX IF NOT EXISTS idx_announcements_due ON announcements(active, next_post_at)",
		"CREATE INDEX IF NOT EXISTS idx_garapon_prizes_garapon ON garapon_prizes(garapon_id)",
		"CREATE INDEX IF NOT EXISTS idx_garapon_players_garapon ON garapon_players(garapon_id)",
		"CREATE INDEX IF NOT EXISTS idx_garapon_players_token ON garapon_players(token)",
		"CREATE INDEX IF NOT EXISTS idx_garapon_draws_garapon ON garapon_draws(garapon_id)",
		"CREATE INDEX IF NOT EXISTS idx_garapon_draws_player ON garapon_draws(player_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_stamps_rally ON stamp_rally_stamps(rally_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_prizes_rally ON stamp_rally_prizes(rally_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_cards_rally ON stamp_rally_cards(rally_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_cards_token ON stamp_rally_cards(token)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_card ON stamp_rally_collected(card_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_stamp ON stamp_rally_collected(stamp_id)",
		"CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_rally ON stamp_rally_collected(rally_id)",
		"CREATE INDEX IF NOT EXISTS idx_tea_rooms_sort ON tea_rooms(sort_order, id)",
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return fmt.Errorf("create indexes: %w", err)
		}
	}
	return nil
}

// migrateSortOrder adds the sort_order column to the patterns table
// for databases created before pattern ordering was supported.
// Backfills sort_order based on insertion order (id).
func migrateSortOrder(db *sql.DB) error {
	if hasColumn(db, "patterns", "sort_order") {
		return nil
	}
	if _, err := db.Exec("ALTER TABLE patterns ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	_, err := db.Exec(`UPDATE patterns SET sort_order = (
		SELECT COUNT(*) FROM patterns AS p2 WHERE p2.id <= patterns.id
	) WHERE sort_order = 0`)
	return err
}

// migrateWinnersCache adds the winners_cache JSON column to the games
// table so winner card IDs can be stored without a separate table.
func migrateWinnersCache(db *sql.DB) error {
	if hasColumn(db, "games", "winners_cache") {
		return nil
	}
	_, err := db.Exec("ALTER TABLE games ADD COLUMN winners_cache TEXT NOT NULL DEFAULT '[]'")
	return err
}

// columnInfo reports whether a table has the given column and, if so, whether it
// is declared NOT NULL — both read from PRAGMA table_info. A missing table (or a
// query error) reports (false, false). Backs hasColumn (idempotent ALTER TABLE
// guards) and the v36 garapon_draws schema detection.
func columnInfo(db *sql.DB, table, column string) (exists, notNull bool) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return false, false
		}
		if name == column {
			return true, notnull == 1
		}
	}
	return false, false
}

// hasColumn checks whether a table has a given column. Used by migration
// functions to make ALTER TABLE operations idempotent (safe to re-run).
func hasColumn(db *sql.DB, table, column string) bool {
	exists, _ := columnInfo(db, table, column)
	return exists
}

// tableExists reports whether a table with the given name exists.
func tableExists(db *sql.DB, name string) bool {
	var found string
	err := db.QueryRow(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, name,
	).Scan(&found)
	return err == nil
}

// migrateSettings creates the key-value settings table for app configuration
// (e.g. app_title, default_draw_delay, active_style_id).
func migrateSettings(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// migrateStyles creates the styles table for custom CSS themes.
func migrateStyles(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS styles (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		css_content TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	return err
}

// migratePatternCategories creates the pattern_categories table and adds
// the category_id foreign key column to patterns. Seeds a default
// "Standard" category and assigns all existing patterns to it.
func migratePatternCategories(db *sql.DB) error {
	// Create the categories table.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS pattern_categories (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		sort_order INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return err
	}

	// Insert the default "Standard" category.
	if _, err := db.Exec(`INSERT OR IGNORE INTO pattern_categories (id, name, sort_order) VALUES (1, 'Standard', 0)`); err != nil {
		return err
	}

	// Add category_id column to patterns if it doesn't exist.
	if !hasColumn(db, "patterns", "category_id") {
		if _, err := db.Exec("ALTER TABLE patterns ADD COLUMN category_id INTEGER NOT NULL DEFAULT 1"); err != nil {
			return err
		}
	}

	// Ensure all existing patterns point to the default category.
	if _, err := db.Exec("UPDATE patterns SET category_id = 1 WHERE category_id = 0 OR category_id IS NULL"); err != nil {
		return err
	}

	return nil
}

// migrateRaffles creates the raffles and raffle_entries tables with
// their associated index on raffle_id.
func migrateRaffles(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS raffles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			rules TEXT NOT NULL DEFAULT '',
			max_entries INTEGER NOT NULL DEFAULT 1,
			signup_instructions TEXT NOT NULL DEFAULT '',
			cost_per_entry REAL NOT NULL DEFAULT 0,
			available_from TEXT NOT NULL DEFAULT '',
			available_to TEXT NOT NULL DEFAULT '',
			prize_image TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'open',
			winner_entry_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS raffle_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			raffle_id INTEGER NOT NULL,
			character_name TEXT NOT NULL,
			world TEXT NOT NULL,
			num_entries INTEGER NOT NULL DEFAULT 1,
			paid INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (raffle_id) REFERENCES raffles(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_raffle_entries_raffle ON raffle_entries(raffle_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate raffles: %w", err)
		}
	}
	return nil
}

// migrateWinnersLog adds player_name and details columns to the cards
// table (if missing) and creates the winners_log table for tracking
// game outcomes over time.
func migrateWinnersLog(db *sql.DB) error {
	// Add player_name and details columns to cards table.
	if !hasColumn(db, "cards", "player_name") {
		if _, err := db.Exec("ALTER TABLE cards ADD COLUMN player_name TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}
	if !hasColumn(db, "cards", "details") {
		if _, err := db.Exec("ALTER TABLE cards ADD COLUMN details TEXT NOT NULL DEFAULT ''"); err != nil {
			return err
		}
	}

	// Create winners_log table.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS winners_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		logged_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		card_id TEXT NOT NULL,
		player_name TEXT NOT NULL DEFAULT '',
		game_details TEXT NOT NULL DEFAULT '',
		winning_patterns TEXT NOT NULL DEFAULT '[]'
	)`); err != nil {
		return err
	}
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_winners_log_logged_at ON winners_log(logged_at)")
	return err
}

// migrateWinnersLogIndex adds a composite index on (player_name, logged_at)
// to the winners_log table for efficient frequent-winner queries.
func migrateWinnersLogIndex(db *sql.DB) error {
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_winners_log_player_date ON winners_log(player_name, logged_at)")
	return err
}

// migrateCardPlayerNameIndex adds an index on cards(player_name) for
// faster lookups when searching or filtering cards by player name.
func migrateCardPlayerNameIndex(db *sql.DB) error {
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_cards_player_name ON cards(player_name)")
	return err
}

// migrateBookClubs creates the reading_lists and reading_list_items tables
// (plus the list_id index) for the Book Clubs admin section. Item sources are
// stored as a JSON array in the reading_list_items.sources column.
func migrateBookClubs(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS reading_lists (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			club_slug TEXT NOT NULL DEFAULT 'yaoi',
			title TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS reading_list_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			list_id INTEGER NOT NULL,
			cover_image TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			summary TEXT NOT NULL DEFAULT '',
			format TEXT NOT NULL DEFAULT '',
			genres TEXT NOT NULL DEFAULT '',
			tropes TEXT NOT NULL DEFAULT '',
			chapters TEXT NOT NULL DEFAULT '',
			comments TEXT NOT NULL DEFAULT '',
			sources TEXT NOT NULL DEFAULT '[]',
			sort_order INTEGER NOT NULL DEFAULT 0,
			FOREIGN KEY (list_id) REFERENCES reading_lists(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_reading_list_items_list ON reading_list_items(list_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate book clubs: %w", err)
		}
	}
	return nil
}

// migrateBookClubTropes adds the tropes column to reading_list_items for
// databases created before the Tropes field existed. Idempotent via hasColumn.
func migrateBookClubTropes(db *sql.DB) error {
	if hasColumn(db, "reading_list_items", "tropes") {
		return nil
	}
	_, err := db.Exec("ALTER TABLE reading_list_items ADD COLUMN tropes TEXT NOT NULL DEFAULT ''")
	return err
}

// migrateRaffleImagePaths rewrites legacy raffle prize-image web paths. When the
// uploads directory was moved out of `assets/` into a top-level `images/`
// folder (so it no longer collides with the Vite `dist/assets/` output).
// Old rows stored paths like "assets/images/raffles/raffle_….png"; rewrite
// the prefix to "images/raffles/…". A leading-slash variant is handled too.
func migrateRaffleImagePaths(db *sql.DB) error {
	stmts := []string{
		`UPDATE raffles SET prize_image = 'images/raffles/' || substr(prize_image, length('assets/images/raffles/') + 1)
		   WHERE prize_image LIKE 'assets/images/raffles/%'`,
		`UPDATE raffles SET prize_image = '/images/raffles/' || substr(prize_image, length('/assets/images/raffles/') + 1)
		   WHERE prize_image LIKE '/assets/images/raffles/%'`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// announcementTypesTableSQL / announcementsTableSQL define the Announcement
// Management tables. They're shared between createTables (fresh install) and
// migrateAnnouncements (existing databases) so the schema is defined once.
const announcementTypesTableSQL = `CREATE TABLE IF NOT EXISTS announcement_types (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	webhook_url TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const announcementsTableSQL = `CREATE TABLE IF NOT EXISTS announcements (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	type_id INTEGER NOT NULL,
	title TEXT NOT NULL,
	details TEXT NOT NULL DEFAULT '',
	image TEXT NOT NULL DEFAULT '',
	thumbnail TEXT NOT NULL DEFAULT '',
	color TEXT NOT NULL DEFAULT '',
	location TEXT NOT NULL DEFAULT '',
	start_local TEXT NOT NULL DEFAULT '',
	end_local TEXT NOT NULL DEFAULT '',
	start_at TEXT NOT NULL DEFAULT '',
	end_at TEXT NOT NULL DEFAULT '',
	start_format TEXT NOT NULL DEFAULT '',
	end_format TEXT NOT NULL DEFAULT '',
	dynamic_dates INTEGER NOT NULL DEFAULT 0,
	schedule_kind TEXT NOT NULL DEFAULT '',
	timezone TEXT NOT NULL DEFAULT '',
	once_local TEXT NOT NULL DEFAULT '',
	schedule_minutes INTEGER NOT NULL DEFAULT 0,
	schedule_weekdays TEXT NOT NULL DEFAULT '',
	schedule_week_of_month INTEGER NOT NULL DEFAULT 0,
	next_post_at TEXT NOT NULL DEFAULT '',
	skip_next INTEGER NOT NULL DEFAULT 0,
	active INTEGER NOT NULL DEFAULT 1,
	last_posted_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	buttons TEXT NOT NULL DEFAULT '[]',
	mention TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (type_id) REFERENCES announcement_types(id)
)`

// announcementRolesTableSQL defines the managed list of taggable Discord roles
// (a friendly name + the role's Discord ID) an announcement can optionally ping.
const announcementRolesTableSQL = `CREATE TABLE IF NOT EXISTS announcement_roles (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	role_id TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// migrateAnnouncements creates the Announcement Management tables
// (announcement_types + announcements) and their indexes. Idempotent.
func migrateAnnouncements(db *sql.DB) error {
	stmts := []string{
		announcementTypesTableSQL,
		announcementsTableSQL,
		`CREATE INDEX IF NOT EXISTS idx_announcements_type ON announcements(type_id)`,
		`CREATE INDEX IF NOT EXISTS idx_announcements_due ON announcements(active, next_post_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate announcements: %w", err)
		}
	}
	return nil
}

// migrateAnnouncementTimezone adds the optional IANA `timezone` column to the
// announcements table (anchors recurring times so they survive DST). Idempotent.
func migrateAnnouncementTimezone(db *sql.DB) error {
	if hasColumn(db, "announcements", "timezone") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN timezone TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add announcements.timezone: %w", err)
	}
	return nil
}

// migrateAnnouncementLocalTimes adds the wall-clock columns (start_local,
// end_local, once_local) so every time on an announcement is anchored to its
// timezone. Idempotent.
func migrateAnnouncementLocalTimes(db *sql.DB) error {
	for _, col := range []string{"start_local", "end_local", "once_local"} {
		if hasColumn(db, "announcements", col) {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add announcements.%s: %w", col, err)
		}
	}
	return nil
}

// migrateAnnouncementColor adds the optional `color` column (a "#rrggbb" embed
// accent colour; empty falls back to the brand default). Idempotent.
func migrateAnnouncementColor(db *sql.DB) error {
	if hasColumn(db, "announcements", "color") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN color TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add announcements.color: %w", err)
	}
	return nil
}

// migrateAnnouncementButtons adds the optional `buttons` column (a JSON array of
// Discord link buttons rendered beneath the embed; "[]" when none). Idempotent.
func migrateAnnouncementButtons(db *sql.DB) error {
	if hasColumn(db, "announcements", "buttons") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN buttons TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return fmt.Errorf("add announcements.buttons: %w", err)
	}
	return nil
}

// migrateAnnouncementRoles creates the announcement_roles table (the managed list
// of taggable Discord roles). Idempotent.
func migrateAnnouncementRoles(db *sql.DB) error {
	if _, err := db.Exec(announcementRolesTableSQL); err != nil {
		return fmt.Errorf("migrate announcement roles: %w", err)
	}
	return nil
}

// migrateAnnouncementMention adds the optional `mention` column (the role tag an
// announcement posts above its embed: ""|"everyone"|"role:<id>"). Idempotent.
func migrateAnnouncementMention(db *sql.DB) error {
	if hasColumn(db, "announcements", "mention") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN mention TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add announcements.mention: %w", err)
	}
	return nil
}

// migrateAnnouncementThumbnail adds the optional `thumbnail` column (full URL of
// the small top-right embed thumbnail, distinct from the large bottom image).
// Idempotent.
func migrateAnnouncementThumbnail(db *sql.DB) error {
	if !tableExists(db, "announcements") || hasColumn(db, "announcements", "thumbnail") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN thumbnail TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add announcements.thumbnail: %w", err)
	}
	return nil
}

// migrateAnnouncementDynamicDates adds the optional `dynamic_dates` flag: when
// set, the embed re-anchors the event start/end onto the day each post goes out
// (for recurring day-of event announcements). Idempotent.
func migrateAnnouncementDynamicDates(db *sql.DB) error {
	if !tableExists(db, "announcements") || hasColumn(db, "announcements", "dynamic_dates") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN dynamic_dates INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add announcements.dynamic_dates: %w", err)
	}
	return nil
}

// migrateAnnouncementSortOrder adds the `sort_order` column used for the admin's
// drag-and-drop ordering of the announcement list. Existing rows default to 0, so
// the list keeps its previous newest-first order (ListAnnouncements breaks ties on
// id DESC) until the admin reorders. Idempotent.
func migrateAnnouncementSortOrder(db *sql.DB) error {
	if !tableExists(db, "announcements") || hasColumn(db, "announcements", "sort_order") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0`); err != nil {
		return fmt.Errorf("add announcements.sort_order: %w", err)
	}
	return nil
}

// migrateStyleFlourishes adds the per-theme `board_flourish`/`number_flourish`
// columns (root-relative paths into images/flourishes; "" = built-in art).
// Idempotent.
func migrateStyleFlourishes(db *sql.DB) error {
	if !tableExists(db, "styles") {
		return nil
	}
	if !hasColumn(db, "styles", "board_flourish") {
		if _, err := db.Exec(`ALTER TABLE styles ADD COLUMN board_flourish TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add styles.board_flourish: %w", err)
		}
	}
	if !hasColumn(db, "styles", "number_flourish") {
		if _, err := db.Exec(`ALTER TABLE styles ADD COLUMN number_flourish TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add styles.number_flourish: %w", err)
		}
	}
	return nil
}

// migrateStyleTokens moves themes from free-form `css_content` blobs to
// structured design-token storage. It adds a `tokens` column (a JSON map of
// token name → CSS value), backfills it by parsing the `:root{…}` block out of
// each theme's existing css_content (keeping only known tokens), and then drops
// the now-unused css_content column. The applied stylesheet is generated from
// the tokens at read time (see TokensToCSS), so themes can no longer carry
// arbitrary CSS. Idempotent.
func migrateStyleTokens(db *sql.DB) error {
	if !tableExists(db, "styles") {
		return nil
	}
	if !hasColumn(db, "styles", "tokens") {
		if _, err := db.Exec(`ALTER TABLE styles ADD COLUMN tokens TEXT NOT NULL DEFAULT '{}'`); err != nil {
			return fmt.Errorf("add styles.tokens: %w", err)
		}
	}

	// Backfill tokens from the old css_content, but only while that column still
	// exists (so a re-run after the drop below is a no-op).
	if hasColumn(db, "styles", "css_content") {
		rows, err := db.Query(`SELECT id, css_content FROM styles WHERE tokens = '' OR tokens = '{}'`)
		if err != nil {
			return fmt.Errorf("read styles for token backfill: %w", err)
		}
		type pending struct {
			id   int64
			json string
		}
		var updates []pending
		for rows.Next() {
			var id int64
			var css string
			if err := rows.Scan(&id, &css); err != nil {
				rows.Close()
				return fmt.Errorf("scan style for token backfill: %w", err)
			}
			tokens := parseRootTokens(css)
			buf, err := json.Marshal(tokens)
			if err != nil {
				rows.Close()
				return fmt.Errorf("marshal backfilled tokens: %w", err)
			}
			updates = append(updates, pending{id: id, json: string(buf)})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return fmt.Errorf("iterate styles for token backfill: %w", err)
		}
		rows.Close()
		for _, u := range updates {
			if _, err := db.Exec(`UPDATE styles SET tokens = ? WHERE id = ?`, u.json, u.id); err != nil {
				return fmt.Errorf("write backfilled tokens: %w", err)
			}
		}

		// Drop the legacy column now that tokens are the source of truth. SQLite
		// supports DROP COLUMN (3.35+); if an older engine ever rejects it, leave
		// the column in place (harmless, unused) rather than failing startup.
		if _, err := db.Exec(`ALTER TABLE styles DROP COLUMN css_content`); err != nil {
			// non-fatal: the column simply remains as dead, unused data.
			_ = err
		}
	}
	return nil
}

// migrateAnnouncementLocation adds the optional `location` column (free-text
// location shown on the embed, e.g. a Discord voice channel). Idempotent.
func migrateAnnouncementLocation(db *sql.DB) error {
	if hasColumn(db, "announcements", "location") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN location TEXT NOT NULL DEFAULT ''`); err != nil {
		return fmt.Errorf("add announcements.location: %w", err)
	}
	return nil
}

// migrateAnnouncementTimeFormats adds the optional `start_format` / `end_format`
// columns (the Discord timestamp style letters — t|T|d|D|f|F|R — used to render
// the embed's start and end times independently; "" falls back to the defaults).
// Idempotent.
func migrateAnnouncementTimeFormats(db *sql.DB) error {
	for _, col := range []string{"start_format", "end_format"} {
		if hasColumn(db, "announcements", col) {
			continue
		}
		if _, err := db.Exec(`ALTER TABLE announcements ADD COLUMN ` + col + ` TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add announcements.%s: %w", col, err)
		}
	}
	return nil
}

// migrateCardCreatedAt adds the `created_at` column to the cards table so the
// Manage Cards table can show when each card was generated. SQLite forbids a
// CURRENT_TIMESTAMP default on ADD COLUMN, so the column is nullable here (older
// cards predate tracking and read as NULL → blank); new inserts set it
// explicitly. Idempotent.
func migrateCardCreatedAt(db *sql.DB) error {
	// A fresh/partial DB may not have the cards table yet (createTables builds it
	// with the column); only alter an existing one.
	if !tableExists(db, "cards") || hasColumn(db, "cards", "created_at") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE cards ADD COLUMN created_at DATETIME`); err != nil {
		return fmt.Errorf("add cards.created_at: %w", err)
	}
	return nil
}

// migrateBookClubEventWebhooks merges the retired book-club "event posts" feature
// into the Announcements tool: each per-club events-channel webhook (settings key
// discord_events_webhook_url_<slug>) becomes a dedicated announcement type so the
// club can author event posts there instead. The old settings rows are removed
// afterward. Idempotent via the schema-version gate; a no-op when none are set.
func migrateBookClubEventWebhooks(db *sql.DB) error {
	// Nothing to migrate if neither table is present (e.g. a partial/legacy DB).
	if !tableExists(db, "settings") || !tableExists(db, "announcement_types") {
		return nil
	}
	rows, err := db.Query(
		`SELECT key, value FROM settings
		   WHERE key LIKE 'discord_events_webhook_url_%' AND TRIM(value) != ''`)
	if err != nil {
		return fmt.Errorf("read events webhooks: %w", err)
	}
	type webhook struct{ key, slug, url string }
	var found []webhook
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			rows.Close()
			return fmt.Errorf("scan events webhook: %w", err)
		}
		slug := strings.TrimPrefix(key, "discord_events_webhook_url_")
		found = append(found, webhook{key: key, slug: slug, url: strings.TrimSpace(value)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate events webhooks: %w", err)
	}
	rows.Close()

	// Insert-then-delete each webhook inside one transaction so a crash between
	// the INSERT and the DELETE can't leave the setting behind for this migration
	// (which isn't version-gated as committed until all migrations finish) to
	// re-insert on the next boot — which would accumulate duplicate types. The
	// NOT-EXISTS guard makes it idempotent even if a prior partial run committed.
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin events webhook migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, w := range found {
		name := titleCaseSlug(w.slug) + " Book Club Events"
		var exists int
		if err := tx.QueryRow(
			`SELECT COUNT(*) FROM announcement_types WHERE name = ? AND webhook_url = ?`, name, w.url,
		).Scan(&exists); err != nil {
			return fmt.Errorf("check announcement type for %q: %w", w.slug, err)
		}
		if exists == 0 {
			if _, err := tx.Exec(
				`INSERT INTO announcement_types (name, webhook_url) VALUES (?, ?)`, name, w.url,
			); err != nil {
				return fmt.Errorf("create announcement type for %q: %w", w.slug, err)
			}
		}
		if _, err := tx.Exec(`DELETE FROM settings WHERE key = ?`, w.key); err != nil {
			return fmt.Errorf("remove events webhook setting %q: %w", w.key, err)
		}
	}
	return tx.Commit()
}

// titleCaseSlug turns a slug like "yaoi" or "yuri-extra" into a friendly,
// space-separated, title-cased label ("Yaoi", "Yuri Extra") for a migrated
// announcement type name.
func titleCaseSlug(slug string) string {
	parts := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	out := strings.Join(parts, " ")
	if out == "" {
		return "Book Club"
	}
	return out
}

// migrateDropBookClubEvents removes the retired book_club_events table and its
// indexes (the feature was merged into Announcements). Idempotent.
func migrateDropBookClubEvents(db *sql.DB) error {
	stmts := []string{
		`DROP INDEX IF EXISTS idx_book_club_events_due`,
		`DROP INDEX IF EXISTS idx_book_club_events_club`,
		`DROP TABLE IF EXISTS book_club_events`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("drop book_club_events: %w", err)
		}
	}
	return nil
}

// migrateUsers creates the users table for the admin-account + per-page
// permission system and seeds a bootstrap "admin" account (username "admin",
// password "admin", full access). Runs on both fresh and existing databases;
// the seed uses INSERT OR IGNORE so it is safe to re-run and never clobbers an
// admin whose password was already changed. The seeded password is intended to
// be rotated immediately after the migration runs.
func migrateUsers(db *sql.DB) error {
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		is_admin INTEGER NOT NULL DEFAULT 0,
		is_active INTEGER NOT NULL DEFAULT 0,
		permissions TEXT NOT NULL DEFAULT '[]',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		last_login_at DATETIME
	)`); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}

	hash, err := auth.Hash("admin")
	if err != nil {
		return fmt.Errorf("hash seed admin password: %w", err)
	}
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO users (username, password_hash, is_admin, is_active, permissions)
		 VALUES ('admin', ?, 1, 1, '[]')`, hash); err != nil {
		return fmt.Errorf("seed admin user: %w", err)
	}
	return nil
}

// migrateUserLastLogin adds the nullable last_login_at column to the users
// table (NULL until an account's first login). Idempotent via hasColumn.
func migrateUserLastLogin(db *sql.DB) error {
	if hasColumn(db, "users", "last_login_at") {
		return nil
	}
	if _, err := db.Exec(`ALTER TABLE users ADD COLUMN last_login_at DATETIME`); err != nil {
		return fmt.Errorf("add users.last_login_at: %w", err)
	}
	return nil
}

// userTokensTableSQL defines the personal-access-token table backing the
// plugin/API bearer auth (one active token per account). Only a SHA-256 hash of
// the token is stored — the plaintext is shown to the user exactly once, at
// generation — so a database leak can't yield usable tokens. token_prefix keeps
// the first few visible characters for at-a-glance identification in the UI. The
// row cascade-deletes with its user. Shared between createTables (fresh install)
// and migrateUserTokens (existing databases) so the schema is defined once.
const userTokensTableSQL = `CREATE TABLE IF NOT EXISTS user_tokens (
	user_id INTEGER PRIMARY KEY,
	token_hash TEXT NOT NULL UNIQUE,
	token_prefix TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME,
	FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
)`

// migrateUserTokens creates the user_tokens table (schema v42) for personal
// access tokens used by external API clients (e.g. the FFXIV plugin). Idempotent.
func migrateUserTokens(db *sql.DB) error {
	if _, err := db.Exec(userTokensTableSQL); err != nil {
		return fmt.Errorf("migrate user tokens: %w", err)
	}
	return nil
}

// migrateGamePresets creates the game_presets table for reusable game templates
// (pre-selected win patterns + pre-written markdown game details).
func migrateGamePresets(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS game_presets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		pattern_ids TEXT NOT NULL DEFAULT '[]',
		game_details TEXT NOT NULL DEFAULT '',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		return fmt.Errorf("migrate game presets: %w", err)
	}
	return nil
}

// garapon*TableSQL define the Garapon festival lottery-drum tables. Shared between
// createTables (fresh install) and migrateGarapons (existing databases) so the
// schema is defined once. A garapon has prize tiers (each a ball color + weight),
// tokenized per-player drawing links, and a draw log. The prize/player/draw rows
// cascade-delete with their garapon (and draws with their player).
const garaponsTableSQL = `CREATE TABLE IF NOT EXISTS garapons (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	details TEXT NOT NULL DEFAULT '',
	grand_prize_image TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open',
	stamp_rally_id INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const garaponPrizesTableSQL = `CREATE TABLE IF NOT EXISTS garapon_prizes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	garapon_id INTEGER NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	ball_color TEXT NOT NULL DEFAULT '',
	rate REAL NOT NULL DEFAULT 0,
	is_grand INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE
)`

const garaponPlayersTableSQL = `CREATE TABLE IF NOT EXISTS garapon_players (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	garapon_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	player_name TEXT NOT NULL DEFAULT '',
	max_draws INTEGER NOT NULL DEFAULT 1,
	stamp_card_id INTEGER,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE
)`

// player_id is nullable with ON DELETE SET NULL (not CASCADE): deleting a drawing
// link detaches its draws but KEEPS them in the log (the draw snapshots
// player_name/prize_name/ball_color, so the record survives). Deleting the whole
// garapon still removes every draw via the garapon_id CASCADE.
const garaponDrawsTableSQL = `CREATE TABLE IF NOT EXISTS garapon_draws (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	garapon_id INTEGER NOT NULL,
	player_id INTEGER,
	prize_id INTEGER NOT NULL DEFAULT 0,
	player_name TEXT NOT NULL DEFAULT '',
	prize_name TEXT NOT NULL DEFAULT '',
	ball_color TEXT NOT NULL DEFAULT '',
	drawn_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE,
	FOREIGN KEY (player_id) REFERENCES garapon_players(id) ON DELETE SET NULL
)`

// affiliatesTableSQL defines the Affiliates table (Senpan Tea House → Affiliates):
// a partner establishment with one or more owners and opening-hours ranges stored
// as JSON arrays (owners/hours), a single timezone anchoring those hours, markdown
// details, and a logo + screenshot picked from the shared image library.
// Shared between createTables (fresh install) and migrateAffiliates
// (existing databases) so the schema is defined once.
const affiliatesTableSQL = `CREATE TABLE IF NOT EXISTS affiliates (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	owners TEXT NOT NULL DEFAULT '[]',
	location TEXT NOT NULL DEFAULT '',
	timezone TEXT NOT NULL DEFAULT '',
	hours TEXT NOT NULL DEFAULT '[]',
	details TEXT NOT NULL DEFAULT '',
	logo TEXT NOT NULL DEFAULT '',
	screenshot TEXT NOT NULL DEFAULT '',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// migrateAffiliates creates the affiliates table for the Affiliates admin section.
// Owners and opening hours are stored as JSON arrays in the owners/hours columns.
// Idempotent (CREATE TABLE IF NOT EXISTS).
func migrateAffiliates(db *sql.DB) error {
	if _, err := db.Exec(affiliatesTableSQL); err != nil {
		return fmt.Errorf("migrate affiliates: %w", err)
	}
	return nil
}

// teaRoomsTableSQL defines the Tea Rooms table (Senpan Tea House → Tea Rooms): a
// bookable room with a per-half-hour gil cost, hashtags, markdown description, a
// handful of status flags, an image picked from the shared library, and a Discord
// embed accent colour. sort_order backs the admin's drag-and-drop ordering. Shared
// between createTables (fresh install) and migrateTeaRooms (existing databases) so
// the schema is defined once.
const teaRoomsTableSQL = `CREATE TABLE IF NOT EXISTS tea_rooms (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	room_number TEXT NOT NULL DEFAULT '',
	cost_per_half_hour INTEGER NOT NULL DEFAULT 0,
	hashtags TEXT NOT NULL DEFAULT '',
	description TEXT NOT NULL DEFAULT '',
	seasonal INTEGER NOT NULL DEFAULT 0,
	open INTEGER NOT NULL DEFAULT 1,
	lockable INTEGER NOT NULL DEFAULT 0,
	discounted INTEGER NOT NULL DEFAULT 0,
	image TEXT NOT NULL DEFAULT '',
	color TEXT NOT NULL DEFAULT '',
	sort_order INTEGER NOT NULL DEFAULT 0,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

// migrateTeaRooms creates the tea_rooms table (+ its ordering index) for the Tea
// Rooms admin section. Idempotent (CREATE TABLE / INDEX IF NOT EXISTS).
func migrateTeaRooms(db *sql.DB) error {
	stmts := []string{
		teaRoomsTableSQL,
		`CREATE INDEX IF NOT EXISTS idx_tea_rooms_sort ON tea_rooms(sort_order, id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate tea rooms: %w", err)
		}
	}
	return nil
}

// stampRally*TableSQL define the Stamp Rally tables (Festival → Stamp Rally). An
// event (stamp_rallies) owns its stamps and prizes (each carries a %-based placement
// on the card image), plus tokenized per-participant cards and a collected-stamp log.
// Stamp/prize/card/collected rows cascade-delete with their rally; a collected row
// also cascades with its stamp. Shared between createTables (fresh install) and
// migrateStampRally (existing databases) so the schema is defined once.
//
// Note: stamp placements use a single embedded set of pos_* columns; the affiliate FK
// is nullable (NULL = the "Senpan Tea House" default) and ON DELETE SET NULL so
// removing an affiliate detaches its stamps rather than deleting them.
const stampRalliesTableSQL = `CREATE TABLE IF NOT EXISTS stamp_rallies (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	card_image TEXT NOT NULL DEFAULT '',
	not_stamped_image TEXT NOT NULL DEFAULT '',
	available_from TEXT NOT NULL DEFAULT '',
	available_to TEXT NOT NULL DEFAULT '',
	details TEXT NOT NULL DEFAULT '',
	redeem_instructions TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'open',
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
)`

const stampRallyStampsTableSQL = `CREATE TABLE IF NOT EXISTS stamp_rally_stamps (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	rally_id INTEGER NOT NULL,
	affiliate_id INTEGER,
	image TEXT NOT NULL DEFAULT '',
	password TEXT NOT NULL DEFAULT '',
	pos_x REAL NOT NULL DEFAULT 0,
	pos_y REAL NOT NULL DEFAULT 0,
	width REAL NOT NULL DEFAULT 20,
	height REAL NOT NULL DEFAULT 20,
	rotation REAL NOT NULL DEFAULT 0,
	active_from TEXT NOT NULL DEFAULT '',
	active_to TEXT NOT NULL DEFAULT '',
	paused INTEGER NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE,
	FOREIGN KEY (affiliate_id) REFERENCES affiliates(id) ON DELETE SET NULL
)`

const stampRallyPrizesTableSQL = `CREATE TABLE IF NOT EXISTS stamp_rally_prizes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	rally_id INTEGER NOT NULL,
	name TEXT NOT NULL DEFAULT '',
	image TEXT NOT NULL DEFAULT '',
	pos_x REAL NOT NULL DEFAULT 0,
	pos_y REAL NOT NULL DEFAULT 0,
	width REAL NOT NULL DEFAULT 20,
	height REAL NOT NULL DEFAULT 20,
	rotation REAL NOT NULL DEFAULT 0,
	sort_order INTEGER NOT NULL DEFAULT 0,
	FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE
)`

const stampRallyCardsTableSQL = `CREATE TABLE IF NOT EXISTS stamp_rally_cards (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	rally_id INTEGER NOT NULL,
	token TEXT NOT NULL UNIQUE,
	participant_name TEXT NOT NULL DEFAULT '',
	completed INTEGER NOT NULL DEFAULT 0,
	completed_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE
)`

// stamp_rally_collected snapshots participant_name + stall_name so the log survives
// card/stamp deletion. Like garapon_draws it carries both a rally_id (ON DELETE
// CASCADE — deleting the whole rally removes its log) and a nullable card_id/stamp_id
// (ON DELETE SET NULL — deleting just a card/stamp KEEPS the log row). UNIQUE
// (card_id, stamp_id) still prevents double-collection while a card exists; once the
// card is gone its card_id is NULL and NULLs don't collide.
const stampRallyCollectedTableSQL = `CREATE TABLE IF NOT EXISTS stamp_rally_collected (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	rally_id INTEGER NOT NULL,
	card_id INTEGER,
	stamp_id INTEGER,
	participant_name TEXT NOT NULL DEFAULT '',
	stall_name TEXT NOT NULL DEFAULT '',
	stamped_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	UNIQUE (card_id, stamp_id),
	FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE,
	FOREIGN KEY (card_id) REFERENCES stamp_rally_cards(id) ON DELETE SET NULL,
	FOREIGN KEY (stamp_id) REFERENCES stamp_rally_stamps(id) ON DELETE SET NULL
)`

// migrateStampRally creates the Stamp Rally tables (stamp_rallies +
// stamp_rally_stamps + stamp_rally_prizes + stamp_rally_cards +
// stamp_rally_collected) and their indexes. Idempotent.
func migrateStampRally(db *sql.DB) error {
	stmts := []string{
		stampRalliesTableSQL,
		stampRallyStampsTableSQL,
		stampRallyPrizesTableSQL,
		stampRallyCardsTableSQL,
		stampRallyCollectedTableSQL,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_stamps_rally ON stamp_rally_stamps(rally_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_prizes_rally ON stamp_rally_prizes(rally_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_cards_rally ON stamp_rally_cards(rally_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_cards_token ON stamp_rally_cards(token)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_card ON stamp_rally_collected(card_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_stamp ON stamp_rally_collected(stamp_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_rally ON stamp_rally_collected(rally_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate stamp rally: %w", err)
		}
	}
	return nil
}

// migrateStampRallyGaraponLink (schema v40) adds the open/closed status to stamp
// rallies and the link columns that tie a garapon to a stamp rally: a garapon may
// link to a rally (garapons.stamp_rally_id), and each drawing link issued on a linked
// garapon also issues a stamp card recorded on the player (garapon_players.stamp_card_id).
// The link columns are kept FK-less here (ALTER can't add FKs and the createTables order
// puts garapons before stamp_rallies); the store nulls/cleans them on delete. Idempotent.
func migrateStampRallyGaraponLink(db *sql.DB) error {
	if tableExists(db, "stamp_rallies") && !hasColumn(db, "stamp_rallies", "status") {
		if _, err := db.Exec(`ALTER TABLE stamp_rallies ADD COLUMN status TEXT NOT NULL DEFAULT 'open'`); err != nil {
			return fmt.Errorf("add stamp_rallies.status: %w", err)
		}
	}
	if tableExists(db, "garapons") && !hasColumn(db, "garapons", "stamp_rally_id") {
		if _, err := db.Exec(`ALTER TABLE garapons ADD COLUMN stamp_rally_id INTEGER`); err != nil {
			return fmt.Errorf("add garapons.stamp_rally_id: %w", err)
		}
	}
	if tableExists(db, "garapon_players") && !hasColumn(db, "garapon_players", "stamp_card_id") {
		if _, err := db.Exec(`ALTER TABLE garapon_players ADD COLUMN stamp_card_id INTEGER`); err != nil {
			return fmt.Errorf("add garapon_players.stamp_card_id: %w", err)
		}
	}
	return nil
}

// migrateStampRallyKeepLogs (schema v41) rebuilds stamp_rally_collected so a stamp log
// survives the deletion of its card/stamp: it adds rally_id (ON DELETE CASCADE) +
// participant_name/stall_name snapshots and switches card_id/stamp_id to nullable with
// ON DELETE SET NULL. SQLite can't ALTER a foreign key, so the table is rebuilt
// (copy → drop → rename) in a transaction, exactly like migrateGaraponDrawKeepLogs.
// Only runs when the old schema is detected (no rally_id column); a fresh DB built with
// the updated const — or a re-run — is a no-op. The table is a leaf (nothing references
// it), so the rebuild is foreign-key-safe.
func migrateStampRallyKeepLogs(db *sql.DB) error {
	if !tableExists(db, "stamp_rally_collected") || hasColumn(db, "stamp_rally_collected", "rally_id") {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		`CREATE TABLE stamp_rally_collected_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			rally_id INTEGER NOT NULL,
			card_id INTEGER,
			stamp_id INTEGER,
			participant_name TEXT NOT NULL DEFAULT '',
			stall_name TEXT NOT NULL DEFAULT '',
			stamped_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE (card_id, stamp_id),
			FOREIGN KEY (rally_id) REFERENCES stamp_rallies(id) ON DELETE CASCADE,
			FOREIGN KEY (card_id) REFERENCES stamp_rally_cards(id) ON DELETE SET NULL,
			FOREIGN KEY (stamp_id) REFERENCES stamp_rally_stamps(id) ON DELETE SET NULL
		)`,
		// Backfill rally_id + the snapshots from the existing joins (old rows always
		// have a non-null card, so rally_id/participant_name resolve).
		`INSERT INTO stamp_rally_collected_new (id, rally_id, card_id, stamp_id, participant_name, stall_name, stamped_at)
			SELECT col.id, c.rally_id, col.card_id, col.stamp_id, c.participant_name,
				COALESCE(a.name, 'Senpan Tea House'), col.stamped_at
			FROM stamp_rally_collected col
			JOIN stamp_rally_cards c ON c.id = col.card_id
			LEFT JOIN stamp_rally_stamps st ON st.id = col.stamp_id
			LEFT JOIN affiliates a ON a.id = st.affiliate_id`,
		`DROP TABLE stamp_rally_collected`,
		`ALTER TABLE stamp_rally_collected_new RENAME TO stamp_rally_collected`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_card ON stamp_rally_collected(card_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_stamp ON stamp_rally_collected(stamp_id)`,
		`CREATE INDEX IF NOT EXISTS idx_stamp_rally_collected_rally ON stamp_rally_collected(rally_id)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("migrate stamp rally keep-logs: %w", err)
		}
	}
	return tx.Commit()
}

// migrateGarapons creates the Garapon tables (garapons + garapon_prizes +
// garapon_players + garapon_draws) and their indexes. Idempotent.
func migrateGarapons(db *sql.DB) error {
	stmts := []string{
		garaponsTableSQL,
		garaponPrizesTableSQL,
		garaponPlayersTableSQL,
		garaponDrawsTableSQL,
		`CREATE INDEX IF NOT EXISTS idx_garapon_prizes_garapon ON garapon_prizes(garapon_id)`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_players_garapon ON garapon_players(garapon_id)`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_players_token ON garapon_players(token)`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_draws_garapon ON garapon_draws(garapon_id)`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_draws_player ON garapon_draws(player_id)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return fmt.Errorf("migrate garapons: %w", err)
		}
	}
	return nil
}

// migrateGaraponDrawKeepLogs rebuilds garapon_draws so player_id is nullable with
// ON DELETE SET NULL (was NOT NULL + CASCADE), so deleting a drawing link keeps
// its draws in the log instead of wiping them. SQLite can't ALTER a foreign key,
// so the table is rebuilt (copy → drop → rename) inside a transaction. Only runs
// when the old schema is detected (player_id NOT NULL), so a fresh DB created with
// the updated const — or a re-run — is a no-op. garapon_draws is a leaf table
// (nothing references it), so the rebuild is foreign-key-safe.
func migrateGaraponDrawKeepLogs(db *sql.DB) error {
	// A missing column/table reports notNull=false, so this also no-ops on a DB
	// that never had garapon_draws.
	if _, notNull := columnInfo(db, "garapon_draws", "player_id"); !notNull {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		`CREATE TABLE garapon_draws_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			garapon_id INTEGER NOT NULL,
			player_id INTEGER,
			prize_id INTEGER NOT NULL DEFAULT 0,
			player_name TEXT NOT NULL DEFAULT '',
			prize_name TEXT NOT NULL DEFAULT '',
			ball_color TEXT NOT NULL DEFAULT '',
			drawn_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (garapon_id) REFERENCES garapons(id) ON DELETE CASCADE,
			FOREIGN KEY (player_id) REFERENCES garapon_players(id) ON DELETE SET NULL
		)`,
		`INSERT INTO garapon_draws_new (id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at)
			SELECT id, garapon_id, player_id, prize_id, player_name, prize_name, ball_color, drawn_at FROM garapon_draws`,
		`DROP TABLE garapon_draws`,
		`ALTER TABLE garapon_draws_new RENAME TO garapon_draws`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_draws_garapon ON garapon_draws(garapon_id)`,
		`CREATE INDEX IF NOT EXISTS idx_garapon_draws_player ON garapon_draws(player_id)`,
	}
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("migrate garapon draws keep-logs: %w", err)
		}
	}
	return tx.Commit()
}
