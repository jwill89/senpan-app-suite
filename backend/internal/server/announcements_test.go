package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"app-suite/internal/model"
	"app-suite/internal/store"
)

// mustParse parses a UTC RFC-3339 instant or fails the test.
func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v.UTC()
}

func TestNextOccurrenceDaily(t *testing.T) {
	a := model.Announcement{ScheduleKind: "daily", Timezone: "UTC", ScheduleMinutes: 19*60 + 30} // 19:30 UTC
	// Before today's time → fires today.
	got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-13T10:00:00Z"))
	if got != "2026-06-13T19:30:00Z" {
		t.Errorf("daily (before): got %q", got)
	}
	// After today's time → fires tomorrow.
	got = nextAnnouncementOccurrence(a, mustParse(t, "2026-06-13T20:00:00Z"))
	if got != "2026-06-14T19:30:00Z" {
		t.Errorf("daily (after): got %q", got)
	}
}

func TestNextOccurrenceWeekly(t *testing.T) {
	// "Every Saturday at 19:00 UTC". 2026-06-13 is a Saturday.
	a := model.Announcement{ScheduleKind: "weekly", Timezone: "UTC", ScheduleWeekdays: "6", ScheduleMinutes: 19 * 60}
	// Saturday morning → same day 19:00.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-13T09:00:00Z")); got != "2026-06-13T19:00:00Z" {
		t.Errorf("weekly same-day: got %q", got)
	}
	// Saturday evening (after) → next Saturday.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-13T19:30:00Z")); got != "2026-06-20T19:00:00Z" {
		t.Errorf("weekly next-week: got %q", got)
	}
}

func TestNextOccurrenceWeeklyMultiDay(t *testing.T) {
	// Tuesday(2) & Thursday(4) at 18:00 UTC. 2026-06-13 is Saturday.
	a := model.Announcement{ScheduleKind: "weekly", Timezone: "UTC", ScheduleWeekdays: "2,4", ScheduleMinutes: 18 * 60}
	// From Saturday → next is Tuesday 2026-06-16.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-13T12:00:00Z")); got != "2026-06-16T18:00:00Z" {
		t.Errorf("multi-day → Tue: got %q", got)
	}
	// From Tuesday after the time → Thursday 2026-06-18.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-16T19:00:00Z")); got != "2026-06-18T18:00:00Z" {
		t.Errorf("multi-day → Thu: got %q", got)
	}
}

func TestNextOccurrenceMonthlyNth(t *testing.T) {
	// "Third Saturday of every month at 20:21 UTC".
	a := model.Announcement{
		ScheduleKind:        "monthly",
		Timezone:            "UTC",
		ScheduleWeekdays:    "6",
		ScheduleWeekOfMonth: 3,
		ScheduleMinutes:     20*60 + 21,
	}
	// June 2026: Saturdays fall on 6, 13, 20, 27 → 3rd = the 20th.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-06-20T20:21:00Z" {
		t.Errorf("monthly 3rd Sat (June): got %q", got)
	}
	// After June's occurrence → July's 3rd Saturday (Saturdays 4,11,18,25 → 18th).
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-20T21:00:00Z")); got != "2026-07-18T20:21:00Z" {
		t.Errorf("monthly 3rd Sat (July): got %q", got)
	}
}

func TestNextOccurrenceMonthlyLast(t *testing.T) {
	// "Last Friday of every month at 17:00 UTC". June 2026 Fridays: 5,12,19,26.
	a := model.Announcement{
		ScheduleKind:        "monthly",
		Timezone:            "UTC",
		ScheduleWeekdays:    "5",
		ScheduleWeekOfMonth: -1,
		ScheduleMinutes:     17 * 60,
	}
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-06-26T17:00:00Z" {
		t.Errorf("monthly last Fri: got %q", got)
	}
}

func TestNextOccurrenceMonthlyFifthSkipsShortMonths(t *testing.T) {
	// "5th Saturday at 12:00". June 2026 has only 4 Saturdays; first 5th-Saturday
	// month after is August 2026 (Saturdays 1,8,15,22,29 → the 29th).
	a := model.Announcement{
		ScheduleKind:        "monthly",
		Timezone:            "UTC",
		ScheduleWeekdays:    "6",
		ScheduleWeekOfMonth: 5,
		ScheduleMinutes:     12 * 60,
	}
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-06-01T00:00:00Z")); got != "2026-08-29T12:00:00Z" {
		t.Errorf("monthly 5th Sat: got %q", got)
	}
}

