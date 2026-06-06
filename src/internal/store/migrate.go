package store

import (
	"database/sql"
	"fmt"
)

// schemaVersion is the target database schema version. Each migration
// increments this by one. On startup, ensureSchema compares the stored
// PRAGMA user_version against this constant and runs only the migrations
// needed to bring the database up to date. Bump this when adding a new
// migration block.
const schemaVersion = 10

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

	_, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion))
	return err
}

// createTables builds all tables from scratch for a fresh database.
// Called only when no tables exist (version 0, no prior data).
func createTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS cards (
			id TEXT PRIMARY KEY,
			board_data TEXT NOT NULL,
			player_name TEXT NOT NULL DEFAULT '',
			details TEXT NOT NULL DEFAULT ''
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
		"CREATE INDEX IF NOT EXISTS idx_game_patterns_game ON game_patterns(game_id)",
		"CREATE INDEX IF NOT EXISTS idx_raffle_entries_raffle ON raffle_entries(raffle_id)",
		"CREATE INDEX IF NOT EXISTS idx_winners_log_logged_at ON winners_log(logged_at)",
		"CREATE INDEX IF NOT EXISTS idx_winners_log_player_date ON winners_log(player_name, logged_at)",
		"CREATE INDEX IF NOT EXISTS idx_cards_player_name ON cards(player_name)",
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

// hasColumn checks whether a table has a given column by inspecting
// PRAGMA table_info. Used by migration functions to make ALTER TABLE
// operations idempotent (safe to re-run).
func hasColumn(db *sql.DB, table, column string) bool {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, typ string
		var notnull int
		var dfltValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notnull, &dfltValue, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
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

// migrateRaffleImagePaths rewrites stored prize_image web paths after the
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
