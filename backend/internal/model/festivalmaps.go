package model

// -- Festival Map -------------------------------------------------------------
//
// A Festival Map is the interactive floor plan of a festival: a base image (the
// venue outline - walls, rest areas, the stage) with the stalls drawn ON TOP of
// it by the app, each a shape at a %-based Placement carrying its own title,
// markdown description, opening times and owning affiliate. Because the shapes
// and their labels are rendered rather than baked into the artwork, renaming a
// stall or moving it never means re-exporting the map.
//
// A map may also carry an optional SHORTCODE (Slug) - "obon-2026" - so it can be
// linked as /festival-maps/obon-2026 instead of by its numeric id. Both forms
// always resolve, so adding or changing a shortcode never breaks a link that has
// already been posted somewhere.

// Festival map publish states. Stored in festival_maps.status.
const (
	// MapStatusInProgress - being authored. Admin-only; never listed publicly.
	MapStatusInProgress = "in_progress"
	// MapStatusPublished - live on the public festival map pages.
	MapStatusPublished = "published"
	// MapStatusClosed - the festival is over. Admin-only again, but kept so the
	// map (and any rally that references its stalls) survives as a record.
	MapStatusClosed = "closed"
)

// NormalizeMapStatus maps a stored/received status onto a known one, falling back
// to MapStatusInProgress - the safe default, since it is the one state that is
// never published to the public pages.
func NormalizeMapStatus(status string) string {
	switch status {
	case MapStatusPublished:
		return MapStatusPublished
	case MapStatusClosed:
		return MapStatusClosed
	default:
		return MapStatusInProgress
	}
}

// Stall shapes - how a stall is drawn on the map. Stored in festival_stalls.shape.
const (
	// StallShapeCircle - an ellipse filling the placement box (the game-stall look).
	StallShapeCircle = "circle"
	// StallShapeRect - a rectangle filling the placement box (the food-stall look).
	StallShapeRect = "rect"
)

// NormalizeStallShape maps a stored/received shape onto a known one, falling back
// to a rectangle (the shape that reads correctly at any aspect ratio).
func NormalizeStallShape(shape string) string {
	if shape == StallShapeCircle {
		return StallShapeCircle
	}
	return StallShapeRect
}

// Stall types - what a stall offers. Stored in festival_stalls.stall_type. The
// type seeds the stall's default shape and color in the editor (a game stall is
// drawn as a circle, a food stall as a rectangle) and, when a Stamp Rally is
// linked to the map, decides whether that stall's stamp counts as food or game.
const (
	// StallTypeGame - games, activities, fortunes: anything that isn't served.
	StallTypeGame = "game"
	// StallTypeFood - a food stall.
	StallTypeFood = "food"
	// StallTypeBoth - a stall that serves food AND runs a game.
	StallTypeBoth = "both"
	// StallTypeOther - everything else on the plan that isn't a stall proper
	// (a front desk, a redemption table, an art raffle booth).
	StallTypeOther = "other"
)

// NormalizeStallType maps a stored/received stall type onto a known one, falling
// back to StallTypeOther - the type that claims nothing about what the stall does.
func NormalizeStallType(t string) string {
	switch t {
	case StallTypeGame:
		return StallTypeGame
	case StallTypeFood:
		return StallTypeFood
	case StallTypeBoth:
		return StallTypeBoth
	default:
		return StallTypeOther
	}
}

// EventTime is one datetime range on a festival map or one of its stalls: an
// optional label ("Day 1", "Opening Ceremony"), a required start and an optional
// end. Both are UTC RFC-3339, matching every other stored time in the app, and
// the whole list is persisted as a JSON column (like AffiliateHour) since it is
// small, always loaded with its row, and edited as a set.
type EventTime struct {
	Label string `json:"label"` // optional descriptor, e.g. "Day 1"
	Start string `json:"start"` // UTC RFC-3339 (required)
	End   string `json:"end"`   // UTC RFC-3339 ("" = no stated end)
}

// FestivalMap is the event: a titled floor plan with a markdown description, the
// datetimes it runs across, the base map image every stall is placed on, and a
// publish status. Stalls are loaded on detail fetches only.
type FestivalMap struct {
	ID          int64       `json:"id"`
	Title       string      `json:"title"`
	Slug        string      `json:"slug"`        // optional shortcode ("" = reachable by id only)
	Description string      `json:"description"` // markdown
	Times       []EventTime `json:"times"`       // when the festival runs (may be several)
	MapImage    string      `json:"map_image"`   // images/... the base floor plan
	Status      string      `json:"status"`      // see the MapStatus* constants
	CreatedAt   string      `json:"created_at"`

	// Populated on detail fetches only (omitted from list responses).
	Stalls []FestivalStall `json:"stalls,omitempty"`

	// Read-only aggregate for the admin list view; omitted (zero) elsewhere.
	StallCount int `json:"stall_count,omitempty"`
}

