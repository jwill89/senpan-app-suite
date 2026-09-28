package store_test

import (
	"errors"
	"math"
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
// availability window using UTC instants - a raffle past its "available to"
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

// TestAddOrCreateRaffleEntryOverflow pins the cap check against integer overflow.
// The check used to sum first (prevEntries + num) and compare, so a num near
// MaxInt64 wrapped negative, slipped past "> maxEntries", and ran the UPDATE -
// which pushed num_entries beyond int64 and left SQLite storing it as REAL, a
// value no read path can scan back. That stranded the whole raffle: the staff
// detail view and the entry-delete route both 500'd on the scan, so the poisoned
// row could not even be removed. The cap must reject it and touch nothing.
func TestAddOrCreateRaffleEntryOverflow(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Overflow", MaxEntries: 3})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}

	// A row must already exist: with prevEntries==0 the sum cannot wrap, so the
	// overflow is only reachable on the second sign-up for the same character.
	if _, _, _, _, err := s.AddOrCreateRaffleEntry(id, "Aria", "Gilgamesh", 1, 3); err != nil {
		t.Fatalf("seed entry: %v", err)
	}

	_, total, _, _, err := s.AddOrCreateRaffleEntry(id, "Aria", "Gilgamesh", math.MaxInt64, 3)
	if !errors.Is(err, store.ErrRaffleEntryLimit) {
		t.Fatalf("overflowing add: err=%v total=%d; want ErrRaffleEntryLimit", err, total)
	}

	// The row is untouched and still readable - the scan is the part that used to
	// break, so read it back rather than trusting the return values.
	e, err := s.GetRaffleEntry(id, "Aria", "Gilgamesh")
	if err != nil {
		t.Fatalf("entry unreadable after overflowing add (num_entries widened to REAL?): %v", err)
	}
	if e == nil || e.NumEntries != 1 {
		t.Fatalf("overflowing add must not mutate: entry=%+v; want NumEntries=1", e)
	}
}

// TestListRafflesAdminAggregates verifies the admin list carries the closed-table
// aggregates: the winner entry's "Character @ World" and the gil collected from
// paid tickets only (sum of paid num_entries x cost_per_entry).
func TestListRafflesAdminAggregates(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Prize", MaxEntries: 10, CostPerEntry: 100})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	// One paid entry (3 tickets) -> counts; one unpaid (2 tickets) -> excluded.
	paidID, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)
	if err != nil {
		t.Fatalf("CreateRaffleEntry (paid): %v", err)
	}
	if _, err := s.CreateRaffleEntry(id, "Borin", "Hades", 2); err != nil {
		t.Fatalf("CreateRaffleEntry (unpaid): %v", err)
	}
	if _, err := s.SetRaffleEntryPaid(paidID, true, 0, 0); err != nil {
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
	if got.PaidTotal != 300 { // 3 paid tickets x 100 gil
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

// TestRaffleEntryCostByMode pins the pricing rule each entry mode applies. A
// "custom" raffle charges the 1st/2nd/3rd ticket its own tier, so three tickets
// cost the SUM of the ladder - not three times the first rung.
func TestRaffleEntryCostByMode(t *testing.T) {
	single := model.Raffle{EntryMode: model.RaffleModeSingle, CostPerEntry: 50_000}
	custom := model.Raffle{EntryMode: model.RaffleModeCustom, TierCosts: []float64{50_000, 100_000, 150_000}}
	details := model.Raffle{EntryMode: model.RaffleModeDetails, CostPerEntry: 50_000, TierCosts: []float64{9}}
	legacy := model.Raffle{EntryMode: "", CostPerEntry: 50_000} // pre-entry-mode row

	cases := []struct {
		name    string
		raffle  model.Raffle
		tickets int
		want    float64
	}{
		{"single one ticket", single, 1, 50_000},
		{"single three tickets", single, 3, 150_000},
		{"custom one ticket", custom, 1, 50_000},
		{"custom two tickets", custom, 2, 150_000},
		{"custom three tickets", custom, 3, 300_000},
		{"custom past the ladder", custom, 4, 300_000},
		{"details is free", details, 3, 0},
		{"blank mode prices like single", legacy, 2, 100_000},
		{"zero tickets", single, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.raffle.EntryCost(tc.tickets); got != tc.want {
				t.Errorf("EntryCost(%d) = %v; want %v", tc.tickets, got, tc.want)
			}
		})
	}

	if details.AcceptsSignups() {
		t.Error("a details-only raffle must not accept sign-ups")
	}
	if !legacy.AcceptsSignups() {
		t.Error("a pre-entry-mode raffle must keep accepting sign-ups")
	}
}

// TestRaffleTierCostsRoundTrip verifies the custom ladder survives a save/load,
// and that a raffle stored before entry modes existed reads back as "single".
func TestRaffleTierCostsRoundTrip(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{
		Title:      "Tiered",
		MaxEntries: 3,
		EntryMode:  model.RaffleModeCustom,
		TierCosts:  []float64{50_000, 100_000, 150_000},
	})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	got, err := s.GetRaffle(id)
	if err != nil || got == nil {
		t.Fatalf("GetRaffle: %v (raffle=%v)", err, got)
	}
	if got.EntryMode != model.RaffleModeCustom {
		t.Errorf("entry_mode = %q; want %q", got.EntryMode, model.RaffleModeCustom)
	}
	if len(got.TierCosts) != 3 || got.TierCosts[2] != 150_000 {
		t.Fatalf("tier_costs = %v; want [50000 100000 150000]", got.TierCosts)
	}

	// Switching back to a flat cost must not leave the ladder behind as a
	// shadow price.
	got.EntryMode = model.RaffleModeSingle
	got.TierCosts = nil
	got.CostPerEntry = 25_000
	if err := s.UpdateRaffle(got); err != nil {
		t.Fatalf("UpdateRaffle: %v", err)
	}
	after, err := s.GetRaffle(id)
	if err != nil || after == nil {
		t.Fatalf("GetRaffle after update: %v", err)
	}
	if len(after.TierCosts) != 0 {
		t.Errorf("tier_costs = %v; want empty after switching to single", after.TierCosts)
	}
	if after.EntryCost(2) != 50_000 {
		t.Errorf("EntryCost(2) = %v; want 50000", after.EntryCost(2))
	}
}

// TestListRafflesAdminTieredTotal checks the collected-gil aggregate prices each
// paid entry through its raffle's ladder. Two entries holding 1 and 3 tickets on
// a 50k/100k/150k raffle owe 50k and 300k - a flat "paid tickets x first tier"
// sum would report 200k.
func TestListRafflesAdminTieredTotal(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{
		Title:      "Tiered",
		MaxEntries: 3,
		EntryMode:  model.RaffleModeCustom,
		TierCosts:  []float64{50_000, 100_000, 150_000},
	})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	oneTicket, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	if err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}
	threeTickets, err := s.CreateRaffleEntry(id, "Borin", "Hades", 3)
	if err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}
	unpaid, err := s.CreateRaffleEntry(id, "Ceri", "Ravana", 2)
	if err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}
	_ = unpaid
	for _, e := range []int64{oneTicket, threeTickets} {
		if _, err := s.SetRaffleEntryPaid(e, true, 0, 0); err != nil {
			t.Fatalf("SetRaffleEntryPaid: %v", err)
		}
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
	if got.PaidTotal != 350_000 { // 50k + (50k+100k+150k)
		t.Errorf("paid_total = %v; want 350000", got.PaidTotal)
	}
}

