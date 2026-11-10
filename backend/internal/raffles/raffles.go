// Package raffles holds the rules of a raffle that need no database, clock or
// network of their own: how an entry mode prices a ticket, what an entry owes and
// has settled, when a raffle can be entered, and how a winner is drawn. Amounts are
// whole gil.
package raffles

import (
	"math"
	"time"
)

// The entry modes: how, and whether, the public may sign up, and how a ticket is
// priced.
const (
	// ModeDetails is a raffle published for reference only. There is no sign-up form;
	// players follow its instructions to enter somewhere else, and staff can still
	// record entries by hand and draw a winner. It tracks no money.
	ModeDetails = "details"
	// ModeSingle charges every ticket the same cost per entry. It is the default.
	ModeSingle = "single"
	// ModeCustom prices each ticket on its own, from a ladder in order: 50k, 100k and
	// 150k charge 300k for three tickets. The ladder's length is the allowance.
	ModeCustom = "custom"
)

// MaxEntries is the most tickets one person may hold in a raffle, and so the longest
// a custom ladder can be.
const MaxEntries = 100

// NormalizeMode maps a stored or received mode onto a known one, falling back to
// ModeSingle: a row written before entry modes existed holds "", and an unrecognized
// value must never read as free or as taking no sign-ups.
func NormalizeMode(mode string) string {
	switch mode {
	case ModeDetails, ModeCustom:
		return mode
	default:
		return ModeSingle
	}
}

// AcceptsSignups reports whether the public sign-up form applies: every mode but
// ModeDetails.
func AcceptsSignups(mode string) bool { return NormalizeMode(mode) != ModeDetails }

// Pricing is what a raffle charges for its tickets.
type Pricing struct {
	// Mode is the entry mode; any value is normalized.
	Mode string
	// CostPerEntry is the price of every ticket in ModeSingle.
	CostPerEntry int64
	// TierCosts is the price of the 1st, 2nd, ... ticket in ModeCustom.
	TierCosts []int64
}

// Entry is how many tickets an entry holds and how far they are settled.
type Entry struct {
	// NumEntries is the tickets held.
	NumEntries int
	// PaidEntries is how many of them are settled.
	PaidEntries int
	// AmountWaived is the gil forgiven across all the entry's settlements.
	AmountWaived int64
}

// TicketCost is the price of the n-th ticket (1-based). A ticket past the end of the
// custom ladder costs nothing: the allowance is pinned to the ladder's length, so
// that guards a hand-edited row rather than a reachable state.
func (p Pricing) TicketCost(n int) int64 {
	switch NormalizeMode(p.Mode) {
	case ModeDetails:
		return 0
	case ModeCustom:
		if n < 1 || n > len(p.TierCosts) {
			return 0
		}
		return p.TierCosts[n-1]
	default:
		return p.CostPerEntry
	}
}

// EntryCost is what holding n tickets costs: n times the cost per entry in
// ModeSingle, the sum of the first n rungs in ModeCustom, and 0 in ModeDetails.
//
// The custom sum walks the ladder, not the ticket count: only rungs that exist carry
// a price, so an allowance that drifted above its ladder costs one pass over the
// rungs rather than one per ticket.
func (p Pricing) EntryCost(n int) int64 {
	if n < 1 {
		return 0
	}
	switch NormalizeMode(p.Mode) {
	case ModeDetails:
		return 0
	case ModeSingle:
		return int64(n) * p.CostPerEntry
	}
	n = min(n, len(p.TierCosts))
	var total int64
	for i := 1; i <= n; i++ {
		total += p.TicketCost(i)
	}
	return total
}

// TicketPrice is the sticker price of every ticket on e, before any waiver.
func (p Pricing) TicketPrice(e Entry) int64 { return p.EntryCost(e.NumEntries) }

// AmountCollected is the gil e handed over: the price of the tickets it has settled,
// less everything waived on it. It is floored at zero, so waiving more than the
// tickets cost reads as collecting nothing rather than as a negative that would eat
// into another entry's contribution.
func (p Pricing) AmountCollected(e Entry) int64 {
	return max(p.EntryCost(e.PaidEntries)-e.AmountWaived, 0)
}