// FestivalStall is one PITCH on the map - a place on the floor plan, not the
// business standing in it: how it is drawn (shape + color + Placement) and who
// occupies it. A pitch always has at least one occupant; it has several when a
// booth changes hands between days, which is the norm at a multi-day festival
// (Flora Teahouse runs it on Day 1, The Great Below on Day 2).
//
// Splitting the two apart is what keeps the plan readable: the pitch is drawn
// once, in one place, whatever the week's rota, instead of two stalls stacked on
// the same coordinates fighting for the same pixels.
type FestivalStall struct {
	ID    int64  `json:"id"`
	MapID int64  `json:"map_id"`
	Shape string `json:"shape"` // "circle" | "rect"
	Color string `json:"color"` // "#rrggbb" ("" = the first occupant's type default)
	// TextColor is the colour this pitch's label is drawn in. Its own setting
	// beside Color, because a label that reads on a pale fill disappears on a deep
	// one. "" = white, which is what most fills want.
	TextColor string `json:"text_color"`
	// SelectionColor is the halo drawn around this pitch when a visitor taps it.
	// Its own colour rather than Color, so the ring can be made to contrast with
	// the fill instead of vanishing into it. "" = the app's default highlight.
	SelectionColor string `json:"selection_color"`
	Placement      `json:"placement"`
	SortOrder      int `json:"sort_order"`

	// Occupants is who stands here, in display order. Never empty on a stored
	// pitch - a pitch nobody occupies is a shape with nothing to say, and is
	// dropped on save.
	Occupants []FestivalStallOccupant `json:"occupants"`
}

// FestivalStallOccupant is one business standing in a pitch: the affiliate
// running it (nil = the owning venue, Senpan Tea House), its title and markdown
// description, what it offers, and the hours it keeps - which is how a pitch with
// several occupants decides who is on when. An occupant with no times of its own
// runs for the whole festival.
type FestivalStallOccupant struct {
	ID            int64  `json:"id"`
	StallID       int64  `json:"stall_id"`
	AffiliateID   *int64 `json:"affiliate_id"`   // nil = Senpan Tea House (default)
	AffiliateName string `json:"affiliate_name"` // joined for display ("" -> "Senpan Tea House")
	// Title is OPTIONAL. The map labels a pitch by its AFFILIATE; a title only
	// adds detail alongside it in the info panel.
	Title       string `json:"title"`
	Description string `json:"description"` // markdown
	// EventCarrd is this occupant's own event page. Per occupant, not per pitch: a
	// pitch that changes hands between days has a different page each day. "" hides
	// the button rather than falling back to the affiliate's own link, which sits
	// beside it and would otherwise be duplicated.
	EventCarrd string `json:"event_carrd"`
	StallType  string `json:"stall_type"` // see the StallType* constants
	// TypeLabel is the caption drawn under the title when StallType is "other" -
	// "Omikuji", "Art Raffle", "Food & Fortunes". It is kept whatever the type, so
	// flipping to a named type and back doesn't lose the wording, but only an
	// "other" occupant renders it.
	TypeLabel string      `json:"type_label"`
	Times     []EventTime `json:"times"` // when this occupant is here ([] = the whole festival)
	SortOrder int         `json:"sort_order"`
}

// FestivalMapsResponse is the body of GET /api/festival-maps.
type FestivalMapsResponse struct {
	Maps []FestivalMap `json:"maps"`
}

// FestivalMapResponse is the body of POST /api/festival-maps - the freshly
// created map echoed back.
type FestivalMapResponse struct {
	Map FestivalMap `json:"map"`
}

// FestivalMapDetailResponse is the body of GET /api/festival-maps/{id}: the map
// with its stalls.
type FestivalMapDetailResponse struct {
	Map FestivalMap `json:"map"`
}

// -- Public payloads ----------------------------------------------------------

// PublicFestivalMapSummary is one map on the public list page: enough to describe
// the festival and link to it, with no stalls.
type PublicFestivalMapSummary struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	// Slug is the map's shortcode, or "" when it has none. The list links each
	// map by it when set, so a visitor lands on the readable URL.
	Slug        string      `json:"slug"`
	Description string      `json:"description"` // markdown
	Times       []EventTime `json:"times"`
	MapImage    string      `json:"map_image"`
	StallCount  int         `json:"stall_count"`
	// IsActive reports that now falls inside one of the map's datetime ranges -
	// the festival is happening right now, not merely published.
	IsActive bool `json:"is_active"`
}