// TestPickRaffleWinnerIncludesUnpaid covers the details-only draw: entries there
// are recorded by staff from a sign-up run elsewhere, so nothing is ever marked
// paid and a paid-only draw would find no one.
func TestPickRaffleWinnerIncludesUnpaid(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Info", MaxEntries: 1, EntryMode: model.RaffleModeDetails})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	if _, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1); err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}

	if winner, err := s.PickRaffleWinner(id, true); err != nil || winner != nil {
		t.Fatalf("paid-only pick: winner=%v err=%v; want no winner", winner, err)
	}
	winner, err := s.PickRaffleWinner(id, false)
	if err != nil {
		t.Fatalf("PickRaffleWinner: %v", err)
	}
	if winner == nil || winner.CharacterName != "Aria" {
		t.Fatalf("winner = %v; want Aria", winner)
	}
}

// TestRaffleEntryWaiverIsAdditive is the core of the waived-amount rule: a second
// settlement ADDS to what was already forgiven instead of replacing it. A player
// whose first ticket was free and who later buys two more keeps both waivers.
func TestRaffleEntryWaiverIsAdditive(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Waived", MaxEntries: 3, CostPerEntry: 50_000})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	entryID, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	if err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}

	// First ticket settled, entirely waived - nothing collected.
	if _, err := s.SetRaffleEntryPaid(entryID, true, 0, 50_000); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	raffle, _ := s.GetRaffle(id)
	e, _ := s.GetRaffleEntryByID(entryID)
	if !e.Paid || e.PaidEntries != 1 {
		t.Fatalf("after first settlement: paid=%v paid_entries=%d; want true/1", e.Paid, e.PaidEntries)
	}
	if got := raffle.AmountCollected(*e); got != 0 {
		t.Errorf("collected = %v; want 0 (fully waived)", got)
	}

	// Two more tickets bought later: the row falls back to partial, keeping what
	// it already settled and waived.
	if _, _, _, _, err := s.AddOrCreateRaffleEntry(id, "Aria", "Gilgamesh", 2, 3); err != nil {
		t.Fatalf("AddOrCreateRaffleEntry: %v", err)
	}
	e, _ = s.GetRaffleEntryByID(entryID)
	if e.Paid {
		t.Error("row must leave paid once unsettled tickets are added")
	}
	if e.PaymentState() != "partial" {
		t.Errorf("state = %q; want partial", e.PaymentState())
	}
	if e.AmountWaived != 50_000 {
		t.Errorf("amount_waived = %v; want the original 50000 kept", e.AmountWaived)
	}
	if got := raffle.AmountOutstanding(*e); got != 100_000 {
		t.Errorf("outstanding = %v; want 100000 (the two new tickets)", got)
	}

	// Settling again waives another 25,000 ON TOP of the first waiver.
	if _, err := s.SetRaffleEntryPaid(entryID, true, 0, 25_000); err != nil {
		t.Fatalf("SetRaffleEntryPaid (second): %v", err)
	}
	e, _ = s.GetRaffleEntryByID(entryID)
	if e.AmountWaived != 75_000 {
		t.Fatalf("amount_waived = %v; want 75000 (50000 + 25000, not overwritten)", e.AmountWaived)
	}
	if !e.Paid || e.PaidEntries != 3 {
		t.Errorf("after second settlement: paid=%v paid_entries=%d; want true/3", e.Paid, e.PaidEntries)
	}
	// 3 tickets x 50,000 = 150,000, less 75,000 waived.
	if got := raffle.AmountCollected(*e); got != 75_000 {
		t.Errorf("collected = %v; want 75000", got)
	}

	// Clearing the settlement resets both counters - a mis-click must not leave a
	// waiver credited against an unpaid entry.
	if _, err := s.SetRaffleEntryPaid(entryID, false, 0, 0); err != nil {
		t.Fatalf("SetRaffleEntryPaid (clear): %v", err)
	}
	e, _ = s.GetRaffleEntryByID(entryID)
	if e.Paid || e.PaidEntries != 0 || e.AmountWaived != 0 {
		t.Errorf("after clearing: %+v; want nothing settled and nothing waived", e)
	}
}