func TestNextOccurrenceTimezoneDST(t *testing.T) {
	// "Every Sunday at 10:00 America/New_York" — the same wall-clock time maps to
	// a different UTC instant in summer (EDT, UTC-4) vs winter (EST, UTC-5),
	// proving the recurrence is anchored to the zone and survives DST.
	a := model.Announcement{
		ScheduleKind:     "weekly",
		Timezone:         "America/New_York",
		ScheduleWeekdays: "0", // Sunday
		ScheduleMinutes:  10 * 60,
	}
	// Summer: next Sunday after 2026-07-01 is 2026-07-05; 10:00 EDT = 14:00 UTC.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-07-01T00:00:00Z")); got != "2026-07-05T14:00:00Z" {
		t.Errorf("DST summer: got %q, want 2026-07-05T14:00:00Z", got)
	}
	// Winter: next Sunday after 2026-12-01 is 2026-12-06; 10:00 EST = 15:00 UTC.
	if got := nextAnnouncementOccurrence(a, mustParse(t, "2026-12-01T00:00:00Z")); got != "2026-12-06T15:00:00Z" {
		t.Errorf("DST winter: got %q, want 2026-12-06T15:00:00Z", got)
	}
}

func TestNextOccurrenceOnceAndUnscheduled(t *testing.T) {
	// once/"" recurrence math has no "next" — the cursor is managed elsewhere.
	if got := nextAnnouncementOccurrence(model.Announcement{ScheduleKind: "once"}, time.Now()); got != "" {
		t.Errorf("once should yield no recurrence: got %q", got)
	}
	if got := nextAnnouncementOccurrence(model.Announcement{ScheduleKind: ""}, time.Now()); got != "" {
		t.Errorf("unscheduled should yield no recurrence: got %q", got)
	}
}

func TestAdvanceCursor(t *testing.T) {
	s := &Server{}
	weekly := model.Announcement{
		ScheduleKind: "weekly", Timezone: "UTC", ScheduleWeekdays: "6", ScheduleMinutes: 19 * 60,
	}

	// Fired on time → rolls forward to next week's Saturday slot.
	onTime := weekly
	onTime.NextPostAt = "2026-06-13T19:00:00Z" // a Saturday
	next, active := s.advanceCursorAt(onTime, mustParse(t, "2026-06-13T19:00:05Z"))
	if !active || next != "2026-06-20T19:00:00Z" {
		t.Errorf("advance on-time: next=%q active=%v", next, active)
	}

	// Overdue by more than a week (e.g. the server was down across a slot): the
	// cursor must jump to the next FUTURE slot, not replay a past one — otherwise
	// it stays due and re-posts every tick (the double-post bug).
	overdue := weekly
	overdue.NextPostAt = "2026-05-30T19:00:00Z" // ~2 Saturdays earlier
	now := mustParse(t, "2026-06-15T12:00:00Z")
	next, active = s.advanceCursorAt(overdue, now)
	if !active || next != "2026-06-20T19:00:00Z" {
		t.Errorf("advance overdue: next=%q active=%v (want the next future slot)", next, active)
	}
	if !mustParse(t, next).After(now) {
		t.Errorf("advance overdue: next %q is not after now %q — it would re-fire", next, now)
	}

	// One-time has no next → deactivates.
	once := model.Announcement{ScheduleKind: "once", NextPostAt: "2026-06-13T19:00:00Z"}
	if next, active := s.advanceCursorAt(once, mustParse(t, "2026-06-14T00:00:00Z")); active || next != "" {
		t.Errorf("advance once: next=%q active=%v", next, active)
	}
}

// newSchedulerEnv builds a minimal Server backed by a temp store plus a daily
// announcement that is already overdue (cursor in the past), for scheduler tests.
// Returns the server, the announcement id, and its starting next_post_at.
func newSchedulerEnv(t *testing.T, webhook string) (*Server, int64, string) {
	t.Helper()
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })

	typeID, err := st.CreateAnnouncementType("T", webhook)
	if err != nil {
		t.Fatal(err)
	}
	startCursor := time.Now().UTC().AddDate(0, 0, -3).Format(time.RFC3339) // 3 days overdue
	a := &model.Announcement{
		Title: "Daily", Details: "d", TypeID: typeID,
		ScheduleKind: "daily", Timezone: "UTC", ScheduleMinutes: 9 * 60,
		NextPostAt: startCursor, Active: true,
	}
	id, err := st.CreateAnnouncement(a)
	if err != nil {
		t.Fatal(err)
	}
	return &Server{store: st}, id, startCursor
}

