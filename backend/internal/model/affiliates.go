package model

// Affiliate is a partner establishment listed in the Senpan Tea House → Affiliates
// admin section: a named place with one or more owners, a location, opening hours
// (multiple time ranges sharing one timezone), markdown details, and two pictures
// picked from dedicated permanent image categories — a Logo and an establishment
// Screenshot. Owners and Hours are persisted as JSON columns (no sub-tables).
type Affiliate struct {
	ID         int64           `json:"id"`
	Name       string          `json:"name"`       // establishment name
	Owners     []string        `json:"owners"`     // one or more owner names
	Location   string          `json:"location"`   // free-text location
	Timezone   string          `json:"timezone"`   // IANA zone anchoring every Hours entry
	Hours      []AffiliateHour `json:"hours"`      // open time ranges (wall-clock in Timezone)
	Details    string          `json:"details"`    // markdown
	Logo       string          `json:"logo"`       // root-relative web path (images/affiliate_logos/…)
	Screenshot string          `json:"screenshot"` // root-relative web path (images/affiliate_images/…)
	CreatedAt  string          `json:"created_at"`
}

// AffiliateHour is one opening-hours entry on an affiliate: an optional label
// (e.g. "Mon–Fri" or "Weekends"), a required start time, and an optional end time.
// Times are wall-clock "HH:MM" values interpreted in the affiliate's Timezone.
type AffiliateHour struct {
	Label string `json:"label"` // optional descriptor, e.g. "Mon–Fri"
	Start string `json:"start"` // wall-clock "HH:MM"
	End   string `json:"end"`   // optional wall-clock "HH:MM"
}

// AffiliatesResponse is the body of GET /api/affiliates.
type AffiliatesResponse struct {
	Affiliates []Affiliate `json:"affiliates"`
}

// AffiliateResponse is the body of POST /api/affiliates (create) when a single
// affiliate is echoed back.
type AffiliateResponse struct {
	Affiliate Affiliate `json:"affiliate"`
}
