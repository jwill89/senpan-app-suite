package model

import "app-suite/internal/raffles"

// Raffle represents a raffle event that players can enter.
// Raffles have configurable entry limits, costs, and time windows.
type Raffle struct {
	ID                 int64     `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Rules              string    `json:"rules"`
	MaxEntries         int       `json:"max_entries"`         // max entries per player
	SignupInstructions string    `json:"signup_instructions"` // how to pay/sign up (details-only: shown on the page; otherwise after signup)
	EntryMode          string    `json:"entry_mode"`          // "details" | "single" | "custom" - see package raffles
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

// pricing is what the raffle charges for its tickets.
func (r Raffle) pricing() raffles.Pricing {
	tiers := make([]int64, len(r.TierCosts))
	for i, c := range r.TierCosts {
		tiers[i] = raffles.Gil(c)
	}
	return raffles.Pricing{Mode: r.EntryMode, CostPerEntry: raffles.Gil(r.CostPerEntry), TierCosts: tiers}
}

// Mode returns the raffle's normalized entry mode (raffles.NormalizeMode).
func (r Raffle) Mode() string { return raffles.NormalizeMode(r.EntryMode) }

// AcceptsSignups reports whether the public sign-up form applies at all.
func (r Raffle) AcceptsSignups() bool { return raffles.AcceptsSignups(r.EntryMode) }

// EntryCost returns what holding n tickets costs in total (raffles.Pricing.EntryCost).
func (r Raffle) EntryCost(n int) float64 { return float64(r.pricing().EntryCost(n)) }

// RaffleLookupEntry is one entry of a participant's own in a festival's raffles: enough
// to recognize their sign-up and reproduce the exact name they used, plus how far it
// has been paid. It carries no entry id and no gil figure, because the amount waived
// on an entry is a private arrangement between that person and the staff.
type RaffleLookupEntry struct {
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	NumEntries    int    `json:"num_entries"`
	PaidEntries   int    `json:"paid_entries"`
	PaymentState  string `json:"payment_state"` // "unpaid" | "partial" | "paid"
}