// TestPostDueAnnouncementsNoDoubleWhenOverdue is the regression test for the
// reported double-posting: an overdue recurring announcement must post exactly
// once and advance its cursor to a future slot, so a second sweep posts nothing.
func TestPostDueAnnouncementsNoDoubleWhenOverdue(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	s, id, _ := newSchedulerEnv(t, ts.URL)

	s.postDueAnnouncements(context.Background())
	s.postDueAnnouncements(context.Background()) // back-to-back: a correct scheduler still posts once

	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("posted %d times across two sweeps; want exactly 1", got)
	}
	reloaded, _ := s.store.GetAnnouncement(id)
	if reloaded == nil || !mustParse(t, reloaded.NextPostAt).After(time.Now()) {
		t.Errorf("cursor not advanced to a future slot: %+v", reloaded)
	}
}

// TestPostDueAnnouncementsAmbiguousAdvances verifies a transport failure (the
// webhook may have been delivered) advances the cursor instead of retrying, so a
// possibly-delivered post is never duplicated on the next tick.
func TestPostDueAnnouncementsAmbiguousAdvances(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	closedURL := ts.URL
	ts.Close() // the port now refuses connections → transport (ambiguous) error

	s, id, startCursor := newSchedulerEnv(t, closedURL)
	s.postDueAnnouncements(context.Background())

	reloaded, _ := s.store.GetAnnouncement(id)
	if reloaded == nil || reloaded.NextPostAt == startCursor {
		t.Errorf("ambiguous failure should advance the cursor (no retry), got %q", reloaded.NextPostAt)
	}
	if reloaded != nil && !mustParse(t, reloaded.NextPostAt).After(time.Now()) {
		t.Errorf("advanced cursor should be in the future, got %q", reloaded.NextPostAt)
	}
}

// TestPostDueAnnouncementsHTTPErrorRetries verifies a definite non-delivery (an
// HTTP error status, e.g. a 429 rate limit or 5xx) leaves the cursor untouched so
// the next tick retries — preserving delivery without duplicating.
func TestPostDueAnnouncementsHTTPErrorRetries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	s, id, startCursor := newSchedulerEnv(t, ts.URL)
	s.postDueAnnouncements(context.Background())

	reloaded, _ := s.store.GetAnnouncement(id)
	if reloaded == nil || reloaded.NextPostAt != startCursor {
		t.Errorf("HTTP error should leave the cursor pending for retry; got %q want %q",
			reloaded.NextPostAt, startCursor)
	}
}

func TestBuildAnnouncementEmbed(t *testing.T) {
	start := mustParse(t, "2026-06-13T19:00:00Z")
	end := mustParse(t, "2026-06-13T21:00:00Z")
	a := model.Announcement{
		Title:     "Tea Time",
		Details:   "**Come hang out** in the lounge.",
		Image:     "https://example.com/banner.png",
		Thumbnail: "https://example.com/thumb.png",
		Color:     "#1abc9c",
		StartAt:   "2026-06-13T19:00:00Z",
		EndAt:     "2026-06-13T21:00:00Z",
	}
	embed := buildAnnouncementEmbed(a)

	if embed.Title != "Tea Time" {
		t.Errorf("title: got %q", embed.Title)
	}
	if embed.Color != 0x1abc9c {
		t.Errorf("color: got %#x, want 0x1abc9c", embed.Color)
	}
	// Two fields, in order: inline time first, then full-width details.
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields (time, details), got %d", len(embed.Fields))
	}
	// First field: inline time. No formats set → defaults: long "F" start, short
	// "t" end.
	timeField := embed.Fields[0]
	if !timeField.Inline {
		t.Error("time field should be inline")
	}
	wantStart := "<t:" + itoa(start.Unix()) + ":F>"
	wantEnd := "<t:" + itoa(end.Unix()) + ":t>"
	if !strings.Contains(timeField.Value, wantStart) || !strings.Contains(timeField.Value, " to "+wantEnd) {
		t.Errorf("time field value %q missing %q / %q", timeField.Value, wantStart, wantEnd)
	}
	// Second field: details, full-width, with a headingless (zero-width space) name.
	detailsField := embed.Fields[1]
	if detailsField.Inline {
		t.Error("details field should be full-width (not inline)")
	}
	if detailsField.Name != embedNoHeading {
		t.Errorf("details field should have no visible heading, got name %q", detailsField.Name)
	}
	if detailsField.Value != a.Details {
		t.Errorf("details field value: got %q, want %q", detailsField.Value, a.Details)
	}
	// Details belong in a field, not the description.
	if embed.Description != "" {
		t.Errorf("description should be empty, got %q", embed.Description)
	}
	if embed.Image == nil || embed.Image.URL != a.Image {
		t.Errorf("image: got %+v", embed.Image)
	}
	if embed.Thumbnail == nil || embed.Thumbnail.URL != a.Thumbnail {
		t.Errorf("thumbnail: got %+v", embed.Thumbnail)
	}
}

