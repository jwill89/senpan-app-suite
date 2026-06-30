package store_test

import (
	"errors"
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

// TestAddOrCreateRaffleEntry verifies the atomic cap-enforced entry write: it
// creates a new row, folds a repeat sign-up (matched case-insensitively) into the
// same row, and refuses an over-cap add without mutating anything.
func TestAddOrCreateRaffleEntry(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Cap", MaxEntries: 5})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}

	// First sign-up creates the row.
	entryID, total, prev, created, err := s.AddOrCreateRaffleEntry(id, "Cloud", "Gaia", 2, 5)
	if err != nil {
		t.Fatalf("AddOrCreateRaffleEntry (create): %v", err)
	}
	if !created || prev != 0 || total != 2 || entryID == 0 {
		t.Fatalf("create: got entryID=%d total=%d prev=%d created=%v; want id>0 total=2 prev=0 created=true", entryID, total, prev, created)
	}

	// Repeat sign-up (different case) folds into the same row.
	gotID, total, prev, created, err := s.AddOrCreateRaffleEntry(id, "cloud", "GAIA", 1, 5)
	if err != nil {
		t.Fatalf("AddOrCreateRaffleEntry (add): %v", err)
	}
	if created || gotID != entryID || prev != 2 || total != 3 {
		t.Fatalf("add: got entryID=%d total=%d prev=%d created=%v; want id=%d total=3 prev=2 created=false", gotID, total, prev, created, entryID)
	}

	// Over-cap add is rejected and leaves the row unchanged.
	if _, _, prev, _, err := s.AddOrCreateRaffleEntry(id, "Cloud", "Gaia", 3, 5); !errors.Is(err, store.ErrRaffleEntryLimit) {
		t.Fatalf("over-cap add: err=%v (prev=%d); want ErrRaffleEntryLimit", err, prev)
	}
	if e, _ := s.GetRaffleEntry(id, "Cloud", "Gaia"); e == nil || e.NumEntries != 3 {
		t.Fatalf("over-cap add must not mutate: entry=%+v; want NumEntries=3", e)
	}
}

// TestListRafflesAdminAggregates verifies the admin list carries the closed-table
// aggregates: the winner entry's "Character @ World" and the gil collected from
// paid tickets only (sum of paid num_entries × cost_per_entry).
func TestListRafflesAdminAggregates(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Prize", MaxEntries: 10, CostPerEntry: 100})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	// One paid entry (3 tickets) → counts; one unpaid (2 tickets) → excluded.
	paidID, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)
	if err != nil {
		t.Fatalf("CreateRaffleEntry (paid): %v", err)
	}
	if _, err := s.CreateRaffleEntry(id, "Borin", "Hades", 2); err != nil {
		t.Fatalf("CreateRaffleEntry (unpaid): %v", err)
	}
	if err := s.SetRaffleEntryPaid(paidID, true); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	if err := s.SetRaffleWinner(id, &paidID); err != nil {
		t.Fatalf("SetRaffleWinner: %v", err)
	}
	if err := s.SetRaffleStatus(id, "closed"); err != nil {
		t.Fatalf("SetRaffleStatus: %v", err)
	}

	all, err := s.ListRaffles(true)
	if err != nil {
		t.Fatalf("ListRaffles(true): %v", err)
	}
	var got *model.Raffle
	for i := range all {
		if all[i].ID == id {
			got = &all[i]
		}
	}
	if got == nil {
		t.Fatal("created raffle not found in admin list")
	}
	if got.WinnerName != "Aria @ Gilgamesh" {
		t.Errorf("winner_name = %q; want %q", got.WinnerName, "Aria @ Gilgamesh")
	}
	if got.PaidTotal != 300 { // 3 paid tickets × 100 gil
		t.Errorf("paid_total = %v; want 300", got.PaidTotal)
	}

	// The public list omits both aggregates.
	pub, err := s.ListRaffles(false)
	if err != nil {
		t.Fatalf("ListRaffles(false): %v", err)
	}
	for _, r := range pub {
		if r.WinnerName != "" || r.PaidTotal != 0 {
			t.Errorf("public list leaked aggregates: %+v", r)
		}
	}
}
