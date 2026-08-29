package store

import "fmt"

// Test-only accessors for PRAGMA user_version, so a test can build a database at a
// chosen schema version and observe whether merely opening it changed that
// version. Production reads user_version through ensureSchema and never needs to
// write it directly.

func (s *Store) SetUserVersionForTest(v int) error {
	_, err := s.db.Exec(fmt.Sprintf("PRAGMA user_version = %d", v))
	return err
}

func (s *Store) UserVersionForTest() (int, error) {
	var v int
	err := s.db.QueryRow("PRAGMA user_version").Scan(&v)
	return v, err
}