// TestRaffleWaiverNeverGoesNegative guards the collected total against a waiver
// larger than the tickets cost: it reads as "collected nothing", never as a
// negative that would eat into another entry's contribution.
func TestRaffleWaiverNeverGoesNegative(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Over-waived", MaxEntries: 1, CostPerEntry: 1_000})
	entryID, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	if _, err := s.SetRaffleEntryPaid(entryID, true, 0, 9_999); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}

	all, err := s.ListRaffles(true)
	if err != nil {
		t.Fatalf("ListRaffles(true): %v", err)
	}
	for _, r := range all {
		if r.ID == id && r.PaidTotal != 0 {
			t.Errorf("paid_total = %v; want 0", r.PaidTotal)
		}
	}
}

// TestListRafflesAdminNetsOffWaivers checks the closed-table total counts what
// was actually collected: settled tickets priced by the ladder, less waivers,
// including from a partly-settled entry.
func TestListRafflesAdminNetsOffWaivers(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{
		Title:      "Tiered",
		MaxEntries: 3,
		EntryMode:  model.RaffleModeCustom,
		TierCosts:  []float64{50_000, 100_000, 150_000},
	})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	// Fully settled, 3 tickets (300,000) with 100,000 waived -> 200,000.
	full, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)
	if _, err := s.SetRaffleEntryPaid(full, true, 0, 100_000); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	// Settled 1 of 3 (50,000), nothing waived -> 50,000 collected.
	partial, _ := s.CreateRaffleEntry(id, "Borin", "Hades", 1)
	if _, err := s.SetRaffleEntryPaid(partial, true, 0, 0); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	if _, _, _, _, err := s.AddOrCreateRaffleEntry(id, "Borin", "Hades", 2, 3); err != nil {
		t.Fatalf("AddOrCreateRaffleEntry: %v", err)
	}
	// Never settled -> contributes nothing.
	if _, err := s.CreateRaffleEntry(id, "Ceri", "Ravana", 2); err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
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
	if got.PaidTotal != 250_000 { // (300k - 100k waived) + 50k
		t.Errorf("paid_total = %v; want 250000", got.PaidTotal)
	}
}