func TestBuildAnnouncementEmbedNoTimes(t *testing.T) {
	a := model.Announcement{Title: "Heads up", Details: "Plain note."}
	embed := buildAnnouncementEmbed(a)
	// Only the details field (no time field), full-width and headingless.
	if len(embed.Fields) != 1 {
		t.Fatalf("expected only the details field, got %d", len(embed.Fields))
	}
	if embed.Fields[0].Name != embedNoHeading || embed.Fields[0].Inline {
		t.Errorf("details field: got name=%q inline=%v", embed.Fields[0].Name, embed.Fields[0].Inline)
	}
	if embed.Fields[0].Value != "Plain note." {
		t.Errorf("details value: got %q", embed.Fields[0].Value)
	}
	// No explicit colour → brand accent default.
	if embed.Color != accentColor {
		t.Errorf("color: got %#x, want default %#x", embed.Color, accentColor)
	}
	if embed.Footer != nil {
		t.Error("no time → no footer expected")
	}
}

func TestBuildAnnouncementEmbedLocation(t *testing.T) {
	a := model.Announcement{Title: "Meetup", Details: "See you there.", Location: "Voice Channel #1"}
	embed := buildAnnouncementEmbed(a)
	// Location renders as an inline "📍 Where" field (details are the description).
	var loc *discordEmbedField
	for i := range embed.Fields {
		if embed.Fields[i].Name == "📍 Where" {
			loc = &embed.Fields[i]
		}
	}
	if loc == nil {
		t.Fatalf("expected a location field, got fields %+v", embed.Fields)
	}
	if !loc.Inline || loc.Value != "Voice Channel #1" {
		t.Errorf("location field: got inline=%v value=%q", loc.Inline, loc.Value)
	}
	// No location → no location field (just the details field remains).
	if got := buildAnnouncementEmbed(model.Announcement{Title: "X", Details: "y"}); len(got.Fields) != 1 {
		t.Errorf("no location should yield only the details field, got %d", len(got.Fields))
	}
}

// TestBuildAnnouncementEmbedLongDetails guards the bug where long details were
// silently truncated at the 1024-char field cap: instead they split across
// consecutive full-width fields (at the newline), preserving the time-first order
// and losing nothing.
func TestBuildAnnouncementEmbedLongDetails(t *testing.T) {
	details := strings.Repeat("a", 1000) + "\n" + strings.Repeat("b", 1000) // 2001 chars
	embed := buildAnnouncementEmbed(model.Announcement{Title: "Long", Details: details})
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 detail fields (split at the newline), got %d", len(embed.Fields))
	}
	for i, f := range embed.Fields {
		if f.Name != embedNoHeading || f.Inline {
			t.Errorf("field %d: got name=%q inline=%v; want headingless full-width", i, f.Name, f.Inline)
		}
		if len([]rune(f.Value)) > embedFieldValueMax {
			t.Errorf("field %d length %d exceeds the %d cap", i, len([]rune(f.Value)), embedFieldValueMax)
		}
		if strings.Contains(f.Value, "…") {
			t.Errorf("field %d was truncated with an ellipsis: %q…", i, f.Value[:20])
		}
	}
	if embed.Fields[0].Value != strings.Repeat("a", 1000) || embed.Fields[1].Value != strings.Repeat("b", 1000) {
		t.Errorf("split content not preserved: got %q / %q", embed.Fields[0].Value[:5], embed.Fields[1].Value[:5])
	}
}

