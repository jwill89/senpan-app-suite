package server

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"app-suite/internal/model"
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
	// Recurring weekly rolls forward from the fired instant.
	weekly := model.Announcement{
		ScheduleKind: "weekly", Timezone: "UTC", ScheduleWeekdays: "6", ScheduleMinutes: 19 * 60,
		NextPostAt: "2026-06-13T19:00:00Z",
	}
	next, active := s.advanceCursor(weekly)
	if !active || next != "2026-06-20T19:00:00Z" {
		t.Errorf("advance weekly: next=%q active=%v", next, active)
	}
	// One-time has no next → deactivates.
	once := model.Announcement{ScheduleKind: "once", NextPostAt: "2026-06-13T19:00:00Z"}
	next, active = s.advanceCursor(once)
	if active || next != "" {
		t.Errorf("advance once: next=%q active=%v", next, active)
	}
}

func TestBuildAnnouncementEmbed(t *testing.T) {
	start := mustParse(t, "2026-06-13T19:00:00Z")
	end := mustParse(t, "2026-06-13T21:00:00Z")
	a := model.Announcement{
		Title:   "Tea Time",
		Details: "**Come hang out** in the lounge.",
		Image:   "https://example.com/banner.png",
		Color:   "#1abc9c",
		StartAt: "2026-06-13T19:00:00Z",
		EndAt:   "2026-06-13T21:00:00Z",
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
	// First field: inline time, long "F" start, short "t" end.
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