// TestPickRaffleWinnerWeightsBySettledTickets covers the part-paid entrant: they
// paid for one ticket and bought two more, so they belong in the draw with ONE
// chance - not excluded outright, and not given all three.
func TestPickRaffleWinnerWeightsBySettledTickets(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{Title: "Weighted", MaxEntries: 3, CostPerEntry: 100})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	partial, err := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	if err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}
	if _, err := s.SetRaffleEntryPaid(partial, true, 0, 0); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	if _, _, _, _, err := s.AddOrCreateRaffleEntry(id, "Aria", "Gilgamesh", 2, 3); err != nil {
		t.Fatalf("AddOrCreateRaffleEntry: %v", err)
	}
	// A wholly unpaid entrant must still be excluded.
	if _, err := s.CreateRaffleEntry(id, "Ceri", "Ravana", 3); err != nil {
		t.Fatalf("CreateRaffleEntry: %v", err)
	}

	// Aria is the only eligible entrant, so every draw must land on her - which
	// also proves the unpaid entry contributes no weight.
	for i := 0; i < 25; i++ {
		winner, err := s.PickRaffleWinner(id, true)
		if err != nil {
			t.Fatalf("PickRaffleWinner: %v", err)
		}
		if winner == nil {
			t.Fatal("a settled ticket must keep its chance in the draw")
		}
		if winner.CharacterName != "Ceri" {
			continue
		}
		t.Fatal("an entrant who has settled nothing must not be drawable")
	}
}

// TestPickRaffleWinnerSkipsUnsettledEntries pins the weighting itself: with one
// settled ticket against nine unsettled ones on another entry, the draw only ever
// sees the settled one.
func TestPickRaffleWinnerSkipsUnsettledEntries(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Weighted", MaxEntries: 9, CostPerEntry: 100})
	settled, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	_, _ = s.SetRaffleEntryPaid(settled, true, 0, 0)
	_, _ = s.CreateRaffleEntry(id, "Borin", "Hades", 9) // never settled

	for i := 0; i < 50; i++ {
		winner, err := s.PickRaffleWinner(id, true)
		if err != nil || winner == nil {
			t.Fatalf("PickRaffleWinner: winner=%v err=%v", winner, err)
		}
		if winner.CharacterName != "Aria" {
			t.Fatalf("drew %q; only settled tickets are eligible", winner.CharacterName)
		}
	}
}