// PublicFestivalMapsResponse is the body of GET /api/festival-maps/public.
type PublicFestivalMapsResponse struct {
	Maps []PublicFestivalMapSummary `json:"maps"`
}

// PublicStallAffiliate is the affiliate card shown when a visitor opens a stall:
// who runs it and where to find them. Deliberately a subset of Affiliate - the
// Discord webhook, embed color and admin sort order stay out of a public payload.
type PublicStallAffiliate struct {
	Name        string   `json:"name"`
	Subtitle    string   `json:"subtitle"`
	Owners      []string `json:"owners"`
	Logo        string   `json:"logo"`
	DiscordLink string   `json:"discord_link"`
	CarrdLink   string   `json:"carrd_link"`
}

// PublicStallOccupant is one business standing in a pitch, as a visitor sees it.
// It carries the stamp-rally link when THIS occupant's stamp belongs to a rally
// that is open right now - the badge follows the occupant, not the pitch, since
// Flora's game stamp isn't The Great Below's.
type PublicStallOccupant struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"` // optional; the map labels by affiliate
	Description string `json:"description"`
	// EventCarrd is this occupant's own event page ("" = none; no fallback).
	EventCarrd string      `json:"event_carrd,omitempty"`
	StallType  string      `json:"stall_type"`
	TypeLabel  string      `json:"type_label"`
	Times      []EventTime `json:"times"`
	// IsOpen reports that now falls inside one of this occupant's time ranges (or
	// inside the festival's, when it keeps no separate hours).
	IsOpen bool `json:"is_open"`
	// Affiliate is nil for a pitch run by the venue itself.
	Affiliate *PublicStallAffiliate `json:"affiliate,omitempty"`
	// InStampRally is set when this occupant carries a stamp on a currently-open
	// rally; RallyTitle/StampImage describe it for the detail panel.
	InStampRally bool   `json:"in_stamp_rally"`
	RallyID      int64  `json:"rally_id,omitempty"`
	RallyTitle   string `json:"rally_title,omitempty"`
	StampImage   string `json:"stamp_image,omitempty"`
	// Raffle is set when a raffle assigned to this occupant is RUNNING RIGHT NOW -
	// open and inside its availability window. A raffle that has closed, or whose
	// window hasn't opened yet, is left off rather than pointing a visitor at a
	// page they can't enter.
	Raffle *PublicStallRaffle `json:"raffle,omitempty"`
}

// PublicStallRaffle is the raffle running at a stall, as the map's detail panel
// shows it: enough to name it and link to /raffles/{id}.
type PublicStallRaffle struct {
	ID         int64  `json:"id"`
	Title      string `json:"title"`
	PrizeImage string `json:"prize_image"`
}

// PublicFestivalStall is one pitch in the public map view: where it sits, how it
// is drawn, and everyone who occupies it. The map draws the pitch once and picks
// which occupant's name to lead with (see Occupants) rather than stacking two
// stalls on the same coordinates.
type PublicFestivalStall struct {
	ID             int64  `json:"id"`
	Shape          string `json:"shape"`
	Color          string `json:"color"`
	TextColor      string `json:"text_color"`
	SelectionColor string `json:"selection_color"`
	Placement      `json:"placement"`
	// Occupants is who stands here, in display order - one entry for a pitch that
	// keeps the same business all festival, several when it changes hands by day.
	Occupants []PublicStallOccupant `json:"occupants"`
}

// PublicFestivalMap is the full visitor-facing map view: the festival, its base
// image, and every stall on it.
type PublicFestivalMap struct {
	ID          int64                 `json:"id"`
	Title       string                `json:"title"`
	Slug        string                `json:"slug"`
	Description string                `json:"description"` // markdown
	Times       []EventTime           `json:"times"`
	MapImage    string                `json:"map_image"`
	IsActive    bool                  `json:"is_active"`
	Stalls      []PublicFestivalStall `json:"stalls"`
	// StampRallyTitle names the open rally whose stamps sit on this map ("" when
	// none), so the view can label its "show only stamp rally stalls" filter.
	StampRallyTitle string `json:"stamp_rally_title,omitempty"`
	// StampRallySignup is true when that rally takes public sign-ups, so the map
	// can send a visitor straight to its sign-up page.
	StampRallySignup bool  `json:"stamp_rally_signup,omitempty"`
	StampRallyID     int64 `json:"stamp_rally_id,omitempty"`
}
