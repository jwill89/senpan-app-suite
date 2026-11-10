package server_test

import (
	"testing"

	"app-suite/internal/model"
	"app-suite/internal/server"
)

// -- adminMutationResource (the resource_changed mapping) ---------------------

// TestAdminMutationResource is the unit-level guard for resource_changed: admin
// CRUD writes map to a resource key (a book club's to that club's), while public,
// sign-in, self-service, and the paths with events of their own must NOT (they
// return ok=false). The public ".../enter" and ".../draw" exclusions are why the
// raffle-enter and garapon-draw handlers tell their page themselves.
func TestAdminMutationResource(t *testing.T) {
	mapped := map[string]string{
		"/api/garapons":                              "garapons",
		"/api/garapons/5/players":                    "garapons",
		"/api/raffles":                               "raffles",
		"/api/raffles/9/entries":                     "raffles",
		"/api/v1/presets/4":                          "presets",
		"/api/users":                                 "users",
		"/api/v1/users/7":                            "users",
		"/api/announcements":                         "announcements",
		"/api/announcement-types":                    "announcements",
		"/api/announcement-roles":                    "announcements",
		"/api/book-clubs/yaoi/reading-lists":         "bookclub-yaoi",
		"/api/book-clubs/yuri/reading-lists/3/items": "bookclub-yuri",
		"/api/v1/winners-log/all":                    "winners-log",
		"/api/fonts":                                 "fonts",
		"/api/fonts/upload":                          "fonts",
		"/api/carrd/projects":                        "carrd",
		"/api/images":                                "images",
		"/api/images/upload":                         "images",
		"/api/image-categories":                      "images",
	}
	for path, want := range mapped {
		got, ok := server.AdminMutationResourceForTest(path)
		if !ok || got != want {
			t.Errorf("adminMutationResource(%q) = (%q, %v); want (%q, true)", path, got, ok, want)
		}
	}

	excluded := []string{
		"/api/garapon/abc123/draw", // public draw - its handler tells garapons
		"/api/raffles/9/enter",     // public sign-up - its handler tells raffles
		"/api/garapon/abc123",      // public token view
		"/api/v1/auth/login",       // signing in
		"/api/v1/me/token",         // self-service
		"/api/v1/game/end",         // the game's own events
		"/api/v1/cards/all",        // cards_changed
		"/api/styles",              // theme_changed
		"/api/v1/styles/3",         // theme_changed
		"/api/v1/settings",         // settings_changed
		"/api/v1/patterns/2/move",  // patterns_changed
		"/api/v1/pattern-categories",
		"/api/book-clubs",
		"/api/unknown",
	}
	for _, path := range excluded {
		if got, ok := server.AdminMutationResourceForTest(path); ok {
			t.Errorf("adminMutationResource(%q) = (%q, true); want ok=false", path, got)
		}
	}
}

// TestAccountMutation names the writes after which every staff stream checks its
// account again.
func TestAccountMutation(t *testing.T) {
	for path, want := range map[string]bool{
		"/api/v1/users/7":       true,
		"/api/v1/me/password":   true,
		"/api/v1/me/token":      true,
		"/api/v1/auth/logout":   true,
		"/api/v1/auth/login":    false,
		"/api/v1/auth/register": false,
		"/api/v1/cards":         false,
	} {
		if got := server.AccountMutationForTest(path); got != want {
			t.Errorf("accountMutation(%q) = %v; want %v", path, got, want)
		}
	}
}

// -- safeFontName -------------------------------------------------------------