// AmountOutstanding is the sticker price of the tickets e has not settled yet. A
// waiver is not predicted: one is chosen when a payment is recorded, so an unsettled
// ticket is quoted at full price.
func (p Pricing) AmountOutstanding(e Entry) int64 {
	return max(p.EntryCost(e.NumEntries)-p.EntryCost(e.PaidEntries), 0)
}

// The payment states.
const (
	Unpaid  = "unpaid"
	Partial = "partial"
	Paid    = "paid"
)

// PaymentState is how far an entry holding numEntries tickets has settled: Unpaid,
// Partial (some tickets settled, more bought since) or Paid.
func PaymentState(numEntries, paidEntries int) string {
	switch {
	case paidEntries <= 0:
		return Unpaid
	case paidEntries < numEntries:
		return Partial
	default:
		return Paid
	}
}

// ValidWaiver reports whether an amount waived can be recorded: not negative. (A
// waiver that is not a whole number of gil never gets this far.)
func ValidWaiver(amount int64) bool { return amount >= 0 }

// Gil is a stored amount as whole gil.
func Gil(amount float64) int64 { return int64(math.Round(amount)) }

// ParseTime reads a raffle time into a UTC instant. Times are stored as RFC 3339;
// older ones are the naive "2006-01-02T15:04", read as UTC to agree with the SQL
// that filters by the clock. It reports false for an empty or unreadable time, which
// is no constraint.
func ParseTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02T15:04", s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// Statuses a raffle holds.
const (
	StatusOpen   = "open"
	StatusClosed = "closed"
)

// InWindow reports whether now is inside the availability window from..to; either
// end may be empty for none.
func InWindow(from, to string, now time.Time) bool {
	if t, ok := ParseTime(from); ok && now.Before(t) {
		return false
	}
	if t, ok := ParseTime(to); ok && now.After(t) {
		return false
	}
	return true
}

// IsOpen reports whether a raffle is taking entries at now: open, and inside its
// availability window.
func IsOpen(status, from, to string, now time.Time) bool {
	return status == StatusOpen && InWindow(from, to, now)
}

// PubliclyViewable reports whether anyone may see a raffle at now: a closed one (its
// winner announcement) or one that IsOpen. A raffle not yet open stays hidden, so
// guessing its id reveals nothing.
func PubliclyViewable(status, from, to string, now time.Time) bool {
	return status == StatusClosed || IsOpen(status, from, to, now)
}

// Running reports whether a raffle can be entered at now: not closed, and inside its
// availability window.
func Running(status, from, to string, now time.Time) bool {
	return status != StatusClosed && InWindow(from, to, now)
}

// Tickets is how many chances an entry has in the draw. A raffle that charges for
// tickets draws on the tickets SETTLED, not those held: entries merge, so somebody
// who paid for one ticket and bought two more is part-paid, and neither dropping
// them nor giving them all three chances is fair. A details-only raffle collects
// nothing through the app, so there every ticket held counts.
func Tickets(numEntries, paidEntries int, paidOnly bool) int {
	if paidOnly {
		return max(min(paidEntries, numEntries), 0)
	}
	return max(numEntries, 0)
}

// Pick draws an index from weights, each chosen in proportion to its weight, using
// intn to choose a number in [0, n). It reports -1 when no weight is above zero.
func Pick(weights []int64, intn func(n int) int) int {
	var total int64
	for _, w := range weights {
		if w > 0 {
			total += w
		}
	}
	if total == 0 {
		return -1
	}
	at := int64(intn(int(total)))
	var cumulative int64
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		cumulative += w
		if at < cumulative {
			return i
		}
	}
	// intn broke its contract; the last weighted entry is the closest honest answer.
	for i := len(weights) - 1; i >= 0; i-- {
		if weights[i] > 0 {
			return i
		}
	}
	return -1
}