// TestSplitForEmbedFields covers the chunker directly: short text stays one
// chunk, and a long block splits at the last newline within the cap.
func TestSplitForEmbedFields(t *testing.T) {
	if got := splitForEmbedFields("", 1024); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
	if got := splitForEmbedFields("short", 1024); len(got) != 1 || got[0] != "short" {
		t.Errorf("short input: got %v, want [short]", got)
	}
	// Break at the newline that falls within the window rather than mid-word.
	text := strings.Repeat("a", 8) + "\n" + strings.Repeat("b", 8) // 17 chars
	got := splitForEmbedFields(text, 10)
	if len(got) != 2 || got[0] != strings.Repeat("a", 8) || got[1] != strings.Repeat("b", 8) {
		t.Errorf("newline split: got %v", got)
	}
	for i, c := range got {
		if len([]rune(c)) > 10 {
			t.Errorf("chunk %d length %d exceeds limit 10", i, len([]rune(c)))
		}
	}
}

// TestDynamicEventTimes verifies the dynamic-dates re-anchoring: the template's
// time-of-day (and the end's next-day offset) move onto the day a post goes out.
func TestDynamicEventTimes(t *testing.T) {
	base := model.Announcement{
		Timezone:   "UTC",
		StartLocal: "2026-06-12T22:00", // Fri 10pm
		EndLocal:   "2026-06-13T01:00", // Sat 1am (runs to the next day)
		StartAt:    "2026-06-12T22:00:00Z",
		EndAt:      "2026-06-13T01:00:00Z",
	}
	ref := mustParse(t, "2026-06-17T09:00:00Z") // a Wednesday morning post

	// Off → stored values unchanged.
	if s, e := dynamicEventTimes(base, ref); s != base.StartAt || e != base.EndAt {
		t.Errorf("dynamic off: got (%q, %q), want stored", s, e)
	}

	// On → re-anchored onto the post day, keeping 10pm and the next-day end.
	on := base
	on.DynamicDates = true
	if s, e := dynamicEventTimes(on, ref); s != "2026-06-17T22:00:00Z" || e != "2026-06-18T01:00:00Z" {
		t.Errorf("dynamic on: got (%q, %q), want (2026-06-17T22:00:00Z, 2026-06-18T01:00:00Z)", s, e)
	}

	// Same-day event keeps a 0-day offset.
	sameDay := model.Announcement{
		DynamicDates: true, Timezone: "UTC",
		StartLocal: "2026-06-12T10:00", EndLocal: "2026-06-12T14:00",
	}
	if s, e := dynamicEventTimes(sameDay, ref); s != "2026-06-17T10:00:00Z" || e != "2026-06-17T14:00:00Z" {
		t.Errorf("same-day: got (%q, %q), want (10:00, 14:00 on 06-17)", s, e)
	}

	// No start time → unchanged even when the flag is on.
	noStart := model.Announcement{DynamicDates: true, EndLocal: "2026-06-12T14:00", StartAt: "", EndAt: "x"}
	if s, e := dynamicEventTimes(noStart, ref); s != "" || e != "x" {
		t.Errorf("no start: got (%q, %q), want stored", s, e)
	}
}

// TestBuildAnnouncementEmbedTimeFormat verifies the independently-chosen Discord
// timestamp styles are applied to the start and end tokens respectively.
func TestBuildAnnouncementEmbedTimeFormat(t *testing.T) {
	start := mustParse(t, "2026-06-13T19:00:00Z")
	end := mustParse(t, "2026-06-13T21:00:00Z")
	a := model.Announcement{
		Title:       "Tea Time",
		Details:     "x",
		StartAt:     "2026-06-13T19:00:00Z",
		EndAt:       "2026-06-13T21:00:00Z",
		StartFormat: "R",
		EndFormat:   "T",
	}
	got := buildAnnouncementEmbed(a).Fields[0].Value
	wantStart := "<t:" + itoa(start.Unix()) + ":R>"
	wantEnd := "<t:" + itoa(end.Unix()) + ":T>"
	if !strings.Contains(got, wantStart) || !strings.Contains(got, " to "+wantEnd) {
		t.Errorf("time field %q missing %q / %q", got, wantStart, wantEnd)
	}
}

func TestDiscordTimeStyle(t *testing.T) {
	cases := []struct{ in, def, want string }{
		{"", "F", "F"},     // empty → given default
		{"", "t", "t"},     // empty → given default
		{"f", "F", "f"},    // valid honored
		{"R", "t", "R"},    // valid honored
		{" t ", "F", "t"},  // trimmed
		{"bad", "F", "F"},  // unknown → default
		{"long", "t", "t"}, // unknown → default
	}
	for _, c := range cases {
		if got := discordTimeStyle(c.in, c.def); got != c.want {
			t.Errorf("discordTimeStyle(%q, %q) = %q, want %q", c.in, c.def, got, c.want)
		}
	}
}

