package model

// Placement positions a stamp or prize on the card image: x/y/width/height are
// percentages of the card image's box (0–100) and Rotation is in degrees. The
// admin sets these by dragging/resizing/rotating in the visual placement editor.
type Placement struct {
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Width    float64 `json:"width"`
	Height   float64 `json:"height"`
	Rotation float64 `json:"rotation"`
}

// StampRally is the event: a named card with a background image, a default
// "not stamped" placeholder image, an availability window, markdown details and
// redeem instructions, plus its stamps and prizes (loaded on detail fetches only).
type StampRally struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	CardImage          string `json:"card_image"`          // images/stamp_cards/… (background)
	NotStampedImage    string `json:"not_stamped_image"`   // images/stamp_stamps/… (uncollected/locked placeholder)
	AvailableFrom      string `json:"available_from"`      // UTC RFC-3339 ("" = unbounded)
	AvailableTo        string `json:"available_to"`        // UTC RFC-3339 ("" = unbounded)
	Details            string `json:"details"`             // markdown
	RedeemInstructions string `json:"redeem_instructions"` // markdown, shown once complete
	RedeemImage        string `json:"redeem_image"`        // images/… screenshot of where to redeem, shown once complete
	// Status is a manual "open"/"closed" flag, independent of the availability window:
	// a closed rally is read-only (no more stamping), moves to the admin's closed table,
	// and isn't offered for Garapon linking.
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`

	// Populated on detail fetches only (omitted from list responses for efficiency).
	Stamps []StampRallyStamp `json:"stamps,omitempty"`
	Prizes []StampRallyPrize `json:"prizes,omitempty"`

	// Read-only aggregates for the admin list view: issued cards, how many of them
	// have been completed, and the stall (stamp) counts used by the list's at-a-glance
	// "X/Y stalls active" summary (ActiveStampCount = stamps not paused). Omitted
	// (zero) on detail/public responses.
	CardCount        int `json:"card_count,omitempty"`
	CompletedCount   int `json:"completed_count,omitempty"`
	StampCount       int `json:"stamp_count,omitempty"`
	ActiveStampCount int `json:"active_stamp_count,omitempty"`
}

// StampRallyStamp is one collectable stamp on a rally card: an image, the password
// a participant enters to collect it, its placement on the card, an optional active
// window (within the event window) and a manual pause toggle, and the affiliate
// (stall) it belongs to. AffiliateID is nil for the "Senpan Tea House" default.
type StampRallyStamp struct {
	ID            int64  `json:"id"`
	RallyID       int64  `json:"rally_id"`
	AffiliateID   *int64 `json:"affiliate_id"`       // nil = Senpan Tea House (default)
	AffiliateName string `json:"affiliate_name"`     // joined for display ("" → "Senpan Tea House")
	Image         string `json:"image"`              // images/stamp_stamps/…
	Password      string `json:"password,omitempty"` // omitted from public payloads
	Placement     `json:"placement"`
	ActiveFrom    string `json:"active_from"` // UTC RFC-3339 within event window ("" = whole event)
	ActiveTo      string `json:"active_to"`   // UTC RFC-3339 ("" = whole event)
	Paused        bool   `json:"paused"`
	SortOrder     int    `json:"sort_order"`
}

// StampRallyPrize is a reward revealed once a card completes: a name, an image, and
// its placement on the card. Before completion the card shows the not-stamped
// placeholder at the prize's slot instead.
type StampRallyPrize struct {
	ID        int64  `json:"id"`
	RallyID   int64  `json:"rally_id"`
	Name      string `json:"name"`
	Image     string `json:"image"` // images/stamp_prizes/…
	Placement `json:"placement"`
	SortOrder int `json:"sort_order"`
}

// StampRallyCard is a participant's tokenized card for a rally: an unguessable URL
// token (the public capability), the participant's name, and completion state.
// CollectedCount is a read-only aggregate populated for admin listings.
type StampRallyCard struct {
	ID              int64  `json:"id"`
	RallyID         int64  `json:"rally_id"`
	Token           string `json:"token"`
	ParticipantName string `json:"participant_name"`
	Completed       bool   `json:"completed"`
	CompletedAt     string `json:"completed_at"`
	CreatedAt       string `json:"created_at"`
	CollectedCount  int    `json:"collected_count,omitempty"`
}

// StampRallyCollected records one stamp a participant collected on their card, with
// the time it was stamped. The (card_id, stamp_id) pair is unique — a stamp can't be
// collected twice and collection can't be undone. ParticipantName + StallName are
// snapshotted at collect time so the stamp log survives card/stamp deletion (RallyID
// cascade-deletes the row only when the whole rally is removed).
type StampRallyCollected struct {
	ID              int64  `json:"id"`
	RallyID         int64  `json:"rally_id"`
	CardID          int64  `json:"card_id"`
	StampID         int64  `json:"stamp_id"`
	ParticipantName string `json:"participant_name"`
	StallName       string `json:"stall_name"`
	StampedAt       string `json:"stamped_at"`
}

// StampRallyLogEntry is one row of the event-wide stamp log (the admin "View Logs"
// page): which participant collected which stall's stamp, and when. Rows are grouped
// by participant in the UI. StallName is the stamp's affiliate, or "Senpan Tea House".
type StampRallyLogEntry struct {
	CardID          int64  `json:"card_id"`
	ParticipantName string `json:"participant_name"`
	StampID         int64  `json:"stamp_id"`
	StallName       string `json:"stall_name"`
	StampedAt       string `json:"stamped_at"`
}

// StampRalliesResponse is the body of GET /api/stamp-rallies.
type StampRalliesResponse struct {
	StampRallies []StampRally `json:"stamp_rallies"`
}

// StampRallyResponse is the body of POST /api/stamp-rallies {action:"create"} — the
// freshly created rally echoed back.
type StampRallyResponse struct {
	StampRally StampRally `json:"stamp_rally"`
}

// StampRallyDetailResponse is the body of GET /api/stamp-rallies/{id}: the event
// (with stamps + prizes) plus its issued cards.
type StampRallyDetailResponse struct {
	StampRally StampRally       `json:"stamp_rally"`
	Cards      []StampRallyCard `json:"cards"`
}

// StampRallyCardResponse is the body of POST /api/stamp-rallies/{id}/cards
// {action:"create_card"} — the issued card (with token).
type StampRallyCardResponse struct {
	Card StampRallyCard `json:"card"`
}

// StampRallyLogsResponse is the body of GET /api/stamp-rallies/{id}/logs.
type StampRallyLogsResponse struct {
	Logs []StampRallyLogEntry `json:"logs"`
}

// PublicStampRally is the participant-facing rally summary (no passwords). IsActive
// is computed against the current time.
type PublicStampRally struct {
	ID                 int64  `json:"id"`
	Title              string `json:"title"`
	CardImage          string `json:"card_image"`
	NotStampedImage    string `json:"not_stamped_image"`
	Details            string `json:"details"`
	RedeemInstructions string `json:"redeem_instructions"`
	RedeemImage        string `json:"redeem_image"`
	AvailableFrom      string `json:"available_from"`
	AvailableTo        string `json:"available_to"`
	IsActive           bool   `json:"is_active"`
}

// PublicStamp is one stamp slot in the participant-facing card. AffiliateName ""
// renders as "Senpan Tea House" on the frontend. Availability/collection are
// computed against the current time.
type PublicStamp struct {
	ID            int64  `json:"id"`
	AffiliateName string `json:"affiliate_name"`
	Image         string `json:"image"`
	Placement     `json:"placement"`
	ActiveFrom    string `json:"active_from"`
	ActiveTo      string `json:"active_to"`
	Available     bool   `json:"available"`
	Collected     bool   `json:"collected"`
	CollectedAt   string `json:"collected_at"`
}

// PublicPrize always carries the placement so the card can show the not-stamped
// placeholder at the slot; Name/Image are populated only once the card is complete.
type PublicPrize struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Image     string `json:"image"`
	Placement `json:"placement"`
}

// PublicStampCard is the full participant-facing card view returned for a token.
type PublicStampCard struct {
	Rally           PublicStampRally `json:"rally"`
	ParticipantName string           `json:"participant_name"`
	Completed       bool             `json:"completed"`
	CompletedAt     string           `json:"completed_at"`
	Stamps          []PublicStamp    `json:"stamps"`
	Prizes          []PublicPrize    `json:"prizes"`
	PrizesRevealed  bool             `json:"prizes_revealed"`
}

// StampSubmitResponse is the body of POST /api/stamp-card/{token}/stamp: the
// refreshed public card plus the id of the stamp just collected.
type StampSubmitResponse struct {
	Card             PublicStampCard `json:"card"`
	CollectedStampID int64           `json:"collected_stamp_id"`
}
