package store_test

import "testing"

func TestUserToken_UpsertGetReplaceDelete(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("tokuser", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	// CreateUser starts an account inactive; token auth resolves active accounts
	// only (see TestUserToken_InactiveAccountDoesNotResolve), so activate it here.
	if err := s.SetUserActive(u.ID, true); err != nil {
		t.Fatal(err)
	}

	// No token initially.
	info, err := s.GetUserTokenInfo(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.HasToken {
		t.Fatal("a new user should have no token")
	}

	// Upsert a token, then read its (non-secret) metadata back.
	if err := s.UpsertUserToken(u.ID, "hash-aaa", "pat_aaaaaa"); err != nil {
		t.Fatal(err)
	}
	info, err = s.GetUserTokenInfo(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !info.HasToken || info.Prefix != "pat_aaaaaa" {
		t.Fatalf("token info = %+v; want HasToken with prefix pat_aaaaaa", info)
	}
	if info.CreatedAt == "" {
		t.Fatal("created_at should be set")
	}
	if info.LastUsedAt != "" {
		t.Fatalf("last_used_at should be empty before first use, got %q", info.LastUsedAt)
	}

	// The hash resolves the owning account.
	got, err := s.GetUserByTokenHash("hash-aaa")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != u.ID {
		t.Fatalf("GetUserByTokenHash = %+v; want user %d", got, u.ID)
	}

	// Regenerate (replace): the old hash stops resolving, the new one works, and
	// the account still has exactly one token.
	if err := s.UpsertUserToken(u.ID, "hash-bbb", "pat_bbbbbb"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetUserByTokenHash("hash-aaa"); got != nil {
		t.Fatal("the replaced token hash should no longer resolve")
	}
	if got, _ := s.GetUserByTokenHash("hash-bbb"); got == nil {
		t.Fatal("the new token hash should resolve")
	}
	if info, _ := s.GetUserTokenInfo(u.ID); info.Prefix != "pat_bbbbbb" {
		t.Fatalf("prefix after regenerate = %q; want pat_bbbbbb", info.Prefix)
	}

	// Revoke.
	deleted, err := s.DeleteUserToken(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted {
		t.Fatal("expected DeleteUserToken to remove a row")
	}
	if got, _ := s.GetUserByTokenHash("hash-bbb"); got != nil {
		t.Fatal("a revoked token must not resolve")
	}
	if info, _ := s.GetUserTokenInfo(u.ID); info.HasToken {
		t.Fatal("info should report no token after revoke")
	}
}

func TestUserToken_InactiveAccountDoesNotResolve(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("inactiveuser", "hash-1") // CreateUser starts inactive
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserToken(u.ID, "hash-xyz", "pat_xyz"); err != nil {
		t.Fatal(err)
	}

	// An inactive account's token authenticates no one.
	if got, _ := s.GetUserByTokenHash("hash-xyz"); got != nil {
		t.Fatal("a token for an inactive account must not resolve")
	}

	// Once activated, the same token resolves.
	if err := s.SetUserActive(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetUserByTokenHash("hash-xyz"); got == nil {
		t.Fatal("the token should resolve once the account is active")
	}
}

func TestUserToken_TouchSetsLastUsed(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("touchuser", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserToken(u.ID, "hash-touch", "pat_touch"); err != nil {
		t.Fatal(err)
	}

	if err := s.TouchUserToken("hash-touch"); err != nil {
		t.Fatal(err)
	}
	info, err := s.GetUserTokenInfo(u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if info.LastUsedAt == "" {
		t.Fatal("last_used_at should be set after TouchUserToken")
	}
}

func TestUserToken_CascadesOnUserDelete(t *testing.T) {
	s := newTestStore(t)
	u, err := s.CreateUser("cascadeuser", "hash-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetUserActive(u.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertUserToken(u.ID, "hash-cascade", "pat_cascade"); err != nil {
		t.Fatal(err)
	}
	// Sanity: the token resolves while the account exists...
	if got, _ := s.GetUserByTokenHash("hash-cascade"); got == nil {
		t.Fatal("token should resolve before the account is deleted")
	}
	if _, err := s.DeleteUser(u.ID); err != nil {
		t.Fatal(err)
	}
	// ...and the user_tokens FK is ON DELETE CASCADE, so deleting the account drops it.
	if got, _ := s.GetUserByTokenHash("hash-cascade"); got != nil {
		t.Fatal("deleting a user should cascade-delete its token")
	}
}
