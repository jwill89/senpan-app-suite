package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// The package's own tests can't import storetest (it imports this package), so
// they keep the same helper: the migrations run once for the test binary, and each
// test gets a copy of the result.

var migratedForTest = sync.OnceValues(func() ([]byte, error) {
	dir, err := os.MkdirTemp("", "senpan-store-")
	if err != nil {
		return nil, err
	}
	built, err := func() ([]byte, error) {
		ctx := context.Background()
		st, err := New(ctx, filepath.Join(dir, "build.sqlite"))
		if err != nil {
			return nil, err
		}
		built := filepath.Join(dir, "migrated.sqlite")
		_, err = st.write.ExecContext(ctx, "VACUUM INTO ?", built)
		if err := errors.Join(err, st.Close()); err != nil {
			return nil, err
		}
		return os.ReadFile(built) //nolint:gosec // the file VACUUM INTO wrote
	}()
	return built, errors.Join(err, os.RemoveAll(dir))
})

// migratedPath is a migrated database file in the test's own directory.
func migratedPath(t testing.TB) string {
	t.Helper()
	built, err := migratedForTest()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "test.db")
	if err := os.WriteFile(path, built, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
