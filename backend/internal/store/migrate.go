package store

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"

	"github.com/pressly/goose/v3"

	"app-suite/internal/auth"
)

// Migrations, as the standards have them (database.md): goose runs the numbered SQL
// files in migrations/ at start, on the write pool, each in its own transaction
// unless it says otherwise, and each also sets PRAGMA user_version to its own
// number, so user_version always equals goose's version.
//
// Version 71 is the baseline. Until 4.1.0 the schema was built by hand-written
// migrations, guarded by user_version, that reached v71; goose took over there.
// Migration 71 builds that schema on an empty database (baseline.sql) and seeds
// it, and leaves a database those migrations brought to v71 as it is. A database
// short of v71 that goose has never seen is refused: a 4.0 release brings it to
// v71 first.

//go:embed migrations/*.sql
var migrationFiles embed.FS

//go:embed baseline.sql
var baselineSQL string

// baselineVersion is the schema the hand-written migrations ended at, where goose
// took over.
const baselineVersion = 71

func provider(write *sql.DB) (*goose.Provider, error) {
	files, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	return goose.NewProvider(goose.DialectSQLite3, write, files,
		goose.WithGoMigrations(goose.NewGoMigration(baselineVersion, &goose.GoFunc{RunTx: baseline}, nil)))
}

// migrate brings the schema up to date.
//
// A database migrated by a NEWER binary is refused rather than run: its tables
// carry columns and constraints this build has never heard of, and starting anyway
// is how a rolled-back deploy quietly corrupts data - the rollback restores the
// previous binary but cannot un-migrate the database. The deploy puts the
// pre-deploy snapshot back with the previous binary for exactly this reason.
func migrate(ctx context.Context, write *sql.DB) error {
	p, err := provider(write)
	if err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	sources := p.ListSources()
	latest := sources[len(sources)-1].Version

	version, err := userVersion(ctx, write)
	if err != nil {
		return err
	}
	if int64(version) > latest {
		return fmt.Errorf(
			"database schema v%d is newer than this binary (v%d); refusing to start - "+
				"restore the pre-deploy snapshot or run a build at or above v%d",
			version, latest, version)
	}
	adopted, err := hasGoose(ctx, write)
	if err != nil {
		return err
	}
	if !adopted && version != 0 && version != baselineVersion {
		return fmt.Errorf(
			"database schema v%d predates goose, which takes over at v%d; "+
				"start a 4.0 release on it once to bring it to v%d, then this one",
			version, baselineVersion, baselineVersion)
	}

	if _, err := p.Up(ctx); err != nil {
		return err
	}
	got, err := p.GetDBVersion(ctx)
	if err != nil {
		return err
	}
	if version, err = userVersion(ctx, write); err != nil {
		return err
	}
	if int64(version) != got {
		return fmt.Errorf("after migrating, user_version is %d where goose's version is %d: a migration did not set it", version, got)
	}
	return nil
}

// baseline is migration 71: the schema the hand-written migrations built, and the
// rows a fresh install needs. On a database those migrations already brought to
// v71 it does nothing.
func baseline(ctx context.Context, tx *sql.Tx) error {
	var version, tables int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == baselineVersion {
		return nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name != 'goose_db_version'`).Scan(&tables); err != nil {
		return err
	}
	if version != 0 || tables != 0 {
		return fmt.Errorf("the baseline needs an empty database or one at v%d; this one is at v%d with %d tables", baselineVersion, version, tables)
	}
	if _, err := tx.ExecContext(ctx, baselineSQL); err != nil {
		return fmt.Errorf("baseline schema: %w", err)
	}

	// The seeds. INSERT OR IGNORE, so a run against a database that has them never
	// overwrites one changed since. The bootstrap account's password is "admin",
	// to be changed as soon as an install creates it (AGENTS.md).
	hash, err := auth.Hash("admin")
	if err != nil {
		return fmt.Errorf("hash the bootstrap password: %w", err)
	}
	seeds := []struct {
		query string
		args  []any
	}{
		{`INSERT OR IGNORE INTO pattern_categories (id, name, sort_order) VALUES (1, 'Standard', 0)`, nil},
		{`INSERT OR IGNORE INTO users (username, password_hash, is_admin, is_active, permissions) VALUES ('admin', ?, 1, 1, '[]')`, []any{hash}},
	}
	for _, seed := range seeds {
		if _, err := tx.ExecContext(ctx, seed.query, seed.args...); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
	}
	_, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", baselineVersion))
	return err
}

func userVersion(ctx context.Context, db *sql.DB) (int, error) {
	var v int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return v, nil
}

// hasGoose reports whether goose has run on the database: its version table is there.
func hasGoose(ctx context.Context, db *sql.DB) (bool, error) {
	var name string
	err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'goose_db_version'`).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