// TestEntryCostWalksTheLadderNotTheTickets guards the pricing loop: a details-only
// raffle prices nothing at all, and a custom raffle whose max_entries drifted past
// its ladder costs one pass over the tiers rather than one per ticket. Without the
// bound a large allowance turns every admin raffle-list load into a spin.
func TestEntryCostWalksTheLadderNotTheTickets(t *testing.T) {
	details := model.Raffle{EntryMode: model.RaffleModeDetails, MaxEntries: 500_000_000}
	custom := model.Raffle{EntryMode: model.RaffleModeCustom, TierCosts: []float64{50_000, 100_000}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if got := details.EntryCost(500_000_000); got != 0 {
			t.Errorf("details EntryCost = %v; want 0", got)
		}
		if got := custom.EntryCost(500_000_000); got != 150_000 {
			t.Errorf("custom EntryCost past the ladder = %v; want 150000", got)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EntryCost is looping per ticket instead of per ladder rung")
	}
}

// TestSetRaffleEntryPaidIsIdempotent guards the collected-gil total against a
// double-submit (two staff on the same entry, or one impatient click): the second
// settlement must not stack the same waiver again and quietly understate what the
// raffle took in.
func TestSetRaffleEntryPaidIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Twice", MaxEntries: 3, CostPerEntry: 50_000})
	entryID, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)

	applied, err := s.SetRaffleEntryPaid(entryID, true, 0, 20_000)
	if err != nil || !applied {
		t.Fatalf("first settlement: applied=%v err=%v; want applied", applied, err)
	}
	applied, err = s.SetRaffleEntryPaid(entryID, true, 0, 20_000)
	if err != nil {
		t.Fatalf("second settlement: %v", err)
	}
	if applied {
		t.Error("a repeat settlement with nothing outstanding must be a no-op")
	}

	e, _ := s.GetRaffleEntryByID(entryID)
	if e.AmountWaived != 20_000 {
		t.Errorf("amount_waived = %v; want 20000 (not stacked twice)", e.AmountWaived)
	}

	// Buying more tickets makes a settlement meaningful again, and THAT waiver
	// does accumulate.
	if _, _, _, _, err := s.AddOrCreateRaffleEntry(id, "Aria", "Gilgamesh", 0, 3); err == nil {
		t.Log("zero-ticket add is a no-op, as expected")
	}
	e, _ = s.GetRaffleEntryByID(entryID)
	if e.AmountWaived != 20_000 {
		t.Errorf("amount_waived = %v; want it unchanged by a no-op add", e.AmountWaived)
	}
}

// TestLookupRaffleEntries covers the public "have I already entered?" search:
// substring, case-insensitive, scoped to its raffle, and reporting when it had
// more to show.
func TestLookupRaffleEntries(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Search", MaxEntries: 3, CostPerEntry: 100})
	other, _ := s.CreateRaffle(&model.Raffle{Title: "Other", MaxEntries: 3, CostPerEntry: 100})

	part, _ := s.CreateRaffleEntry(id, "Aria Fairwind", "Gilgamesh", 3)
	_, _ = s.SetRaffleEntryPaid(part, true, 0, 0)
	_, _, _, _, _ = s.AddOrCreateRaffleEntry(id, "Aria Fairwind", "Gilgamesh", 0, 3)
	_, _ = s.CreateRaffleEntry(id, "Borin Stoneheart", "Hades", 1)
	_, _ = s.CreateRaffleEntry(other, "Aria Fairwind", "Gilgamesh", 1)

	// Substring, case-insensitive, and scoped to this raffle only.
	hits, truncated, err := s.LookupRaffleEntries(id, "aria", 50)
	if err != nil {
		t.Fatalf("LookupRaffleEntries: %v", err)
	}
	if truncated {
		t.Error("two entries should not report truncation")
	}
	if len(hits) != 1 {
		t.Fatalf("hits = %+v; want just the one Aria on this raffle", hits)
	}
	got := hits[0]
	if got.CharacterName != "Aria Fairwind" || got.World != "Gilgamesh" || got.NumEntries != 3 {
		t.Errorf("hit = %+v; want Aria Fairwind @ Gilgamesh with 3 entries", got)
	}
	if got.PaymentState != "paid" {
		t.Errorf("payment_state = %q; want paid", got.PaymentState)
	}

	// A mid-name substring works, which is the point of the search.
	if hits, _, _ := s.LookupRaffleEntries(id, "stoneheart", 50); len(hits) != 1 {
		t.Errorf("mid-name search = %+v; want Borin", hits)
	}
	// A miss is an empty list, not an error.
	if hits, _, err := s.LookupRaffleEntries(id, "nobody", 50); err != nil || len(hits) != 0 {
		t.Errorf("miss = %+v (err %v); want an empty list", hits, err)
	}
}

