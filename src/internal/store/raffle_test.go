package store_test

import (
	"testing"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// fmtUTC formats an instant the way the frontend's `Date.toISOString()` does
// (UTC, millisecond precision, trailing 'Z') so the test exercises the exact
// stored format produced by the admin form.
func fmtUTC(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

func mustCreateRaffle(t *testing.T, s *store.Store, title, from, to string) int64 {
	t.Helper()
	id, err := s.CreateRaffle(&model.Raffle{
		Title:         title,
		MaxEntries:    1,
		AvailableFrom: from,
		AvailableTo:   to,
	})
	if err != nil {
		t.Fatalf("CreateRaffle(%s): %v", title, err)
	}
	return id
}

// TestListRafflesAvailabilityWindow verifies the public list honours the
// availability window using UTC instants — a raffle past its "available to"
// time must not appear (the timezone bug), while in-window and unbounded
// raffles do. It also confirms SQLite's datetime() normalizes the UTC ISO 'Z'
// format the frontend now stores.
func TestListRafflesAvailabilityWindow(t *testing.T) {
	s := newTestStore(t)
	now := time.Now().UTC()
	hourAgo := fmtUTC(now.Add(-time.Hour))
	hourAhead := fmtUTC(now.Add(time.Hour))

	pastID := mustCreateRaffle(t, s, "past", "", hourAgo)                  // ended an hour ago
	openID := mustCreateRaffle(t, s, "open", hourAgo, hourAhead)           // currently open
	futureID := mustCreateRaffle(t, s, "future", hourAhead, "")            // not yet open
	alwaysID := mustCreateRaffle(t, s, "always", "", "")                   // no window
	legacyPast := mustCreateRaffle(t, s, "legacy", "", "2000-01-01T00:00") // legacy naive, long past

	raffles, err := s.ListRaffles(false)
	if err != nil {
		t.Fatalf("ListRaffles(false): %v", err)
	}

	visible := map[int64]bool{}
	for _, r := range raffles {
		visible[r.ID] = true
	}

	if visible[pastID] {
		t.Error("raffle past its available_to should NOT be visible to the public")
	}
	if visible[futureID] {
		t.Error("raffle before its available_from should NOT be visible to the public")
	}
	if visible[legacyPast] {
		t.Error("legacy raffle long past its available_to should NOT be visible")
	}
	if !visible[openID] {
		t.Error("in-window raffle should be visible to the public")
	}
	if !visible[alwaysID] {
		t.Error("raffle with no availability window should be visible to the public")
	}

	// Admin sees everything regardless of the window.
	all, err := s.ListRaffles(true)
	if err != nil {
		t.Fatalf("ListRaffles(true): %v", err)
	}
	if len(all) != 5 {
		t.Errorf("admin ListRaffles = %d raffles; want 5", len(all))
	}
}
