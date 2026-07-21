package store_test

import (
	"errors"
	"testing"

	"app-suite/internal/store"
)

func TestUsersSeedAdmin(t *testing.T) {
	s := newTestStore(t)

	admin, hash, err := s.GetUserByUsername("admin")
	if err != nil {
		t.Fatal(err)
	}
	if admin == nil {
		t.Fatal("expected migration to seed an 'admin' user")
	}
	if !admin.IsAdmin || !admin.IsActive {
		t.Fatalf("seeded admin should be admin+active, got admin=%v active=%v", admin.IsAdmin, admin.IsActive)
	}
	if hash == "" {
		t.Fatal("seeded admin should have a password hash")
	}
}

func TestUsersPasswordEpochBumps(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("epoch-user", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if u.PasswordEpoch != 0 {
		t.Fatalf("a new user should start at epoch 0, got %d", u.PasswordEpoch)
	}

	// Each password change must advance the epoch so prior sessions go stale.
	for want := int64(1); want <= 2; want++ {
		if err := s.SetUserPassword(u.ID, "hash-new"); err != nil {
			t.Fatal(err)
		}
		got, err := s.GetUserByID(u.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.PasswordEpoch != want {
			t.Fatalf("after %d password change(s), epoch = %d, want %d", want, got.PasswordEpoch, want)
		}
	}

	// The epoch is also readable via the login path (GetUserByUsername).
	byName, _, err := s.GetUserByUsername("epoch-user")
	if err != nil {
		t.Fatal(err)
	}
	if byName.PasswordEpoch != 2 {
		t.Fatalf("GetUserByUsername epoch = %d, want 2", byName.PasswordEpoch)
	}
}

func TestUsersUpdateLastLogin(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("tester", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if u.LastLoginAt != "" {
		t.Fatalf("a new user should have no last-login time, got %q", u.LastLoginAt)
	}

	if err := s.UpdateLastLogin(u.ID); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetUserByID(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.LastLoginAt == "" {
		t.Fatalf("last_login_at should be set after UpdateLastLogin, got %q", got.LastLoginAt)
	}
}

func TestUsersCreateInactiveAndMutate(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("tester", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if u.IsActive || u.IsAdmin {
		t.Fatal("new users must start inactive and non-admin")
	}
	if len(u.Permissions) != 0 {
		t.Fatalf("new users start with no permissions, got %v", u.Permissions)
	}

	if err := s.SetUserActive(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPermissions(u.ID, []string{"bingo-game", "bingo-cards"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserPassword(u.ID, "hash-2"); err != nil {
		t.Fatal(err)
	}

	got, hash, err := s.GetUserByUsername("tester")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsActive {
		t.Fatal("expected user to be active after SetUserActive")
	}
	if hash != "hash-2" {
		t.Fatalf("expected updated hash, got %q", hash)
	}
	if len(got.Permissions) != 2 || got.Permissions[0] != "bingo-game" {
		t.Fatalf("unexpected permissions: %v", got.Permissions)
	}
}

func TestUsersDuplicateUsername(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.CreateUser("dup", "h"); err != nil {
		t.Fatal(err)
	}
	_, err := s.CreateUser("dup", "h2")
	if !errors.Is(err, store.ErrUsernameTaken) {
		t.Fatalf("expected ErrUsernameTaken, got %v", err)
	}
}

func TestUsersDelete(t *testing.T) {
	s := newTestStore(t)

	u, err := s.CreateUser("temp", "h")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := s.DeleteUser(u.ID)
	if err != nil || !deleted {
		t.Fatalf("expected delete to succeed, got deleted=%v err=%v", deleted, err)
	}
	got, _, err := s.GetUserByUsername("temp")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatal("expected user to be gone after delete")
	}
}