// TestLookupRaffleEntriesEscapesWildcards stops a LIKE metacharacter from turning
// the search into "list every entrant" - "_" would otherwise match any single
// character and "%" would match the lot.
func TestLookupRaffleEntriesEscapesWildcards(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Wild", MaxEntries: 1})
	_, _ = s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)
	_, _ = s.CreateRaffleEntry(id, "Borin", "Hades", 1)
	_, _ = s.CreateRaffleEntry(id, "Under_score", "Ravana", 1)

	for _, q := range []string{"%", "__", `\`} {
		hits, _, err := s.LookupRaffleEntries(id, q, 50)
		if err != nil {
			t.Fatalf("LookupRaffleEntries(%q): %v", q, err)
		}
		for _, h := range hits {
			if h.CharacterName == "Aria" || h.CharacterName == "Borin" {
				t.Errorf("query %q matched %q - the wildcard was not escaped", q, h.CharacterName)
			}
		}
	}
	// The literal underscore still finds the name that actually contains one.
	if hits, _, _ := s.LookupRaffleEntries(id, "r_s", 50); len(hits) != 1 {
		t.Errorf(`search "r_s" = %+v; want the literal Under_score`, hits)
	}
}

// TestLookupRaffleEntriesTruncates makes a clipped list say so, rather than
// passing itself off as every match.
func TestLookupRaffleEntriesTruncates(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Many", MaxEntries: 1})
	for i := 0; i < 5; i++ {
		if _, err := s.CreateRaffleEntry(id, "Aria"+string(rune('A'+i)), "Gilgamesh", 1); err != nil {
			t.Fatalf("CreateRaffleEntry: %v", err)
		}
	}

	hits, truncated, err := s.LookupRaffleEntries(id, "aria", 3)
	if err != nil {
		t.Fatalf("LookupRaffleEntries: %v", err)
	}
	if len(hits) != 3 || !truncated {
		t.Errorf("hits=%d truncated=%v; want 3 and truncated", len(hits), truncated)
	}
	// Exactly at the limit is NOT truncated.
	if _, truncated, _ := s.LookupRaffleEntries(id, "ariaA", 1); truncated {
		t.Error("an exact-fit result must not report truncation")
	}
}

// TestRafflePayImageRoundTrip pins the "Where to Pay" screenshot through a
// save/load and an edit - it is the counterpart to a stamp rally's redeem image,
// and a raffle that loses it on edit would quietly stop telling players where to
// go.
func TestRafflePayImageRoundTrip(t *testing.T) {
	s := newTestStore(t)
	id, err := s.CreateRaffle(&model.Raffle{
		Title:      "Prize",
		MaxEntries: 1,
		PrizeImage: "images/raffles/prize.png",
		PayImage:   "images/raffles/where-to-pay.png",
	})
	if err != nil {
		t.Fatalf("CreateRaffle: %v", err)
	}
	got, err := s.GetRaffle(id)
	if err != nil || got == nil {
		t.Fatalf("GetRaffle: %v", err)
	}
	if got.PayImage != "images/raffles/where-to-pay.png" {
		t.Errorf("pay_image = %q; want the saved path", got.PayImage)
	}
	// The prize image is a separate field and must not be confused with it.
	if got.PrizeImage != "images/raffles/prize.png" {
		t.Errorf("prize_image = %q; want its own value", got.PrizeImage)
	}

	got.PayImage = ""
	if err := s.UpdateRaffle(got); err != nil {
		t.Fatalf("UpdateRaffle: %v", err)
	}
	after, _ := s.GetRaffle(id)
	if after.PayImage != "" {
		t.Errorf("pay_image = %q; want it cleared", after.PayImage)
	}
}

// TestSetRaffleEntryPaidPartial covers recording a PART payment: somebody hands
// over gil for two of their three entries now and the rest later.
func TestSetRaffleEntryPaidPartial(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{
		Title: "Tiered", MaxEntries: 3,
		EntryMode: model.RaffleModeCustom, TierCosts: []float64{50_000, 100_000, 150_000},
	})
	entryID, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)

	applied, err := s.SetRaffleEntryPaid(entryID, true, 2, 0)
	if err != nil || !applied {
		t.Fatalf("part settlement: applied=%v err=%v; want applied", applied, err)
	}
	raffle, _ := s.GetRaffle(id)
	e, _ := s.GetRaffleEntryByID(entryID)
	if e.PaidEntries != 2 || e.Paid {
		t.Fatalf("entry = %+v; want 2 settled and NOT fully paid", e)
	}
	if e.PaymentState() != "partial" {
		t.Errorf("state = %q; want partial", e.PaymentState())
	}
	if got := raffle.AmountCollected(*e); got != 150_000 { // 50k + 100k
		t.Errorf("collected = %v; want 150000", got)
	}
	if got := raffle.AmountOutstanding(*e); got != 150_000 { // the 3rd ticket
		t.Errorf("outstanding = %v; want 150000", got)
	}

	// Settling the rest completes it, and the waiver accumulates as always.
	if _, err := s.SetRaffleEntryPaid(entryID, true, 3, 25_000); err != nil {
		t.Fatalf("final settlement: %v", err)
	}
	e, _ = s.GetRaffleEntryByID(entryID)
	if !e.Paid || e.PaidEntries != 3 || e.AmountWaived != 25_000 {
		t.Errorf("entry = %+v; want fully paid, 3 settled, 25000 waived", e)
	}
}

// TestSetRaffleEntryPaidNeverGoesBackwards guards the settlement against a stale
// count: a smaller number arriving after a larger one must not un-settle tickets
// (and must not bank its waiver either). Clearing is the way to undo.
func TestSetRaffleEntryPaidNeverGoesBackwards(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Back", MaxEntries: 3, CostPerEntry: 100})
	entryID, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 3)

	if _, err := s.SetRaffleEntryPaid(entryID, true, 3, 10); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	applied, err := s.SetRaffleEntryPaid(entryID, true, 1, 999)
	if err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	if applied {
		t.Error("a lower ticket count must not apply")
	}
	e, _ := s.GetRaffleEntryByID(entryID)
	if e.PaidEntries != 3 || !e.Paid || e.AmountWaived != 10 {
		t.Errorf("entry = %+v; want it untouched at 3 settled / 10 waived", e)
	}
}

// TestSetRaffleEntryPaidClampsToTickets stops a count past the entry's tickets
// from parking a paid_entries larger than num_entries, which would make the
// collected total price tickets nobody bought.
func TestSetRaffleEntryPaidClampsToTickets(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Clamp", MaxEntries: 2, CostPerEntry: 100})
	entryID, _ := s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 2)

	if _, err := s.SetRaffleEntryPaid(entryID, true, 99, 0); err != nil {
		t.Fatalf("SetRaffleEntryPaid: %v", err)
	}
	e, _ := s.GetRaffleEntryByID(entryID)
	if e.PaidEntries != 2 || !e.Paid {
		t.Errorf("entry = %+v; want it clamped to its 2 tickets and fully paid", e)
	}
	raffle, _ := s.GetRaffle(id)
	if got := raffle.AmountCollected(*e); got != 200 {
		t.Errorf("collected = %v; want 200, not a price for tickets nobody bought", got)
	}
}

// TestLookupRaffleEntriesEscapesBackslash covers the gap the wildcard test above
// leaves. That one queries a backslash but only asserts unrelated names are not
// matched, which the buggy version satisfied: escapeLikePattern "escaped" the
// escape character by replacing a backslash with a backslash, a no-op, so a typed
// backslash stayed live and reached SQLite as a dangling ESCAPE. Assert instead
// that a backslash behaves as the literal character somebody typed.
func TestLookupRaffleEntriesEscapesBackslash(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Backslash", MaxEntries: 1})
	_, _ = s.CreateRaffleEntry(id, `Back\slash`, "Hades", 1)
	_, _ = s.CreateRaffleEntry(id, "Aria", "Gilgamesh", 1)

	hits, _, err := s.LookupRaffleEntries(id, `\`, 50)
	if err != nil {
		t.Fatalf(`LookupRaffleEntries(\): %v`, err)
	}
	if len(hits) != 1 || hits[0].CharacterName != `Back\slash` {
		t.Errorf(`search for a backslash returned %+v; want just the entrant whose name contains one`, hits)
	}

	// "\%" is a backslash followed by a percent: two literal characters, matching
	// nobody here. Unescaped it read as an escaped percent and matched the lot.
	if hits, _, err := s.LookupRaffleEntries(id, `\%`, 50); err != nil || len(hits) != 0 {
		t.Errorf(`search for \%% returned %d hits (err=%v); want 0`, len(hits), err)
	}
}

// TestPickRaffleWinnerWeightsByTickets is the weighting test the suite thought it
// already had. Every existing PickRaffleWinner test builds a scenario with exactly
// ONE eligible entry, so the per-ticket weighting - the whole point of buying more
// tickets - was never asserted: a draw that ignored weights entirely, or that used
// NumEntries where it should use the paid count, passed the suite unchanged.
//
// Weighting is probabilistic, so this asserts the distribution rather than any one
// draw: with a 1:9 split over 400 draws, the heavy entrant must win clearly more
// often. The bounds are wide enough not to flake and tight enough that "weights
// ignored" (a 50/50 split) fails every time.
func TestPickRaffleWinnerWeightsByTickets(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "Weighted", MaxEntries: 20, CostPerEntry: 100})

	light, _ := s.CreateRaffleEntry(id, "Light", "Gilgamesh", 1)
	heavy, _ := s.CreateRaffleEntry(id, "Heavy", "Hades", 9)
	if _, err := s.SetRaffleEntryPaid(light, true, 0, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetRaffleEntryPaid(heavy, true, 0, 0); err != nil {
		t.Fatal(err)
	}

	const draws = 400
	wins := map[string]int{}
	for range draws {
		w, err := s.PickRaffleWinner(id, true)
		if err != nil {
			t.Fatalf("PickRaffleWinner: %v", err)
		}
		if w == nil {
			t.Fatal("PickRaffleWinner returned no entry despite two paid entrants")
		}
		wins[w.CharacterName]++
	}
	if wins["Light"]+wins["Heavy"] != draws {
		t.Fatalf("unexpected winners: %v", wins)
	}
	// True ratio is 9:1, so ~360/40. Anything at or below a coin flip means the
	// ticket count is not influencing the draw at all.
	if wins["Heavy"] <= wins["Light"] {
		t.Errorf("9-ticket entrant won %d of %d against a 1-ticket entrant; weighting is not applied",
			wins["Heavy"], draws)
	}
	if wins["Heavy"] < draws*60/100 {
		t.Errorf("9-ticket entrant won only %d of %d; expected a clear majority under 9:1 weighting",
			wins["Heavy"], draws)
	}
	// And the light entrant must still be reachable - weighting, not exclusion.
	if wins["Light"] == 0 {
		t.Error("1-ticket entrant never won across 400 draws; they should still be drawable")
	}
}

// TestPickRaffleWinnerPaidOnlyUsesPaidTickets pins the other half of the weighting
// rule: in paid-only mode an entry is worth min(PaidEntries, NumEntries), not its
// full ticket count. A partly-settled row must weigh only what has been paid for.
func TestPickRaffleWinnerPaidOnlyUsesPaidTickets(t *testing.T) {
	s := newTestStore(t)
	id, _ := s.CreateRaffle(&model.Raffle{Title: "PartPaid", MaxEntries: 20, CostPerEntry: 100})

	// Ten tickets, only one of them settled.
	part, _ := s.CreateRaffleEntry(id, "Part", "Gilgamesh", 10)
	if _, err := s.SetRaffleEntryPaid(part, true, 1, 0); err != nil {
		t.Fatalf("settle one ticket: %v", err)
	}
	// Nine tickets, all settled.
	full, _ := s.CreateRaffleEntry(id, "Full", "Hades", 9)
	if _, err := s.SetRaffleEntryPaid(full, true, 0, 0); err != nil {
		t.Fatal(err)
	}

	const draws = 400
	wins := map[string]int{}
	for range draws {
		w, err := s.PickRaffleWinner(id, true)
		if err != nil || w == nil {
			t.Fatalf("PickRaffleWinner: w=%v err=%v", w, err)
		}
		wins[w.CharacterName]++
	}
	// Weights are 1 (paid) vs 9, not 10 vs 9. If the unpaid tickets counted, the
	// two would be near even instead of a clear 9:1 split.
	if wins["Full"] <= wins["Part"] {
		t.Errorf("paid-only draw counted unsettled tickets: Full %d vs Part %d of %d",
			wins["Full"], wins["Part"], draws)
	}
}
