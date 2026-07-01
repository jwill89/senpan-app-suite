package model

// Raffle represents a raffle event that players can enter.
// Raffles have configurable entry limits, costs, and time windows.
type Raffle struct {
	ID                 int64   `json:"id"`
	Title              string  `json:"title"`
	Description        string  `json:"description"`
	Rules              string  `json:"rules"`
	MaxEntries         int     `json:"max_entries"`         // max entries per player
	SignupInstructions string  `json:"signup_instructions"` // shown to player after signup
	CostPerEntry       float64 `json:"cost_per_entry"`
	AvailableFrom      string  `json:"available_from"` // UTC RFC-3339 datetime; empty = always open
	AvailableTo        string  `json:"available_to"`   // UTC RFC-3339 datetime; empty = no end
	PrizeImage         string  `json:"prize_image"`    // relative web path to uploaded image
	Status             string  `json:"status"`         // "open" or "closed"
	WinnerEntryID      *int64  `json:"winner_entry_id"`
	CreatedAt          string  `json:"created_at"`

	// Read-only aggregates populated for the admin list view only (the closed-raffle
	// table): the verified winner's "Character @ World", and the collected total
	// (sum of paid tickets × cost_per_entry). Omitted from public responses.
	WinnerName string  `json:"winner_name,omitempty"`
	PaidTotal  float64 `json:"paid_total,omitempty"`
}

// RaffleEntry represents a user's entry (one or more tickets) into a raffle.
type RaffleEntry struct {
	ID            int64  `json:"id"`
	RaffleID      int64  `json:"raffle_id"`
	CharacterName string `json:"character_name"` // in-game character name
	World         string `json:"world"`          // game world/server
	NumEntries    int    `json:"num_entries"`    // number of tickets purchased
	Paid          bool   `json:"paid"`           // admin-verified payment status
	CreatedAt     string `json:"created_at"`
}

// RafflesResponse is the body of GET /api/raffles — the visible raffle list
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
//   - raffle        — always present.
//   - total_entries — present whenever the count query succeeds (pointer +
//     omitempty so a count error omits the key, matching the map that only sets it
//     on err == nil).
//   - entries       — admins only: the full entry list. A POINTER to the slice so
//     the admin branch always emits the key (even "entries":[] when there are no
//     entries — the store returns a non-nil empty slice), while a nil pointer omits
//     the key entirely for public callers. A plain []RaffleEntry with omitempty
//     would wrongly drop the key on an empty admin list.
//   - winner_entry  — public only, and only for a closed raffle that has a verified
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

// RaffleEnterResponse is the body of POST /api/raffles/{id}/enter — the public
// sign-up confirmation (HTTP 201 on a new entry, 200 when adding to an existing
// one; the body shape is identical either way).
type RaffleEnterResponse struct {
	Message            string  `json:"message"`
	TotalEntries       int     `json:"total_entries"`
	TotalCost          float64 `json:"total_cost"`
	SignupInstructions string  `json:"signup_instructions"`
}

// RaffleEntryResponse wraps a single entry — the body of POST
// /api/raffles/{id}/entries {action:"add_entry"} (the created/updated entry).
type RaffleEntryResponse struct {
	Entry RaffleEntry `json:"entry"`
}

// RaffleWinnerResponse wraps the picked entry — the body of POST
// /api/raffles/{id}/entries {action:"pick_winner"|"pick_another"}.
type RaffleWinnerResponse struct {
	Winner RaffleEntry `json:"winner"`
}
