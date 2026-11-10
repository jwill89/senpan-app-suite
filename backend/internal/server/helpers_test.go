package server_test

import (
	"testing"

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
		"/api/v1/garapons":                              "garapons",
		"/api/v1/garapons/5/players":                    "garapons",
		"/api/v1/stamp-rallies":                         "stamp-rallies",
		"/api/v1/stamp-rallies/5/cards/2":               "stamp-rallies",
		"/api/v1/festival-maps":                         "festival-maps",
		"/api/v1/festival-maps/5":                       "festival-maps",
		"/api/v1/ema-boards/4/plaques/9/approve":        "ema-boards",
		"/api/v1/ema-boards/4/close":                    "ema-boards",
		"/api/v1/ema-requests/3/post":                   "ema-boards",
		"/api/v1/ema-requests/3/complete":               "ema-boards",
		"/api/v1/raffles":                               "raffles",
		"/api/v1/raffles/9/entries":                     "raffles",
		"/api/v1/raffles/9/entries/4/settle":            "raffles",
		"/api/v1/presets/4":                             "presets",
		"/api/v1/affiliates/2/post":                     "affiliates",
		"/api/v1/affiliates/reorder":                    "affiliates",
		"/api/v1/tea-rooms":                             "tea-rooms",
		"/api/v1/tea-rooms/3/post":                      "tea-rooms",
		"/api/v1/tea-rooms/reorder":                     "tea-rooms",
		"/api/users":                                    "users",
		"/api/v1/users/7":                               "users",
		"/api/v1/announcements":                         "announcements",
		"/api/v1/announcements/4/send":                  "announcements",
		"/api/v1/announcement-types":                    "announcements",
		"/api/v1/announcement-roles/2":                  "announcements",
		"/api/v1/book-clubs/yaoi/reading-lists":         "bookclub-yaoi",
		"/api/v1/book-clubs/yuri/reading-lists/3/items": "bookclub-yuri",
		"/api/v1/book-clubs/yuri/webhook":               "bookclub-yuri",
		"/api/v1/winners-log/all":                       "winners-log",
		"/api/v1/fonts":                                 "fonts",
		"/api/v1/fonts/jasper":                          "fonts",
		"/api/v1/font-files/jasper.ttf":                 "fonts",
		"/api/v1/carrd-projects":                        "carrd",
		"/api/v1/carrd-projects/obon/files":             "carrd",
		"/api/v1/image-categories":                      "images",
		"/api/v1/image-categories/raffles/images":       "images",
		"/api/v1/image-categories/raffles/images/a.png": "images",
	}
	for path, want := range mapped {
		got, ok := server.AdminMutationResourceForTest(path)
		if !ok || got != want {
			t.Errorf("adminMutationResource(%q) = (%q, %v); want (%q, true)", path, got, ok, want)
		}
	}

	excluded := []string{
		"/api/v1/garapon-players/abc123/draw", // public draw - its operation tells garapons
		"/api/v1/garapon-players/abc123",      // public token view
		"/api/v1/stamp-cards/abc123/stamp",    // public stamp - its operation tells stamp-rallies
		"/api/v1/stamp-cards/lookup",          // public lookup - changes nothing
		"/api/v1/stamp-rallies/5/sign-up",     // public sign-up - its operation tells its pages
		"/api/v1/festivals/7/dashboard",       // a public read, though a POST
		"/api/v1/festivals/7/ema-plaques",     // public - its operation tells ema-boards
		"/api/v1/festivals/7/ema-requests",    // public - its operation tells ema-boards
		"/api/v1/raffles/9/enter",             // public sign-up - its operation tells raffles
		"/api/v1/raffles/9/lookup",            // public search - changes nothing
		"/api/v1/auth/login",                  // signing in
		"/api/v1/me/token",                    // self-service
		"/api/v1/game/end",                    // the game's own events
		"/api/v1/cards/all",                   // cards_changed
		"/api/styles",                         // theme_changed
		"/api/v1/styles/3",                    // theme_changed
		"/api/v1/settings",                    // settings_changed
		"/api/v1/patterns/2/move",             // patterns_changed
		"/api/v1/pattern-categories",
		"/api/v1/book-clubs",
		"/api/v1/book-clubs/yaoi/covers", // an upload changes no reading list

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
