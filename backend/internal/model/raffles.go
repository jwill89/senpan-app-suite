package model

// Raffle entry modes - how (and whether) the public may sign up, and how a
// ticket is priced. Stored in raffles.entry_mode.
const (
	// RaffleModeDetails - "Details Only": the raffle is published for reference
	// only. There is no sign-up form; players follow signup_instructions to enter
	// somewhere else (in game, on Discord, ...). Staff can still record entries by
	// hand and draw a winner.
	RaffleModeDetails = "details"
	// RaffleModeSingle - "Single Cost per Entry": every ticket costs
	// cost_per_entry. This is the original behavior and the default for any raffle
	// created before entry modes existed.
	RaffleModeSingle = "single"
	// RaffleModeCustom - "Custom Cost per Entry": each ticket has its own price,
	// read from TierCosts in order, so a ladder of 50k/100k/150k charges 300,000
	// for all three. TierCosts also fixes MaxEntries (one tier = one allowed
	// ticket).
	RaffleModeCustom = "custom"
)

// NormalizeRaffleMode maps a stored/received mode onto a known one, falling back
// to RaffleModeSingle. Rows written before entry modes existed hold "", and API
// clients that predate the field omit it - both mean the original flat-cost
// behavior, so an unrecognized value must never read as "free" or "no sign-up".
func NormalizeRaffleMode(mode string) string {
	switch mode {
	case RaffleModeDetails, RaffleModeCustom:
		return mode
	default:
		return RaffleModeSingle
	}
}

