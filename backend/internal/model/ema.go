package model

// -- Ema Plaques --------------------------------------------------------------
//
// Ema are the small wooden plaques hung at a shrine with a wish or a prayer on
// them. At a festival, a visitor writes one of three ways: designs it off the app
// from the template and posts it to Discord themselves, designs it on the site
// (and staff approve it before the app posts it to Discord for them), or asks the
// resident artist to make it. A board belongs to one festival (a FestivalMap) and
// takes plaques in windows - usually one per festival day - and each person gets
// ONE plaque per window, whichever way they made it.

// Ema plaque kinds - how a plaque was made. Stored in ema_plaques.kind.
const (
	// EmaKindSite - designed on the site and sent for posting.
	EmaKindSite = "site"
	// EmaKindArtist - requested from the board's artist, who makes and posts it.
	EmaKindArtist = "artist"
)

// Ema plaque states. A site plaque goes pending -> posting -> posted (or back to
// pending when the post fails, or to rejected); an artist request goes
// requested -> complete (and may be reopened). "posting" is held only while an
// approval's Discord post is in flight - it is how a second approval of the same
// plaque is refused rather than posted twice.
const (
	EmaStatusPending   = "pending"
	EmaStatusPosting   = "posting"
	EmaStatusPosted    = "posted"
	EmaStatusRejected  = "rejected"
	EmaStatusRequested = "requested"
	EmaStatusComplete  = "complete"
)

// EmaBoard is one festival's plaque board: the template and the words around it,
// which of the three ways to write a plaque are offered, and its windows.
type EmaBoard struct {
	ID              int64  `json:"id"`
	FestivalMapID   int64  `json:"festival_map_id"`
	FestivalMapName string `json:"festival_map_name,omitempty"` // joined for display
	Title           string `json:"title"`
	// TemplateImage is the blank plaque everyone starts from (images/...).
	TemplateImage string `json:"template_image"`
	// Instructions is markdown shown with the do-it-yourself option: how to edit the
	// template and where to post it.
	Instructions string `json:"instructions"`
	// DiscordLink is where self-made plaques are posted (a channel or invite URL).
	DiscordLink string `json:"discord_link"`
	AllowSelf   bool   `json:"allow_self"`
	AllowSite   bool   `json:"allow_site"`
	AllowArtist bool   `json:"allow_artist"`
	// ArtistName names the resident artist in the request option ("Tara").
	ArtistName string `json:"artist_name"`
	Status     string `json:"status"` // "open" | "closed"
	CreatedAt  string `json:"created_at"`

	// Windows is populated on detail fetches only.
	Windows []EmaWindow `json:"windows,omitempty"`

	// Read-only aggregates for the admin list view.
	PendingCount   int `json:"pending_count,omitempty"`
	RequestedCount int `json:"requested_count,omitempty"`
}

// EmaWindow is one stretch of the festival a plaque is taken for - usually a day -
// with the Discord webhook its approved site plaques are posted to. The webhook is
// a secret: it is returned to staff who manage the board and nowhere else.
type EmaWindow struct {
	ID         int64  `json:"id"`
	BoardID    int64  `json:"board_id"`
	Label      string `json:"label"`       // "Day 1"
	StartAt    string `json:"start_at"`    // UTC RFC-3339
	EndAt      string `json:"end_at"`      // UTC RFC-3339
	WebhookURL string `json:"webhook_url"` // staff only
	SortOrder  int    `json:"sort_order"`
}

// EmaPlaque is one visitor's plaque: designed on the site (with its image, until it
// has been posted or rejected - the file is deleted then, since Discord holds the
// posted copy) or requested from the artist (with the description they gave).
type EmaPlaque struct {
	ID            int64  `json:"id"`
	BoardID       int64  `json:"board_id"`
	WindowID      *int64 `json:"window_id"`
	WindowLabel   string `json:"window_label"` // snapshotted, so the record survives an edited window
	Kind          string `json:"kind"`         // see the EmaKind* constants
	CharacterName string `json:"character_name"`
	World         string `json:"world"`
	Description   string `json:"description"` // artist requests
	// Image is the site plaque's PNG (images/ema_plaques/...) while it waits for
	// review; "" once posted or rejected.
	Image      string `json:"image"`
	Status     string `json:"status"` // see the EmaStatus* constants
	ReviewedBy string `json:"reviewed_by"`
	ReviewedAt string `json:"reviewed_at"`
	CreatedAt  string `json:"created_at"`
	// FestivalMapName is joined for the artist's queue, which spans festivals.
	FestivalMapName string `json:"festival_map_name,omitempty"`
}

// EmaBoardsResponse is the body of GET /api/ema-boards.
type EmaBoardsResponse struct {
	Boards []EmaBoard `json:"boards"`
}

// EmaBoardResponse wraps one board - the body of a create, and of the detail read.
type EmaBoardResponse struct {
	Board EmaBoard `json:"board"`
}

// EmaPlaquesResponse is the body of the plaque lists (a board's, or the artist's
// queue).
type EmaPlaquesResponse struct {
	Plaques []EmaPlaque `json:"plaques"`
}

// EmaPlaqueResponse wraps one plaque - the body of a submission and of a review.
type EmaPlaqueResponse struct {
	Plaque EmaPlaque `json:"plaque"`
}

// GoogleFontsResponse is the body of GET /api/google-fonts: the Google Fonts
// family names, most popular first - read with the server's own API key so the
// key never reaches a browser. Empty when no key is configured.
type GoogleFontsResponse struct {
	Families []string `json:"families"`
}

// -- Public (the festival dashboard's Ema section) ------------------------------

// PublicEmaWindow is a window as a visitor sees it: no webhook.
type PublicEmaWindow struct {
	ID      int64  `json:"id"`
	Label   string `json:"label"`
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
	IsOpen  bool   `json:"is_open"`
}

// PublicEmaPlaque is one of the visitor's own plaques. It carries the image only
// while the plaque is waiting for review - the visitor's own upload, shown back to
// them - and no reviewer name.
type PublicEmaPlaque struct {
	ID          int64  `json:"id"`
	WindowID    *int64 `json:"window_id"`
	WindowLabel string `json:"window_label"`
	Kind        string `json:"kind"`
	Status      string `json:"status"`
	Description string `json:"description"`
	Image       string `json:"image"`
	CreatedAt   string `json:"created_at"`
}

// FestivalDashboardEma is the festival's plaque board on the dashboard, with the
// visitor's own plaques. CurrentWindowID is the window taking plaques right now
// (nil = none); HasPlaqueThisWindow says the visitor already has one in it, which
// is the one-per-window rule the submit endpoints enforce.
type FestivalDashboardEma struct {
	ID                  int64             `json:"id"`
	Title               string            `json:"title"`
	TemplateImage       string            `json:"template_image"`
	Instructions        string            `json:"instructions"`
	DiscordLink         string            `json:"discord_link"`
	AllowSelf           bool              `json:"allow_self"`
	AllowSite           bool              `json:"allow_site"`
	AllowArtist         bool              `json:"allow_artist"`
	ArtistName          string            `json:"artist_name"`
	Windows             []PublicEmaWindow `json:"windows"`
	CurrentWindowID     *int64            `json:"current_window_id"`
	HasPlaqueThisWindow bool              `json:"has_plaque_this_window"`
	Plaques             []PublicEmaPlaque `json:"plaques"`
}