func TestSafeFontName(t *testing.T) {
	valid := map[string]string{
		"Roboto.ttf":      "Roboto.ttf",
		"  spaced.otf  ":  "spaced.otf",
		"font.WOFF2":      "font.WOFF2",  // extension check is case-insensitive
		"a/b/nested.woff": "nested.woff", // directory components stripped to base
	}
	for in, want := range valid {
		got, ok := server.SafeFontNameForTest(in)
		if !ok || got != want {
			t.Errorf("safeFontName(%q) = (%q, %v); want (%q, true)", in, got, ok, want)
		}
	}

	invalid := []string{
		"",             // empty
		"   ",          // blank
		"..",           // dotdot
		"noext",        // no extension
		"evil.exe",     // disallowed extension
		"shell.ttf.sh", // disallowed final extension
	}
	for _, in := range invalid {
		if got, ok := server.SafeFontNameForTest(in); ok {
			t.Errorf("safeFontName(%q) = (%q, true); want ok=false", in, got)
		}
	}
}

// -- sanitizeGaraponPrizes ----------------------------------------------------

func TestSanitizeGaraponPrizes(t *testing.T) {
	t.Run("drops blanks, defaults color, floors negative rate", func(t *testing.T) {
		out, msg := server.SanitizeGaraponPrizesForTest([]model.GaraponPrize{
			{Name: "  ", Rate: 5},                    // blank -> dropped
			{Name: "Grand", Rate: -3, IsGrand: true}, // negative rate -> 0
			{Name: "Other", BallColor: "", Rate: 2},  // empty color -> default
		})
		if msg != "" {
			t.Fatalf("unexpected error msg: %q", msg)
		}
		if len(out) != 2 {
			t.Fatalf("len(out) = %d; want 2", len(out))
		}
		if out[0].Rate != 0 {
			t.Errorf("negative rate not floored: %v", out[0].Rate)
		}
		if out[1].BallColor == "" {
			t.Error("empty ball color was not defaulted")
		}
	})

	t.Run("auto-promotes the first prize when none is grand", func(t *testing.T) {
		out, msg := server.SanitizeGaraponPrizesForTest([]model.GaraponPrize{
			{Name: "A", Rate: 1}, {Name: "B", Rate: 1},
		})
		if msg != "" {
			t.Fatalf("unexpected msg: %q", msg)
		}
		if !out[0].IsGrand {
			t.Error("first prize should be auto-promoted to grand")
		}
	})

	t.Run("rejects empty and multi-grand", func(t *testing.T) {
		if _, msg := server.SanitizeGaraponPrizesForTest(nil); msg == "" {
			t.Error("expected error for no usable prizes")
		}
		if _, msg := server.SanitizeGaraponPrizesForTest([]model.GaraponPrize{
			{Name: "A", IsGrand: true}, {Name: "B", IsGrand: true},
		}); msg == "" {
			t.Error("expected error for multiple grand prizes")
		}
	})
}

// -- isDiscordSnowflake -------------------------------------------------------

func TestIsDiscordSnowflake(t *testing.T) {
	if !server.IsDiscordSnowflakeForTest("123456789012345678") {
		t.Error("all-digit id should be a snowflake")
	}
	for _, bad := range []string{"", "12a45", " 123", "12.3", "abc"} {
		if server.IsDiscordSnowflakeForTest(bad) {
			t.Errorf("%q should not be a snowflake", bad)
		}
	}
}

// -- parseRaffleTime ----------------------------------------------------------

func TestParseRaffleTime(t *testing.T) {
	if _, ok := server.ParseRaffleTimeForTest(""); ok {
		t.Error("empty string should report ok=false (no constraint)")
	}
	if _, ok := server.ParseRaffleTimeForTest("not-a-date"); ok {
		t.Error("garbage should report ok=false")
	}
	rfc, ok := server.ParseRaffleTimeForTest("2026-06-13T20:00:00.000Z")
	if !ok || rfc.Location().String() != "UTC" {
		t.Errorf("RFC-3339 parse failed: ok=%v loc=%v", ok, rfc.Location())
	}
	if legacy, ok := server.ParseRaffleTimeForTest("2026-06-13T20:00"); !ok || legacy.Location().String() != "UTC" {
		t.Errorf("legacy naive parse failed: ok=%v loc=%v", ok, legacy.Location())
	}
}