// Raffle represents a raffle event that players can enter.
// Raffles have configurable entry limits, costs, and time windows.
type Raffle struct {
	ID                 int64     `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Rules              string    `json:"rules"`
	MaxEntries         int       `json:"max_entries"`         // max entries per player
	SignupInstructions string    `json:"signup_instructions"` // how to pay/sign up (details-only: shown on the page; otherwise after signup)
	EntryMode          string    `json:"entry_mode"`          // "details" | "single" | "custom" - see the constants above
	CostPerEntry       float64   `json:"cost_per_entry"`      // "single" mode: the price of every ticket
	TierCosts          []float64 `json:"tier_costs"`          // "custom" mode: price of the 1st, 2nd, ... ticket
	AvailableFrom      string    `json:"available_from"`      // UTC RFC-3339 datetime; empty = always open
	AvailableTo        string    `json:"available_to"`        // UTC RFC-3339 datetime; empty = no end
	PrizeImage         string    `json:"prize_image"`         // relative web path to uploaded image
	PayImage           string    `json:"pay_image"`           // images/... "Where to Pay" screenshot, shown under the sign-up instructions
	Status             string    `json:"status"`              // "open" or "closed"
	WinnerEntryID      *int64    `json:"winner_entry_id"`
	// FestivalMapID optionally files the raffle under a Festival Map - the same
	// grouping a Stamp Rally gets, so a festival's raffles, rallies and floor plan
	// all hang off one event. nil = not part of a festival.
	FestivalMapID   *int64 `json:"festival_map_id"`
	FestivalMapName string `json:"festival_map_name,omitempty"` // joined for display
	// OccupantID assigns the raffle to one stall on that map, so the stall's panel
	// on the public plan can link to it. Only meaningful with FestivalMapID set;
	// cleared on save when the raffle names no map (see resolveRaffleStall).
	OccupantID *int64 `json:"occupant_id"`
	StallName  string `json:"stall_name,omitempty"` // joined from the occupant
	CreatedAt  string `json:"created_at"`

	// Read-only aggregates populated for the admin list view only (the closed-raffle
	// table): the verified winner's "Character @ World", and the collected total
	// (every settled ticket priced by the raffle's entry mode, less what was
	// waived). Omitted from public responses.
	WinnerName string  `json:"winner_name,omitempty"`
	PaidTotal  float64 `json:"paid_total,omitempty"`
}

// Mode returns the raffle's normalized entry mode (see NormalizeRaffleMode).
func (r Raffle) Mode() string { return NormalizeRaffleMode(r.EntryMode) }

// AcceptsSignups reports whether the public sign-up form applies at all. A
// "Details Only" raffle is published for reference and entered elsewhere.
func (r Raffle) AcceptsSignups() bool { return r.Mode() != RaffleModeDetails }

// TicketCost returns the price of the n-th ticket (1-based). A ticket past the
// end of the custom ladder costs nothing - MaxEntries is pinned to the ladder
// length, so that guards a hand-edited row rather than a reachable state.
func (r Raffle) TicketCost(n int) float64 {
	switch r.Mode() {
	case RaffleModeDetails:
		return 0
	case RaffleModeCustom:
		if n < 1 || n > len(r.TierCosts) {
			return 0
		}
		return r.TierCosts[n-1]
	default:
		return r.CostPerEntry
	}
}

// EntryCost returns what holding n tickets costs in total: n x cost_per_entry in
// "single" mode, the sum of the first n tiers in "custom" mode, and 0 for a
// details-only raffle (which tracks no money).
//
// The custom-mode sum walks the LADDER, not the ticket count: only rungs that
// exist carry a price, so a raffle whose max_entries drifted above its ladder
// (an admin shortening it after entries were taken) costs one pass over the
// tiers rather than one per ticket.
func (r Raffle) EntryCost(n int) float64 {
	if n < 1 {
		return 0
	}
	switch r.Mode() {
	case RaffleModeDetails:
		return 0
	case RaffleModeSingle:
		return float64(n) * r.CostPerEntry
	}
	if n > len(r.TierCosts) {
		n = len(r.TierCosts)
	}
	total := 0.0
	for i := 1; i <= n; i++ {
		total += r.TicketCost(i)
	}
	return total
}

// RaffleEntry represents a user's entry (one or more tickets) into a raffle.
//
// Entries are merged per character+world, so one row can grow over time: a player
// who settles up and then buys two more tickets keeps the same row. PaidEntries
// records how many of NumEntries have actually been settled, which is what makes
// that PARTIAL state visible instead of a paid row silently absorbing new unpaid
// tickets. Paid is the derived "nothing outstanding" flag (PaidEntries >=
// NumEntries) that the winner draw and the collected-gil totals read.
type RaffleEntry struct {
	ID            int64   `json:"id"`
	RaffleID      int64   `json:"raffle_id"`
	CharacterName string  `json:"character_name"` // in-game character name
	World         string  `json:"world"`          // game world/server
	NumEntries    int     `json:"num_entries"`    // number of tickets purchased
	PaidEntries   int     `json:"paid_entries"`   // how many of them are settled
	AmountWaived  float64 `json:"amount_waived"`  // gil forgiven across all settlements (cumulative)
	Paid          bool    `json:"paid"`           // derived: nothing outstanding
	CreatedAt     string  `json:"created_at"`
}

// PaymentState returns how far an entry has settled: "unpaid", "partial" (some
// tickets settled, more bought since) or "paid".
func (e RaffleEntry) PaymentState() string {
	switch {
	case e.PaidEntries <= 0:
		return "unpaid"
	case e.PaidEntries < e.NumEntries:
		return "partial"
	default:
		return "paid"
	}
}

// PublicView strips an entry down to what a non-staff caller may see. The winner
// of a closed raffle is shown publicly, and the settlement columns say how much
// that named person was let off - "Aria @ Gilgamesh had 300,000 gil waived" is
// nobody else's business, and it reads as favouritism whether or not it was. The
// ticket count stays (the raffle already publishes entry totals) and so does the
// bare paid flag, which was public before settlements were tracked.
func (e RaffleEntry) PublicView() RaffleEntry {
	e.PaidEntries = 0
	e.AmountWaived = 0
	return e
}

// TicketPrice returns the sticker price of every ticket on the entry, before any
// waiver.
func (r Raffle) TicketPrice(e RaffleEntry) float64 { return r.EntryCost(e.NumEntries) }

// AmountCollected returns the gil this entry actually handed over: the price of
// the tickets it has settled, less everything waived on it. Floored at zero, so
// waiving more than the tickets cost reads as "collected nothing" rather than as
// a negative that would eat into another entry's contribution.
func (r Raffle) AmountCollected(e RaffleEntry) float64 {
	collected := r.EntryCost(e.PaidEntries) - e.AmountWaived
	if collected < 0 {
		return 0
	}
	return collected
}

// AmountOutstanding returns the sticker price of the tickets this entry has NOT
// settled yet. Waivers are not predicted here - one is chosen at the moment a
// payment is recorded, so an unsettled ticket is quoted at full price.
func (r Raffle) AmountOutstanding(e RaffleEntry) float64 {
	outstanding := r.EntryCost(e.NumEntries) - r.EntryCost(e.PaidEntries)
	if outstanding < 0 {
		return 0
	}
	return outstanding
}

// RafflesResponse is the body of GET /api/raffles - the visible raffle list
// (filtered by role: admins see all, public sees open + in-window).
type RafflesResponse struct {
	Raffles []Raffle `json:"raffles"`
}

// RaffleResponse wraps a single raffle. Returned by POST /api/raffles
// {action:"create"} (HTTP 201) with the freshly created raffle.
type RaffleResponse struct {
	Raffle Raffle `json:"raffle"`
}

// RaffleDetailResponse is the body of GET /api/raffles/{id}. The shape varies by
// role and raffle state, so several fields are conditional:
//   - raffle        - always present.
//   - total_entries - present whenever the count query succeeds (pointer +
//     omitempty so a count error omits the key, matching the map that only sets it
//     on err == nil).
//   - entries       - admins only: the full entry list. A POINTER to the slice so
//     the admin branch always emits the key (even "entries":[] when there are no
//     entries - the store returns a non-nil empty slice), while a nil pointer omits
//     the key entirely for public callers. A plain []RaffleEntry with omitempty
//     would wrongly drop the key on an empty admin list.
//   - winner_entry  - public only, and only for a closed raffle that has a verified
//     winner whose entry loads (omitempty pointer; absent otherwise).
//
// entries and winner_entry are mutually exclusive in practice (admin vs public
// branch); whichever branch ran emits exactly its key.
type RaffleDetailResponse struct {
	Raffle       Raffle         `json:"raffle"`
	TotalEntries *int           `json:"total_entries,omitempty"`
	Entries      *[]RaffleEntry `json:"entries,omitempty"`
	WinnerEntry  *RaffleEntry   `json:"winner_entry,omitempty"`
}

// RaffleLookupEntry is one hit from the public "have I already entered?" search.
//
// It is a SEPARATE type from RaffleEntry rather than a filtered view of it, so a
// field added to the stored entry can never reach this public response by
// accident. What it carries is deliberate: enough to recognise your own sign-up
// and reproduce the exact name you used, plus how far it has been paid. What it
// omits is equally deliberate - the entry id (nothing public needs to address a
// row) and every gil figure, because the amount waived on an entry is a private
// arrangement between that person and the staff.
type RaffleLookupEntry struct {
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	NumEntries    int    `json:"num_entries"`
	PaidEntries   int    `json:"paid_entries"`
	PaymentState  string `json:"payment_state"` // "unpaid" | "partial" | "paid"
}

// RaffleLookupResponse is the body of POST /api/raffles/{id}/lookup. Truncated
// says the search matched more entries than were returned, so the page can ask
// for a narrower one instead of quietly presenting a partial list as the whole.
type RaffleLookupResponse struct {
	Entries   []RaffleLookupEntry `json:"entries"`
	Truncated bool                `json:"truncated"`
}

// RaffleEnterResponse is the body of POST /api/raffles/{id}/enter - the public
// sign-up confirmation (HTTP 201 on a new entry, 200 when adding to an existing
// one; the body shape is identical either way).
//
// TotalCost is the sticker price of every ticket the character now holds;
// AmountDue is what they actually have to send. The two differ once an entry has
// been settled in part - entries merge per character+world, so somebody who
// already paid for a ticket and comes back for two more owes only the new ones.
// Quoting TotalCost alone there invites an overpayment, which is why AmountDue is
// the figure the confirmation leads with.
//
// AmountDue deliberately says nothing about WHY the balance is lower: it folds
// settled tickets and waived gil together, so the response can't be used to work
// out that a particular character was comped.
type RaffleEnterResponse struct {
	Message            string  `json:"message"`
	TotalEntries       int     `json:"total_entries"`
	TotalCost          float64 `json:"total_cost"`
	AmountDue          float64 `json:"amount_due"`
	SignupInstructions string  `json:"signup_instructions"`
}

// RaffleEntryResponse wraps a single entry - the body of POST
// /api/raffles/{id}/entries {action:"add_entry"} (the created/updated entry).
type RaffleEntryResponse struct {
	Entry RaffleEntry `json:"entry"`
}

// RaffleWinnerResponse wraps the picked entry - the body of POST
// /api/raffles/{id}/entries {action:"pick_winner"|"pick_another"}.
type RaffleWinnerResponse struct {
	Winner RaffleEntry `json:"winner"`
}
