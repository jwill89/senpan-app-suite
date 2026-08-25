package model

import "strings"

// Stamp types - what kind of stall a stamp belongs to, so a rally can require a
// number of each. Stored in stamp_rally_stamps.stamp_type and snapshotted onto
// each collected-stamp log row.
const (
	// StampTypeFood - a food stall's stamp. The original (and only) kind of stamp,
	// so every stamp that predates types is one.
	StampTypeFood = "food"
	// StampTypeGame - a game stall's stamp.
	StampTypeGame = "game"
)

// NormalizeStampType maps a stored/received stamp type onto a known one, falling
// back to StampTypeFood. Rows written before stamp types existed hold "", and API
// clients that predate the field omit it - both mean a food stamp, which is what
// every stamp on every past rally was.
func NormalizeStampType(t string) string {
	switch t {
	case StampTypeGame:
		return StampTypeGame
	default:
		return StampTypeFood
	}
}

// Rally completion modes - what it takes to finish a card. Stored in
// stamp_rallies.completion_mode.
const (
	// RallyCompletionAll - collect every stamp on the card (any that can never be
	// collected again no longer blocks it). The original behavior and the default
	// for any rally created before completion modes existed.
	RallyCompletionAll = "all"
	// RallyCompletionCounts - collect RequiredFood food stamps and RequiredGame
	// game stamps; the rest are optional. A rally with 5 food stalls and 5 game
	// stalls can ask for 3 of each.
	RallyCompletionCounts = "counts"
)

// NormalizeRallyCompletion maps a stored/received completion mode onto a known
// one, falling back to RallyCompletionAll - so a rally written before the field
// existed (or by an older client) keeps requiring the whole card.
func NormalizeRallyCompletion(mode string) string {
	switch mode {
	case RallyCompletionCounts:
		return RallyCompletionCounts
	default:
		return RallyCompletionAll
	}
}

// Placement positions a stamp or prize on the card image: x/y/width/height are
// percentages of the card image's box (0-100) and Rotation is in degrees. The
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
	CardImage          string `json:"card_image"`          // images/stamp_cards/... (background)
	NotStampedImage    string `json:"not_stamped_image"`   // images/stamp_stamps/... (uncollected/locked placeholder)
	AvailableFrom      string `json:"available_from"`      // UTC RFC-3339 ("" = unbounded)
	AvailableTo        string `json:"available_to"`        // UTC RFC-3339 ("" = unbounded)
	Details            string `json:"details"`             // markdown
	RedeemInstructions string `json:"redeem_instructions"` // markdown, shown once complete
	RedeemImage        string `json:"redeem_image"`        // images/... screenshot of where to redeem, shown once complete
	// Status is a manual "open"/"closed" flag, independent of the availability window:
	// a closed rally is read-only (no more stamping), moves to the admin's closed table,
	// and isn't offered for Garapon linking.
	Status string `json:"status"`
	// PublicSignup opts the rally into self-service sign-up: it is then listed on the
	// public sign-up page and anyone may issue themselves a card. Off by default, so a
	// rally whose cards the staff hand out (via Garapon links or the admin card list)
	// stays invite-only unless someone deliberately opens it.
	PublicSignup bool `json:"public_signup"`
	// CompletionMode is "all" (collect the whole card) or "counts" (collect
	// RequiredFood food stamps + RequiredGame game stamps) - see the constants above.
	CompletionMode string `json:"completion_mode"`
	// RequiredFood/RequiredGame are the per-type stamp counts a card needs in
	// "counts" mode; both are ignored in "all" mode.
	RequiredFood int `json:"required_food"`
	RequiredGame int `json:"required_game"`
	// FestivalMapID optionally ties the rally to a Festival Map. When set, each
	// stamp names one of that map's stalls instead of a bare affiliate, and the
	// public map badges those stalls as part of the rally. nil = not linked.
	FestivalMapID   *int64 `json:"festival_map_id"`
	FestivalMapName string `json:"festival_map_name,omitempty"` // joined for display
	CreatedAt       string `json:"created_at"`

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
// a participant enters to collect it, its type (food or game - what a "counts"
// rally counts), its placement on the card, an optional active window (within the
// event window) and a manual pause toggle, and the stall it belongs to.
//
// The stall is either a Festival Map pitch OCCUPANT (OccupantID, when the rally is
// linked to a map) or a bare affiliate (AffiliateID, nil for the "Senpan Tea
// House" default). A stamp with an OccupantID takes its affiliate from that
// occupant, so AffiliateID is kept in step on save rather than being a second,
// disagreeing source of truth. It names the occupant rather than the pitch because
// a pitch that changes hands between days hosts two different stalls.
type StampRallyStamp struct {
	ID            int64  `json:"id"`
	RallyID       int64  `json:"rally_id"`
	OccupantID    *int64 `json:"occupant_id"`        // nil = not tied to a Festival Map occupant
	StallName     string `json:"stall_name"`         // joined from the occupant ("" when unlinked)
	AffiliateID   *int64 `json:"affiliate_id"`       // nil = Senpan Tea House (default)
	AffiliateName string `json:"affiliate_name"`     // joined for display ("" -> "Senpan Tea House")
	Image         string `json:"image"`              // images/stamp_stamps/...
	Password      string `json:"password,omitempty"` // omitted from public payloads
	StampType     string `json:"stamp_type"`         // "food" | "game" - see the constants above
	Placement     `json:"placement"`
	ActiveFrom    string `json:"active_from"` // UTC RFC-3339 within event window ("" = whole event)
	ActiveTo      string `json:"active_to"`   // UTC RFC-3339 ("" = whole event)
	Paused        bool   `json:"paused"`
	SortOrder     int    `json:"sort_order"`
}

