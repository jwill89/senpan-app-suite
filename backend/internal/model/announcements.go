package model

// AnnouncementType is a named Discord destination for announcements: a friendly
// label plus the channel its announcements post to. Each announcement references
// one type, so the list can be filtered by where things go.
type AnnouncementType struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// ChannelID is the Discord channel the bot posts this type's announcements to
	// ("" = none picked yet; its announcements wait until one is).
	ChannelID string `json:"channel_id"`
	CreatedAt string `json:"created_at"`
}

// Announcement is an admin-authored message the bot posts to Discord as an embed in
// its type's channel - manually ("send now") or automatically on a schedule.
//
// One IANA Timezone anchors every time on the announcement: the admin enters
// wall-clock values (StartLocal/EndLocal for the event window, OnceLocal for a
// one-time post, and the recurring time-of-day/weekday selections) and the
// backend resolves them in Timezone with time.LoadLocation. The absolute instants
// (StartAt/EndAt/NextPostAt, UTC RFC-3339) are computed server-side and drive the
// embed timestamps + the scheduler. Because the zone is explicit, all times stay
// put across DST transitions (e.g. "every Saturday 7pm America/New_York").
type Announcement struct {
	ID         int64  `json:"id"`
	TypeID     int64  `json:"type_id"`
	Title      string `json:"title"`
	Details    string `json:"details"`     // markdown
	Image      string `json:"image"`       // full URL, shown full-width at the bottom of the embed
	Thumbnail  string `json:"thumbnail"`   // full URL, shown as the small top-right embed thumbnail
	Color      string `json:"color"`       // embed accent colour, "#rrggbb" ("" = brand default)
	Location   string `json:"location"`    // optional free-text location (e.g. a Discord voice channel)
	StartLocal string `json:"start_local"` // optional event start, wall-clock "2006-01-02T15:04" in Timezone
	EndLocal   string `json:"end_local"`   // optional event end, wall-clock in Timezone
	StartAt    string `json:"start_at"`    // computed event start, UTC RFC-3339
	EndAt      string `json:"end_at"`      // computed event end, UTC RFC-3339
	// Discord timestamp styles (t|T|d|D|f|F|R) for how the start/end render in the
	// embed; each is chosen independently ("" = default - F for start, t for end).
	StartFormat string `json:"start_format"`
	EndFormat   string `json:"end_format"`
	// DynamicDates re-anchors the start/end onto the day the announcement is posted
	// (preserving the time-of-day and how many days the end runs past the start),
	// so a recurring "day-of" event post always shows the current occurrence rather
	// than the first one. StartLocal/EndLocal define the template time-of-day.
	DynamicDates bool `json:"dynamic_dates"`

	// Schedule (all optional; ScheduleKind == "" means unscheduled - manual only).
	// All recurring/one-time times are wall-clock values anchored to Timezone.
	ScheduleKind        string `json:"schedule_kind"`          // ""|once|daily|weekly|monthly
	Timezone            string `json:"timezone"`               // IANA zone anchoring every time on the announcement
	OnceLocal           string `json:"once_local"`             // wall-clock "2006-01-02T15:04" in Timezone (one-time schedule)
	ScheduleMinutes     int    `json:"schedule_minutes"`       // local minutes-of-day in Timezone (recurring)
	ScheduleWeekdays    string `json:"schedule_weekdays"`      // CSV of local weekdays 0=Sun..6=Sat (weekly; first value reused for monthly)
	ScheduleWeekOfMonth int    `json:"schedule_week_of_month"` // 1..5 or -1=last (monthly)

	NextPostAt string `json:"next_post_at"` // next scheduled instant, UTC RFC-3339 ("" = none)
	// SkipCount is how many upcoming occurrences to pass over before posting again.
	// Each skipped occurrence decrements it, so a Friday+Saturday announcement can
	// be told once to skip 2 and sit out a whole weekend. 0 = post as scheduled.
	SkipCount    int    `json:"skip_count"`
	Active       bool   `json:"active"`         // whether the schedule is live
	LastPostedAt string `json:"last_posted_at"` // ISO timestamp of last post ("" if never)
	CreatedAt    string `json:"created_at"`

	// Optional Discord link buttons (up to 5) rendered as a single action row
	// below the embed. Persisted as a JSON array in the announcements.buttons column.
	Buttons []AnnouncementButton `json:"buttons"`

	// Optional role tag posted in the message content (above the embed), since
	// mentions inside an embed don't notify. One of: "" (don't tag), "everyone"
	// (@everyone), or "role:<role id>" (a Discord role in the server).
	Mention string `json:"mention"`
	// RepostMode is "new" (leave earlier posts up) or "replace" (delete the previous
	// post once the new one is up). Announcements never edit in place: an edit
	// notifies nobody, and notifying is what a recurring announcement is for.
	RepostMode string `json:"repost_mode"`
	// DiscordURL links to the newest post still up in Discord ("" when there is none).
	// Read-only; filled on the admin list.
	DiscordURL string `json:"discord_url,omitempty"`

	// Read-only convenience for list rendering (joined from announcement_types).
	TypeName string `json:"type_name,omitempty"`
}

// AnnouncementButton is one Discord link button shown beneath an announcement's
// embed: a label, an optional emoji (a unicode emoji like "🎉" or a custom-emoji
// token "<:name:id>"/"<a:name:id>"), and the URL it opens.
type AnnouncementButton struct {
	Label string `json:"label"`
	Emoji string `json:"emoji"`
	URL   string `json:"url"`
}

// AnnouncementTypesResponse is the body of GET /api/announcement-types - all
// announcement types. JSON: {"types": [...]}.
type AnnouncementTypesResponse struct {
	Types []AnnouncementType `json:"types"`
}

// AnnouncementTypeResponse is the body of POST /api/announcement-types (create)
// and PUT /api/announcement-types/{id} (update) - the saved type. The handler
// passes the *AnnouncementType from GetAnnouncementType through, so the pointer is
// preserved (a nil serializes to null, matching the old map literal). JSON:
// {"type": ...}.
type AnnouncementTypeResponse struct {
	Type *AnnouncementType `json:"type"`
}

// AnnouncementsResponse is the body of GET /api/announcements - all announcements
// (with type_name joined). JSON: {"announcements": [...]}.
type AnnouncementsResponse struct {
	Announcements []Announcement `json:"announcements"`
}

// AnnouncementResponse is the body of POST /api/announcements
// {action:"create"|"update"|"send_now"|"skip_next"} - the saved announcement.
// The handler passes the *Announcement from GetAnnouncement through, so the
// pointer is preserved (nil -> null, matching the old map literal). JSON:
// {"announcement": ...}.
type AnnouncementResponse struct {
	Announcement *Announcement `json:"announcement"`
	// Note says something worth knowing about a send (a dry run, the test channel).
	Note string `json:"note,omitempty"`
}
