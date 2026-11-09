package store

import (
	"context"
	"fmt"
)

// Test-only accessors for PRAGMA user_version, so a test can build a database at a
// chosen schema version and observe whether merely opening it changed that
// version. Production reads user_version through migrate and never needs to
// write it directly.

func (s *Store) SetUserVersionForTest(ctx context.Context, v int) error {
	_, err := s.write.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

func (s *Store) UserVersionForTest(ctx context.Context) (int, error) {
	var v int
	err := s.read.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v)
	return v, err
}
