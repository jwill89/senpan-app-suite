package raffles_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"app-suite/internal/raffles"
)

// A custom raffle charges the 1st, 2nd and 3rd ticket its own rung, so three tickets
// cost the sum of the ladder, not three times the first rung.
func TestEntryCostByMode(t *testing.T) {
	single := raffles.Pricing{Mode: raffles.ModeSingle, CostPerEntry: 50_000}
	custom := raffles.Pricing{Mode: raffles.ModeCustom, TierCosts: []int64{50_000, 100_000, 150_000}}
	details := raffles.Pricing{Mode: raffles.ModeDetails, CostPerEntry: 50_000, TierCosts: []int64{9}}
	legacy := raffles.Pricing{Mode: "", CostPerEntry: 50_000}

	cases := []struct {
		name    string
		pricing raffles.Pricing
		tickets int
		want    int64
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
			if got := tc.pricing.EntryCost(tc.tickets); got != tc.want {
				t.Errorf("EntryCost(%d) = %d; want %d", tc.tickets, got, tc.want)
			}
		})
	}

	if raffles.AcceptsSignups(raffles.ModeDetails) {
		t.Error("a details-only raffle must not accept sign-ups")
	}
	if !raffles.AcceptsSignups("") {
		t.Error("a raffle with no mode must keep accepting sign-ups")
	}
}

// A large allowance must not turn pricing into a loop per ticket.
func TestEntryCostWalksTheLadderNotTheTickets(t *testing.T) {
	details := raffles.Pricing{Mode: raffles.ModeDetails}
	custom := raffles.Pricing{Mode: raffles.ModeCustom, TierCosts: []int64{50_000, 100_000}}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if got := details.EntryCost(500_000_000); got != 0 {
			t.Errorf("details EntryCost = %d; want 0", got)
		}
		if got := custom.EntryCost(500_000_000); got != 150_000 {
			t.Errorf("custom EntryCost past the ladder = %d; want 150000", got)
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("EntryCost is looping per ticket instead of per ladder rung")
	}
}

// A waiver is added to what a settlement collected, never below zero, and what is
// outstanding is the unsettled tickets at full price.
func TestSettlementAmounts(t *testing.T) {
	p := raffles.Pricing{Mode: raffles.ModeCustom, TierCosts: []int64{50_000, 100_000, 150_000}}
	e := raffles.Entry{NumEntries: 3, PaidEntries: 2, AmountWaived: 25_000}

	if got := p.TicketPrice(e); got != 300_000 {
		t.Errorf("TicketPrice = %d; want 300000", got)
	}
	if got := p.AmountCollected(e); got != 125_000 { // 150k settled less 25k waived
		t.Errorf("AmountCollected = %d; want 125000", got)
	}
	if got := p.AmountOutstanding(e); got != 150_000 { // the 3rd ticket
		t.Errorf("AmountOutstanding = %d; want 150000", got)
	}
	over := raffles.Entry{NumEntries: 1, PaidEntries: 1, AmountWaived: 999_999}
	if got := p.AmountCollected(over); got != 0 {
		t.Errorf("an over-generous waiver collected %d; want it floored at 0", got)
	}
}

func TestPaymentState(t *testing.T) {
	for _, c := range []struct {
		num, paid int
		want      string
	}{{3, 0, raffles.Unpaid}, {3, 1, raffles.Partial}, {3, 3, raffles.Paid}, {3, 5, raffles.Paid}} {
		if got := raffles.PaymentState(c.num, c.paid); got != c.want {
			t.Errorf("PaymentState(%d, %d) = %q; want %q", c.num, c.paid, got, c.want)
		}
	}
}

func TestNormalizeMode(t *testing.T) {
	for in, want := range map[string]string{
		"": raffles.ModeSingle, "weird": raffles.ModeSingle, "single": raffles.ModeSingle,
		"details": raffles.ModeDetails, "custom": raffles.ModeCustom,
	} {
		if got := raffles.NormalizeMode(in); got != want {
			t.Errorf("NormalizeMode(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestParseTime(t *testing.T) {
	if _, ok := raffles.ParseTime(""); ok {
		t.Error("an empty time should report false (no constraint)")
	}
	if _, ok := raffles.ParseTime("not-a-date"); ok {
		t.Error("garbage should report false")
	}
	rfc, ok := raffles.ParseTime("2026-06-13T20:00:00.000Z")
	if !ok || rfc.Location().String() != "UTC" {
		t.Errorf("RFC 3339 parse failed: ok=%v loc=%v", ok, rfc.Location())
	}
	if naive, ok := raffles.ParseTime("2026-06-13T20:00"); !ok || naive.Location().String() != "UTC" {
		t.Errorf("naive parse failed: ok=%v loc=%v", ok, naive.Location())
	}
}

func TestWindowRules(t *testing.T) {
	now := time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	const (
		past   = "2026-06-01T00:00:00Z"
		future = "2026-07-01T00:00:00Z"
	)
	for _, c := range []struct {
		name                          string
		status, from, to              string
		isOpen, publiclyViewable, run bool
	}{
		{"open, no window", "open", "", "", true, true, true},
		{"open, inside", "open", past, future, true, true, true},
		{"open, not yet", "open", future, "", false, false, false},
		{"open, over", "open", "", past, false, false, false},
		{"closed", "closed", past, future, false, true, false},
		{"closed, over", "closed", "", past, false, true, false},
	} {
		if got := raffles.IsOpen(c.status, c.from, c.to, now); got != c.isOpen {
			t.Errorf("%s: IsOpen = %v", c.name, got)
		}
		if got := raffles.PubliclyViewable(c.status, c.from, c.to, now); got != c.publiclyViewable {
			t.Errorf("%s: PubliclyViewable = %v", c.name, got)
		}
		if got := raffles.Running(c.status, c.from, c.to, now); got != c.run {
			t.Errorf("%s: Running = %v", c.name, got)
		}
	}
}

func TestTickets(t *testing.T) {
	if got := raffles.Tickets(10, 1, true); got != 1 {
		t.Errorf("paid-only: %d; want the 1 settled ticket", got)
	}
	if got := raffles.Tickets(2, 5, true); got != 2 {
		t.Errorf("paid-only past the held tickets: %d; want 2", got)
	}
	if got := raffles.Tickets(4, 0, false); got != 4 {
		t.Errorf("not paid-only: %d; want every ticket held", got)
	}
}

func TestPick(t *testing.T) {
	if got := raffles.Pick(nil, rand.IntN); got != -1 {
		t.Errorf("no weights: %d, want -1", got)
	}
	if got := raffles.Pick([]int64{0, 0}, rand.IntN); got != -1 {
		t.Errorf("zero weights: %d, want -1", got)
	}
	// The draw maps a number onto the cumulative weights.
	for at, want := range []int{1, 1, 1, 2, 2} {
		got := raffles.Pick([]int64{0, 3, 2}, func(int) int { return at })
		if got != want {
			t.Errorf("number %d lands on %d, want %d", at, got, want)
		}
	}

	// Weighting, not exclusion: with 1:9 over many draws the heavy entry wins clearly
	// more often and the light one is still reachable.
	wins := [2]int{}
	for range 4000 {
		wins[raffles.Pick([]int64{1, 9}, rand.IntN)]++
	}
	if wins[1] < 3000 || wins[0] == 0 {
		t.Errorf("1:9 weighting gave %v", wins)
	}
}
