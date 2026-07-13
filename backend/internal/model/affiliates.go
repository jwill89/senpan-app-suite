package model

// Affiliate is a partner establishment listed in the Senpan Tea House → Affiliates
// admin section: a named place with one or more owners, a location, opening hours
// (multiple time ranges sharing one timezone), markdown details, and two pictures
// picked from the shared image library — a Logo and an establishment
// Screenshot. Owners and Hours are persisted as JSON columns (no sub-tables).
type Affiliate struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`         // establishment name
	Owners      []string        `json:"owners"`       // one or more owner names
	Location    string          `json:"location"`     // free-text location
	Timezone    string          `json:"timezone"`     // IANA zone anchoring every Hours entry
	Hours       []AffiliateHour `json:"hours"`        // open time ranges (wall-clock in Timezone)
	Details     string          `json:"details"`      // markdown
	Logo        string          `json:"logo"`         // root-relative web path (images/affiliate_logos/…)
	Screenshot  string          `json:"screenshot"`   // root-relative web path (images/affiliate_images/…)
	EmbedColor  string          `json:"embed_color"`  // Discord embed accent, "#rrggbb" ("" = brand default)
	DiscordLink string          `json:"discord_link"` // optional Discord invite/link
	CarrdLink   string          `json:"carrd_link"`   // optional Carrd (or other) site link
	SortOrder   int             `json:"sort_order"`   // manual drag order (ascending)
	CreatedAt   string          `json:"created_at"`
}

// AffiliateHour is one opening-hours entry on an affiliate: an optional label
// (e.g. "Mon–Fri" or "Weekends"), a required start time, and an optional end time.
// Times are wall-clock "HH:MM" values interpreted in the affiliate's Timezone.
type AffiliateHour struct {
	Label string `json:"label"` // optional descriptor, e.g. "Mon–Fri"
	Start string `json:"start"` // wall-clock "HH:MM"
	End   string `json:"end"`   // optional wall-clock "HH:MM"
}

// AffiliatesResponse is the body of GET /api/affiliates. It also carries the
// single shared Discord webhook every affiliate posts to — safe to include here
// because the endpoint is permission-gated (it is kept out of public settings).
type AffiliatesResponse struct {
	Affiliates []Affiliate `json:"affiliates"`
	WebhookURL string      `json:"webhook_url"`
}

// AffiliateResponse is the body of POST /api/affiliates (create) and
// POST /api/affiliates/{id}/post when a single affiliate is echoed back.
type AffiliateResponse struct {
	Affiliate Affiliate `json:"affiliate"`
}

// AffiliateWebhookResponse is the body of PUT /api/affiliates/webhook.
type AffiliateWebhookResponse struct {
	WebhookURL string `json:"webhook_url"`
}
