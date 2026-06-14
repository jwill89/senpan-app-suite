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
		StartAt: "2026-06-13T19:00:00Z",
		EndAt:   "2026-06-13T21:00:00Z",
	}
	embed := buildAnnouncementEmbed(a)

	if embed.Title != "Tea Time" {
		t.Errorf("title: got %q", embed.Title)
	}
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields (time + details), got %d", len(embed.Fields))
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
	// Second field: full-width details.
	if embed.Fields[1].Inline {
		t.Error("details field should be full-width (not inline)")
	}
	if embed.Image == nil || embed.Image.URL != a.Image {
		t.Errorf("image: got %+v", embed.Image)
	}
}

func TestBuildAnnouncementEmbedNoTimes(t *testing.T) {
	a := model.Announcement{Title: "Heads up", Details: "Plain note."}
	embed := buildAnnouncementEmbed(a)
	if len(embed.Fields) != 1 || embed.Fields[0].Name != "Details" {
		t.Errorf("expected only a details field, got %+v", embed.Fields)
	}
	if embed.Footer != nil {
		t.Error("no time → no footer expected")
	}
}

// itoa is a tiny int64→string helper for assembling expected token strings.
func itoa(n int64) string {
	return strconv.FormatInt(n, 10)
}
