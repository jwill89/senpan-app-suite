package model

// TeaRoom is a bookable room listed in the Senpan Tea House → Tea Rooms admin
// section: a named, numbered room with a per-half-hour gil cost, hashtags, a
// markdown description, a handful of status flags (seasonal, open, lockable,
// discounted), an image picked from the shared image library, and a Discord embed
// accent colour. Admins manage a drag-orderable list of them and post each as a
// Discord embed to a single shared webhook (see the tea-room webhook setting).
//
// The list is also exposed read-only through a public, cross-origin API so an
// external Carrd site can render live room availability and pricing.
type TeaRoom struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`     // room name (embed title)
	// Subtitle is a short secondary line under the name (e.g. a Japanese phrase);
	// stored as UTF-8, so it holds any script natively.
	Subtitle string `json:"subtitle"`
	// RoomNumber is the room's public identifier: required and unique, and the key
	// the public API + Carrd embed look rooms up by (e.g. "12", "West 3").
	RoomNumber string `json:"room_number"`
	// CostPerHalfHour is the price in gil per half hour. When Discounted is set the
	// effective price is halved (a fixed 50% rule); the raw full price is stored here.
	CostPerHalfHour int64  `json:"cost_per_half_hour"`
	Hashtags        string `json:"hashtags"`    // normalized space-separated "#tag" list
	Description     string `json:"description"` // markdown (embed body)
	Seasonal        bool   `json:"seasonal"`    // seasonal vs permanent
	Open            bool   `json:"open"`        // open vs closed
	Lockable        bool   `json:"lockable"`    // can be locked for an extra fee
	Discounted      bool   `json:"discounted"`  // currently 50%-off
	Image           string `json:"image"`       // full URL, shown full-width in the embed
	Color           string `json:"color"`       // embed accent colour, "#rrggbb" ("" = brand default)
	SortOrder       int    `json:"sort_order"`
	CreatedAt       string `json:"created_at"`
}

// TeaRoomsResponse is the body of GET /api/tea-rooms — every room in the admin's
// chosen order, plus the shared Discord webhook (returned only to callers who can
// manage the feature; never exposed on the public API). JSON: {"tea_rooms": [...],
// "webhook_url": "..."}.
type TeaRoomsResponse struct {
	TeaRooms   []TeaRoom `json:"tea_rooms"`
	WebhookURL string    `json:"webhook_url"`
}

// TeaRoomResponse is the body of POST /api/tea-rooms (create), PUT /api/tea-rooms/
// {id} (replace), PATCH /api/tea-rooms/{id} (toggle), and POST /api/tea-rooms/{id}/
// post (post to Discord) — the saved room. The handler passes the *TeaRoom from
// GetTeaRoom through, so the pointer is preserved (nil → null). JSON: {"tea_room": …}.
type TeaRoomResponse struct {
	TeaRoom *TeaRoom `json:"tea_room"`
}

// TeaRoomWebhookResponse is the body of PUT /api/tea-rooms/webhook — the saved
// shared Discord webhook. JSON: {"webhook_url": "..."}.
type TeaRoomWebhookResponse struct {
	WebhookURL string `json:"webhook_url"`
}

// TeaRoomsPublicResponse is the body of the public GET /api/tea-rooms/public — the
// full room list for an external site (no webhook). JSON: {"tea_rooms": [...]}.
type TeaRoomsPublicResponse struct {
	TeaRooms []TeaRoom `json:"tea_rooms"`
}

// TeaRoomPublicResponse is the body of the public GET /api/tea-rooms/public/{id} —
// one room with all its data + status flags. JSON: {"tea_room": …}.
type TeaRoomPublicResponse struct {
	TeaRoom *TeaRoom `json:"tea_room"`
}