// DisplayStall is the name to show (and log) for a stamp's stall: its Festival Map
// stall title when the rally is linked to a map, else its affiliate, else the
// owning venue. One place decides it so the card, the log and the public map can't
// drift apart.
func (s StampRallyStamp) DisplayStall() string {
	for _, name := range []string{s.StallName, s.AffiliateName} {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return DefaultStallName
}

// DefaultStallName is the stall shown for a stamp with neither a map stall nor an
// affiliate - the festival's own venue.
const DefaultStallName = "Senpan Tea House"

// StampRallyPrize is a reward revealed once a card completes: a name, an image, and
// its placement on the card. Before completion the card shows the not-stamped
// placeholder at the prize's slot instead.
type StampRallyPrize struct {
	ID        int64  `json:"id"`
	RallyID   int64  `json:"rally_id"`
	Name      string `json:"name"`
	Image     string `json:"image"` // images/stamp_prizes/...
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
// the time it was stamped. The (card_id, stamp_id) pair is unique - a stamp can't be
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
	StampType       string `json:"stamp_type"` // snapshotted alongside StallName
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
	StampType       string `json:"stamp_type"`
	StampedAt       string `json:"stamped_at"`
}

// StampRalliesResponse is the body of GET /api/stamp-rallies.
type StampRalliesResponse struct {
	StampRallies []StampRally `json:"stamp_rallies"`
}

// StampRallyResponse is the body of POST /api/stamp-rallies {action:"create"} - the
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
// {action:"create_card"} - the issued card (with token).
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
	// What finishing the card takes: "all" (every stamp) or "counts" (RequiredFood
	// food stamps + RequiredGame game stamps). Sent so the card can show progress
	// against the same rule the server applies.
	CompletionMode string `json:"completion_mode"`
	RequiredFood   int    `json:"required_food"`
	RequiredGame   int    `json:"required_game"`
}

// PublicStamp is one stamp slot in the participant-facing card. AffiliateName ""
// renders as "Senpan Tea House" on the frontend. Availability/collection are
// computed against the current time.
type PublicStamp struct {
	ID            int64  `json:"id"`
	AffiliateName string `json:"affiliate_name"`
	StampType     string `json:"stamp_type"` // "food" | "game"
	Image         string `json:"image"`
	Placement     `json:"placement"`
	ActiveFrom    string `json:"active_from"`
	ActiveTo      string `json:"active_to"`
	Available     bool   `json:"available"`
	// Expired reports that the stamp can never be collected again (its own window
	// ended, or the event did), as opposed to merely closed right now. The card uses
	// it to tell a participant when a per-type requirement has gone out of reach,
	// which is otherwise indistinguishable from a stall that reopens later.
	Expired     bool   `json:"expired"`
	Collected   bool   `json:"collected"`
	CollectedAt string `json:"collected_at"`
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

// -- Public self-service sign-up ----------------------------------------------

// SignupRally is one rally on the public sign-up page: enough to describe the
// event and set expectations, with no passwords, no card list and no participant
// names. GaraponTitle is set when an open Garapon is linked to the rally, in which
// case signing up also issues a drawing link for it.
type SignupRally struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	CardImage     string `json:"card_image"`
	Details       string `json:"details"` // markdown
	AvailableFrom string `json:"available_from"`
	AvailableTo   string `json:"available_to"`
	GaraponTitle  string `json:"garapon_title,omitempty"`
}

// SignupRalliesResponse is the body of GET /api/stamp-signup.
type SignupRalliesResponse struct {
	Rallies []SignupRally `json:"rallies"`
}

// StampSignupResponse is the body of POST /api/stamp-signup/{id}: the tokens the
// participant needs, which the client turns into links. GaraponToken is "" when the
// rally has no open linked Garapon; when it is set it equals CardToken, since a
// paired drawing link and stamp card share one token.
type StampSignupResponse struct {
	ParticipantName string `json:"participant_name"`
	RallyTitle      string `json:"rally_title"`
	CardToken       string `json:"card_token"`
	GaraponToken    string `json:"garapon_token,omitempty"`
	GaraponTitle    string `json:"garapon_title,omitempty"`
}

// StampLookupEntry is one rally a looked-up participant holds a card for.
type StampLookupEntry struct {
	RallyID      int64  `json:"rally_id"`
	RallyTitle   string `json:"rally_title"`
	CardToken    string `json:"card_token"`
	GaraponToken string `json:"garapon_token,omitempty"`
	GaraponTitle string `json:"garapon_title,omitempty"`
	Completed    bool   `json:"completed"`
}

// StampLookupResponse is the body of POST /api/stamp-lookup. An unknown name and a
// name with no cards return the same empty list - the endpoint never distinguishes
// them, so it can't be used to probe who signed up.
type StampLookupResponse struct {
	Entries []StampLookupEntry `json:"entries"`
}