// TestDiscordMarkdown verifies the three Milkdown→Discord normalizations:
// backslash hard breaks, <br> tags, and escaped timestamp tokens.
func TestDiscordMarkdown(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"empty", "", ""},
		{"plain", "Just text.", "Just text."},
		{"backslash break after slash url", "See https://example.com/\\\nNext line", "See https://example.com/\nNext line"},
		{"trailing backslash at end", "ends here\\", "ends here"},
		{"br tag", "line one<br />line two", "line one\nline two"},
		{"br variants", "a<br>b<br/>c<BR />d", "a\nb\nc\nd"},
		// A <br> hard break the serializer follows with a source newline must
		// collapse to ONE line break, not a blank line between every line.
		{"br hard break + source newline", "line one<br />\nline two", "line one\nline two"},
		{"br with crlf", "a<br/>\r\nb", "a\nb"},
		{"real paragraph preserved", "para one\n\npara two", "para one\n\npara two"},
		{"br before blank stays a paragraph", "a<br />\n\nb", "a\n\nb"},
		{"escaped timestamp", "Starts \\<t:1782009000:t> sharp", "Starts <t:1782009000:t> sharp"},
		{"escaped both brackets", "\\<t:1782009000:F\\>", "<t:1782009000:F>"},
		{"unescaped timestamp untouched", "<t:1782009000:R>", "<t:1782009000:R>"},
		{"timestamp no style", "<t:1782009000>", "<t:1782009000>"},
		// CRLF (the editor stores it) is normalized to LF.
		{"crlf normalized", "a\r\nb", "a\nb"},
		// A <br> used as a standalone spacer paragraph (blank lines on both sides,
		// CRLF) collapses to a single blank line — not a stack of them.
		{"br spacer paragraph crlf", "a\r\n\r\n<br />\r\n\r\nb", "a\n\nb"},
		// Loose list (blank line between every item) → tight list.
		{"loose bullet list", "* one\r\n\r\n* two\r\n\r\n* three", "* one\n* two\n* three"},
		{"loose ordered list", "1. a\r\n\r\n2. b", "1. a\n2. b"},
		// A loose list whose items span multiple (indented) lines still tightens.
		{"loose list multiline items", "* one\r\n  cont\r\n\r\n* two", "* one\n  cont\n* two"},
		// The blank line before a list and before a following paragraph is kept.
		{"blank around list kept", "intro\r\n\r\n- a\r\n\r\n- b\r\n\r\nouttro", "intro\n\n- a\n- b\n\nouttro"},
		// A bullet (*) item followed by a dash (-) list joins into one tight list.
		{"mixed markers tighten", "* a\r\n\r\n- b\r\n- c", "* a\n- b\n- c"},
	}
	for _, tc := range cases {
		if got := discordMarkdown(tc.in); got != tc.want {
			t.Errorf("%s: discordMarkdown(%q) = %q, want %q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestColorFromHex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"#1abc9c", 0x1abc9c},
		{"1abc9c", 0x1abc9c},
		{"  #FFFFFF  ", 0xFFFFFF},
		{"", accentColor},            // empty → default
		{"not-a-color", accentColor}, // invalid hex → default
		{"#ffffffff", accentColor},   // out of 24-bit range → default
	}
	for _, c := range cases {
		if got := colorFromHex(c.in, accentColor); got != c.want {
			t.Errorf("colorFromHex(%q): got %#x, want %#x", c.in, got, c.want)
		}
	}
}

// itoa is a tiny int64→string helper for assembling expected token strings.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestSanitizeAnnouncementButtons(t *testing.T) {
	in := []model.AnnouncementButton{
		{Label: "  Sign up  ", Emoji: " 🎉 ", URL: " https://example.com/a "},
		{Label: "", URL: "https://example.com/b"}, // dropped: no label
		{Label: "Bad", URL: "ftp://nope"},         // dropped: non-http URL
		{Label: "One", URL: "https://example.com/1"},
		{Label: "Two", URL: "https://example.com/2"},
		{Label: "Three", URL: "https://example.com/3"},
		{Label: "Four", URL: "https://example.com/4"},
		{Label: "Five", URL: "https://example.com/5"}, // 6th valid → capped out
	}
	got := sanitizeAnnouncementButtons(in)
	if len(got) != maxAnnouncementButtons {
		t.Fatalf("want %d buttons, got %d", maxAnnouncementButtons, len(got))
	}
	if got[0].Label != "Sign up" || got[0].Emoji != "🎉" || got[0].URL != "https://example.com/a" {
		t.Errorf("first button not trimmed: %+v", got[0])
	}
	if sanitizeAnnouncementButtons(nil) != nil {
		t.Error("nil input should yield nil")
	}
}

