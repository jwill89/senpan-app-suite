package store_test

import (
	"testing"

	"app-suite/internal/auth"
	"app-suite/internal/db"
)

// The migrations seed the bootstrap admin: active, an admin, password "admin" (to be
// changed at once).
func TestUsersSeedAdmin(t *testing.T) {
	s := newTestStore(t)
	q := db.New(s.ReadDB())

	admin, err := q.GetUserByUsername(t.Context(), auth.ReservedUsername)
	if err != nil {
		t.Fatalf("expected the migrations to seed the %q account: %v", auth.ReservedUsername, err)
	}
	if admin.IsAdmin != 1 || admin.IsActive != 1 {
		t.Fatalf("the seeded admin should be an active admin, got admin=%d active=%d", admin.IsAdmin, admin.IsActive)
	}
	hash, err := q.GetPasswordHash(t.Context(), admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := auth.Verify("admin", hash); err != nil || !ok {
		t.Fatalf("the seeded admin's password should be %q (ok=%v, err=%v)", "admin", ok, err)
	}
}
