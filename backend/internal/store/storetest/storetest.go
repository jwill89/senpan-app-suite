// Package storetest gives a test a database of its own with every migration
// applied. The migrations run once for the test binary and each test gets a copy
// of the result: under the race detector migrating takes seconds (migration 73
// rebuilds 31 tables in pure-Go SQLite) and copying milliseconds. Kindred Nights'
// dbtest is the model.
package storetest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"app-suite/internal/store"
)

var migrated = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "senpan-store-")
	if err != nil {
		return nil, err
	}
	built, err := migrate(dir)
	return built, errors.Join(err, os.RemoveAll(dir))
})

func migrate(dir string) ([]byte, error) {
	ctx := context.Background()
	st, err := store.New(ctx, filepath.Join(dir, "build.sqlite"))
	if err != nil {
		return nil, err
	}
	built := filepath.Join(dir, "migrated.sqlite")
	_, err = st.DB().ExecContext(ctx, "VACUUM INTO ?", built)
	if err := errors.Join(err, st.Close()); err != nil {
		return nil, err
	}
	return os.ReadFile(built) //nolint:gosec // the file VACUUM INTO wrote, in the folder made for it
}

// Path is a migrated database file in the test's own directory, for code that
// opens its database itself (store.New finds nothing left to migrate).
func Path(t testing.TB) string {
	t.Helper()
	built, err := migrated()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, built, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// Open is a migrated database opened, and closed when the test ends.
func Open(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.New(t.Context(), Path(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Error(err)
		}
	})
	return st
}