func TestAnnouncementComponents(t *testing.T) {
	a := model.Announcement{Buttons: []model.AnnouncementButton{
		{Label: "Open", Emoji: "🎉", URL: "https://example.com"},
		{Label: "Custom", Emoji: "<:wave:123>", URL: "https://example.com/2"},
		{Label: "Skip", URL: "not-a-url"}, // dropped: invalid URL
	}}
	comps := announcementComponents(a)
	if len(comps) != 1 || comps[0].Type != componentActionRow {
		t.Fatalf("expected one action row, got %+v", comps)
	}
	row := comps[0].Components
	if len(row) != 2 {
		t.Fatalf("expected 2 valid buttons, got %d", len(row))
	}
	if row[0].Type != componentButton || row[0].Style != buttonStyleLink || row[0].URL != "https://example.com" {
		t.Errorf("button 0: %+v", row[0])
	}
	if row[0].Emoji == nil || row[0].Emoji.Name != "🎉" {
		t.Errorf("button 0 emoji: %+v", row[0].Emoji)
	}
	if row[1].Emoji == nil || row[1].Emoji.Name != "wave" || row[1].Emoji.ID != "123" {
		t.Errorf("button 1 custom emoji: %+v", row[1].Emoji)
	}
	// No buttons → no components (so the payload omits the field entirely).
	if got := announcementComponents(model.Announcement{}); got != nil {
		t.Errorf("no buttons should yield nil components, got %+v", got)
	}
}

func TestAnnouncementMention(t *testing.T) {
	st, err := store.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	s := &Server{store: st}

	// No mention → empty content, no whitelist.
	if c, am := s.announcementMention(model.Announcement{}); c != "" || am != nil {
		t.Errorf("none: got content=%q allowed=%+v", c, am)
	}

	// @everyone → content + parse ["everyone"].
	c, am := s.announcementMention(model.Announcement{Mention: "everyone"})
	if c != "@everyone" || am == nil || len(am.Parse) != 1 || am.Parse[0] != "everyone" {
		t.Errorf("everyone: got content=%q allowed=%+v", c, am)
	}

	// A managed role → "<@&id>" with the role whitelisted (and parse suppressed).
	id, err := st.CreateAnnouncementRole("Crew", "123456789012345678")
	if err != nil {
		t.Fatal(err)
	}
	c, am = s.announcementMention(model.Announcement{Mention: "role:" + strconv.FormatInt(id, 10)})
	if c != "<@&123456789012345678>" || am == nil || len(am.Roles) != 1 || am.Roles[0] != "123456789012345678" {
		t.Errorf("role: got content=%q allowed=%+v", c, am)
	}
	if am != nil && len(am.Parse) != 0 {
		t.Errorf("role: parse should be empty to suppress other mentions, got %+v", am.Parse)
	}

	// A role reference that no longer resolves → no mention (announcement still posts).
	if c, am := s.announcementMention(model.Announcement{Mention: "role:999999"}); c != "" || am != nil {
		t.Errorf("missing role: got content=%q allowed=%+v", c, am)
	}
}

func TestParseEmoji(t *testing.T) {
	if e := parseEmoji("  "); e != nil {
		t.Errorf("blank → nil, got %+v", e)
	}
	if e := parseEmoji("🎉"); e == nil || e.Name != "🎉" || e.ID != "" {
		t.Errorf("unicode emoji: %+v", e)
	}
	if e := parseEmoji("<:wave:123>"); e == nil || e.Name != "wave" || e.ID != "123" || e.Animated {
		t.Errorf("custom emoji: %+v", e)
	}
	if e := parseEmoji("<a:spin:456>"); e == nil || e.Name != "spin" || e.ID != "456" || !e.Animated {
		t.Errorf("animated custom emoji: %+v", e)
	}
}
