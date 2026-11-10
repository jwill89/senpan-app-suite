package announcements_test

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"app-suite/internal/announcements"
	"app-suite/internal/discord"
)

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return v.UTC()
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestIsSnowflake(t *testing.T) {
	if !announcements.IsSnowflake("123456789012345678") {
		t.Error("an all-digit ID should be a snowflake")
	}
	for _, bad := range []string{"", "12a45", "-123", "12 34", "1.5"} {
		if announcements.IsSnowflake(bad) {
			t.Errorf("%q should not be a snowflake", bad)
		}
	}
}

func TestEmbed(t *testing.T) {
	start := mustParse(t, "2026-06-13T19:00:00Z")
	end := mustParse(t, "2026-06-13T21:00:00Z")
	a := announcements.Announcement{
		Title:     "Tea Time",
		Details:   "**Come hang out** in the lounge.",
		Image:     "https://example.com/banner.png",
		Thumbnail: "https://example.com/thumb.png",
		Color:     "#1abc9c",
		StartAt:   "2026-06-13T19:00:00Z",
		EndAt:     "2026-06-13T21:00:00Z",
	}
	embed := announcements.Embed(a)

	if embed.Title != "Tea Time" {
		t.Errorf("title: got %q", embed.Title)
	}
	if embed.Color != 0x1abc9c {
		t.Errorf("color: got %#x, want 0x1abc9c", embed.Color)
	}
	// Two fields, in order: full-width time first, then full-width details.
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 fields (time, details), got %d", len(embed.Fields))
	}
	timeField := embed.Fields[0]
	if timeField.Inline {
		t.Error("time field should be full-width (not inline)")
	}
	wantStart := "<t:" + itoa(start.Unix()) + ":F>"
	wantEnd := "<t:" + itoa(end.Unix()) + ":t>"
	if !strings.Contains(timeField.Value, wantStart) || !strings.Contains(timeField.Value, " to "+wantEnd) {
		t.Errorf("time field value %q missing %q / %q", timeField.Value, wantStart, wantEnd)
	}
	details := embed.Fields[1]
	if details.Inline {
		t.Error("details field should be full-width (not inline)")
	}
	if details.Name != discord.NoHeading {
		t.Errorf("details field should have no visible heading, got name %q", details.Name)
	}
	if details.Value != a.Details {
		t.Errorf("details field value: got %q, want %q", details.Value, a.Details)
	}
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

func TestEmbedNoTimes(t *testing.T) {
	embed := announcements.Embed(announcements.Announcement{Title: "Heads up", Details: "Plain note."})
	if len(embed.Fields) != 1 {
		t.Fatalf("expected only the details field, got %d", len(embed.Fields))
	}
	if embed.Fields[0].Name != discord.NoHeading || embed.Fields[0].Inline {
		t.Errorf("details field: got name=%q inline=%v", embed.Fields[0].Name, embed.Fields[0].Inline)
	}
	if embed.Fields[0].Value != "Plain note." {
		t.Errorf("details value: got %q", embed.Fields[0].Value)
	}
	if embed.Color != discord.AccentColor {
		t.Errorf("color: got %#x, want default %#x", embed.Color, discord.AccentColor)
	}
	if embed.Footer != nil {
		t.Error("no time -> no footer expected")
	}
}

func TestEmbedLocation(t *testing.T) {
	embed := announcements.Embed(announcements.Announcement{Title: "Meetup", Details: "See you there.", Location: "Voice Channel #1"})
	var loc *discord.EmbedField
	for i := range embed.Fields {
		if embed.Fields[i].Name == "📍 Where" {
			loc = &embed.Fields[i]
		}
	}
	if loc == nil {
		t.Fatalf("expected a location field, got fields %+v", embed.Fields)
	}
	if loc.Inline || loc.Value != "Voice Channel #1" {
		t.Errorf("location field: got inline=%v value=%q", loc.Inline, loc.Value)
	}
	if got := announcements.Embed(announcements.Announcement{Title: "X", Details: "y"}); len(got.Fields) != 1 {
		t.Errorf("no location should yield only the details field, got %d", len(got.Fields))
	}
}

// Long details split across consecutive full-width fields at the newline instead of
// being cut at the field cap, keeping the time-first order.
func TestEmbedLongDetails(t *testing.T) {
	details := strings.Repeat("a", 1000) + "\n" + strings.Repeat("b", 1000)
	embed := announcements.Embed(announcements.Announcement{Title: "Long", Details: details})
	if len(embed.Fields) != 2 {
		t.Fatalf("expected 2 detail fields (split at the newline), got %d", len(embed.Fields))
	}
	for i, f := range embed.Fields {
		if f.Name != discord.NoHeading || f.Inline {
			t.Errorf("field %d: got name=%q inline=%v; want headingless full-width", i, f.Name, f.Inline)
		}
		if len([]rune(f.Value)) > discord.EmbedFieldValueMax {
			t.Errorf("field %d length %d exceeds the %d cap", i, len([]rune(f.Value)), discord.EmbedFieldValueMax)
		}
		if strings.Contains(f.Value, "...") {
			t.Errorf("field %d was truncated with an ellipsis", i)
		}
	}
	if embed.Fields[0].Value != strings.Repeat("a", 1000) || embed.Fields[1].Value != strings.Repeat("b", 1000) {
		t.Error("split content not preserved")
	}
}

func TestSplitForFields(t *testing.T) {
	if got := announcements.SplitForFields("", 1024); got != nil {
		t.Errorf("empty input: got %v, want nil", got)
	}
	if got := announcements.SplitForFields("short", 1024); len(got) != 1 || got[0] != "short" {
		t.Errorf("short input: got %v, want [short]", got)
	}
	text := strings.Repeat("a", 8) + "\n" + strings.Repeat("b", 8)
	got := announcements.SplitForFields(text, 10)
	if len(got) != 2 || got[0] != strings.Repeat("a", 8) || got[1] != strings.Repeat("b", 8) {
		t.Errorf("newline split: got %v", got)
	}
	// No newline: the last space; no space either: a hard cut.
	if got := announcements.SplitForFields("aaaa bbbb cccc", 10); len(got) != 2 || got[0] != "aaaa bbbb" || got[1] != "cccc" {
		t.Errorf("space split: got %v", got)
	}
	if got := announcements.SplitForFields(strings.Repeat("x", 25), 10); len(got) != 3 || len([]rune(got[0])) != 10 {
		t.Errorf("hard cut: got %v", got)
	}
}

// With dynamic dates the template's time of day (and the end's next-day offset) move
// onto the day a post goes out.
func TestEventTimes(t *testing.T) {
	base := announcements.Announcement{
		Timezone:   "UTC",
		StartLocal: "2026-06-12T22:00",
		EndLocal:   "2026-06-13T01:00",
		StartAt:    "2026-06-12T22:00:00Z",
		EndAt:      "2026-06-13T01:00:00Z",
	}
	ref := mustParse(t, "2026-06-17T09:00:00Z")

	if s, e := announcements.EventTimes(base, ref); s != base.StartAt || e != base.EndAt {
		t.Errorf("dynamic off: got (%q, %q), want stored", s, e)
	}

	on := base
	on.DynamicDates = true
	if s, e := announcements.EventTimes(on, ref); s != "2026-06-17T22:00:00Z" || e != "2026-06-18T01:00:00Z" {
		t.Errorf("dynamic on: got (%q, %q)", s, e)
	}

	sameDay := announcements.Announcement{
		DynamicDates: true, Timezone: "UTC", StartLocal: "2026-06-12T10:00", EndLocal: "2026-06-12T14:00",
	}
	if s, e := announcements.EventTimes(sameDay, ref); s != "2026-06-17T10:00:00Z" || e != "2026-06-17T14:00:00Z" {
		t.Errorf("same-day: got (%q, %q)", s, e)
	}

	noStart := announcements.Announcement{DynamicDates: true, EndLocal: "2026-06-12T14:00", EndAt: "x"}
	if s, e := announcements.EventTimes(noStart, ref); s != "" || e != "x" {
		t.Errorf("no start: got (%q, %q), want stored", s, e)
	}
}

func TestEmbedTimeFormat(t *testing.T) {
	start := mustParse(t, "2026-06-13T19:00:00Z")
	end := mustParse(t, "2026-06-13T21:00:00Z")
	got := announcements.Embed(announcements.Announcement{
		Title: "Tea Time", Details: "x",
		StartAt: "2026-06-13T19:00:00Z", EndAt: "2026-06-13T21:00:00Z",
		StartFormat: "R", EndFormat: "T",
	}).Fields[0].Value
	wantStart := "<t:" + itoa(start.Unix()) + ":R>"
	wantEnd := "<t:" + itoa(end.Unix()) + ":T>"
	if !strings.Contains(got, wantStart) || !strings.Contains(got, " to "+wantEnd) {
		t.Errorf("time field %q missing %q / %q", got, wantStart, wantEnd)
	}
}

func TestSanitizeButtons(t *testing.T) {
	in := []announcements.Button{
		{Label: "  Sign up  ", Emoji: " 🎉 ", URL: " https://example.com/a "},
		{Label: "", URL: "https://example.com/b"},
		{Label: "Bad", URL: "ftp://nope"},
		{Label: "One", URL: "https://example.com/1"},
		{Label: "Two", URL: "https://example.com/2"},
		{Label: "Three", URL: "https://example.com/3"},
		{Label: "Four", URL: "https://example.com/4"},
		{Label: "Five", URL: "https://example.com/5"},
	}
	got := announcements.SanitizeButtons(in)
	if len(got) != announcements.MaxButtons {
		t.Fatalf("want %d buttons, got %d", announcements.MaxButtons, len(got))
	}
	if got[0].Label != "Sign up" || got[0].Emoji != "🎉" || got[0].URL != "https://example.com/a" {
		t.Errorf("first button not trimmed: %+v", got[0])
	}
	if got := announcements.SanitizeButtons(nil); len(got) != 0 {
		t.Errorf("nil input should yield none, got %v", got)
	}
}

func TestMessageButtons(t *testing.T) {
	a := announcements.Announcement{Title: "T", Details: "d", Buttons: []announcements.Button{
		{Label: "Open", Emoji: "🎉", URL: "https://example.com"},
		{Label: "Custom", Emoji: "<:wave:123>", URL: "https://example.com/2"},
		{Label: "Skip", URL: "not-a-url"},
	}}
	comps := announcements.Message(a, "", time.Now()).Components
	if len(comps) != 1 || comps[0].Type != discord.ComponentActionRow {
		t.Fatalf("expected one action row, got %+v", comps)
	}
	row := comps[0].Components
	if len(row) != 2 {
		t.Fatalf("expected 2 valid buttons, got %d", len(row))
	}
	if row[0].Type != discord.ComponentButton || row[0].Style != discord.ButtonStyleLink || row[0].URL != "https://example.com" {
		t.Errorf("button 0: %+v", row[0])
	}
	if row[0].Emoji == nil || row[0].Emoji.Name != "🎉" {
		t.Errorf("button 0 emoji: %+v", row[0].Emoji)
	}
	if row[1].Emoji == nil || row[1].Emoji.Name != "wave" || row[1].Emoji.ID != "123" {
		t.Errorf("button 1 custom emoji: %+v", row[1].Emoji)
	}
	if got := announcements.Message(announcements.Announcement{Title: "T", Details: "d"}, "", time.Now()).Components; got != nil {
		t.Errorf("no buttons should yield nil components, got %+v", got)
	}
}

func TestMentionContent(t *testing.T) {
	if c, am := announcements.MentionContent("", ""); c != "" || am != nil {
		t.Errorf("none: got content=%q allowed=%+v", c, am)
	}
	c, am := announcements.MentionContent("everyone", "")
	if c != "@everyone" || am == nil || len(am.Parse) != 1 || am.Parse[0] != "everyone" {
		t.Errorf("everyone: got content=%q allowed=%+v", c, am)
	}
	// A managed role: its Discord id tagged, with parsing suppressed.
	c, am = announcements.MentionContent("role:7", "123456789012345678")
	if c != "<@&123456789012345678>" || am == nil || len(am.Roles) != 1 || am.Roles[0] != "123456789012345678" {
		t.Errorf("role: got content=%q allowed=%+v", c, am)
	}
	if am != nil && len(am.Parse) != 0 {
		t.Errorf("role: parse should be empty to suppress other mentions, got %+v", am.Parse)
	}
	// A role that no longer resolves posts without a tag.
	if c, am := announcements.MentionContent("role:7", ""); c != "" || am != nil {
		t.Errorf("missing role: got content=%q allowed=%+v", c, am)
	}
	if c, am := announcements.MentionContent("role:x", "1"); c != "" || am != nil {
		t.Errorf("malformed role: got content=%q allowed=%+v", c, am)
	}
}

func TestParseMention(t *testing.T) {
	cases := []struct {
		in       string
		everyone bool
		role     int64
		ok       bool
	}{
		{"", false, 0, true},
		{"everyone", true, 0, true},
		{"role:12", false, 12, true},
		{"role:0", false, 0, false},
		{"role:-3", false, 0, false},
		{"role:x", false, 0, false},
		{"12", false, 0, false},
		{"here", false, 0, false},
	}
	for _, c := range cases {
		everyone, role, ok := announcements.ParseMention(c.in)
		if everyone != c.everyone || role != c.role || ok != c.ok {
			t.Errorf("ParseMention(%q) = %v, %d, %v; want %v, %d, %v", c.in, everyone, role, ok, c.everyone, c.role, c.ok)
		}
	}
}

// -- Resolve -----------------------------------------------------------------

func fields(mod func(*announcements.Fields)) announcements.Fields {
	f := announcements.Fields{Title: "Bingo Night", Details: "Tonight!"}
	if mod != nil {
		mod(&f)
	}
	return f
}

func problemFields(ps []announcements.Problem) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Field
	}
	return out
}

func TestResolveUnscheduled(t *testing.T) {
	r, problems := announcements.Resolve(fields(nil), nil, time.Now())
	if len(problems) != 0 {
		t.Fatalf("problems: %+v", problems)
	}
	if r.NextPostAt != "" || r.Active {
		t.Errorf("unscheduled: next=%q active=%v; want none and inactive", r.NextPostAt, r.Active)
	}
	if r.StartFormat != "F" || r.EndFormat != "t" {
		t.Errorf("formats default to %q/%q; got %q/%q", "F", "t", r.StartFormat, r.EndFormat)
	}
}

func TestResolveRequiredFields(t *testing.T) {
	_, problems := announcements.Resolve(fields(func(f *announcements.Fields) { f.Title = "  "; f.Details = "" }), nil, time.Now())
	got := problemFields(problems)
	if len(got) != 2 || got[0] != "title" || got[1] != "details" {
		t.Errorf("problems = %v; want title and details", got)
	}
}

func TestResolveTimezone(t *testing.T) {
	timed := func(tz string) announcements.Fields {
		return fields(func(f *announcements.Fields) { f.StartLocal = "2026-06-13T19:00"; f.Timezone = tz })
	}
	if _, ps := announcements.Resolve(timed(""), nil, time.Now()); len(ps) != 1 || ps[0].Field != "timezone" {
		t.Errorf("no timezone: %+v", ps)
	}
	if _, ps := announcements.Resolve(timed("Mars/Olympus"), nil, time.Now()); len(ps) != 1 || ps[0].Field != "timezone" {
		t.Errorf("bad timezone: %+v", ps)
	}
	r, ps := announcements.Resolve(timed("America/Chicago"), nil, time.Now())
	if len(ps) != 0 || r.StartAt != "2026-06-14T00:00:00Z" {
		t.Errorf("start = %q, problems %+v; want 2026-06-14T00:00:00Z", r.StartAt, ps)
	}
	// With no times, the timezone is irrelevant.
	if _, ps := announcements.Resolve(fields(nil), nil, time.Now()); len(ps) != 0 {
		t.Errorf("no times needs no timezone: %+v", ps)
	}
}

func TestResolveBadTimes(t *testing.T) {
	_, ps := announcements.Resolve(fields(func(f *announcements.Fields) {
		f.Timezone, f.StartLocal, f.EndLocal = "UTC", "soon", "later"
	}), nil, time.Now())
	if got := problemFields(ps); len(got) != 2 || got[0] != "startLocal" || got[1] != "endLocal" {
		t.Errorf("problems = %v; want startLocal and endLocal", got)
	}
}

func TestResolveMention(t *testing.T) {
	for _, bad := range []string{"here", "role:", "role:0", "role:abc", "Everyone"} {
		_, ps := announcements.Resolve(fields(func(f *announcements.Fields) { f.Mention = bad }), nil, time.Now())
		if len(ps) != 1 || ps[0].Field != "mention" {
			t.Errorf("mention %q: problems %+v; want one on mention", bad, ps)
		}
	}
	for _, ok := range []string{"", "everyone", "role:12"} {
		if _, ps := announcements.Resolve(fields(func(f *announcements.Fields) { f.Mention = ok }), nil, time.Now()); len(ps) != 0 {
			t.Errorf("mention %q: %+v", ok, ps)
		}
	}
}

func TestResolveRecurring(t *testing.T) {
	now := mustParse(t, "2026-06-13T10:00:00Z")
	weekly := func(mod func(*announcements.Fields)) announcements.Fields {
		return fields(func(f *announcements.Fields) {
			f.ScheduleKind, f.Timezone, f.ScheduleWeekdays, f.ScheduleMinutes = "weekly", "UTC", "6", 19*60
			if mod != nil {
				mod(f)
			}
		})
	}
	r, ps := announcements.Resolve(weekly(nil), nil, now)
	if len(ps) != 0 || r.NextPostAt != "2026-06-13T19:00:00Z" || !r.Active {
		t.Errorf("weekly: next=%q active=%v problems %+v", r.NextPostAt, r.Active, ps)
	}

	if _, ps := announcements.Resolve(weekly(func(f *announcements.Fields) { f.ScheduleMinutes = 1440 }), nil, now); len(ps) != 1 || ps[0].Field != "scheduleMinutes" {
		t.Errorf("out-of-range minutes: %+v", ps)
	}
	if _, ps := announcements.Resolve(weekly(func(f *announcements.Fields) { f.ScheduleMinutes = -1 }), nil, now); len(ps) != 1 || ps[0].Field != "scheduleMinutes" {
		t.Errorf("negative minutes: %+v", ps)
	}
	if _, ps := announcements.Resolve(weekly(func(f *announcements.Fields) { f.ScheduleWeekdays = "" }), nil, now); len(ps) != 1 || ps[0].Field != "scheduleWeekdays" {
		t.Errorf("no weekdays: %+v", ps)
	}
	monthly := func(week int) announcements.Fields {
		return weekly(func(f *announcements.Fields) { f.ScheduleKind, f.ScheduleWeekOfMonth = "monthly", week })
	}
	if _, ps := announcements.Resolve(monthly(0), nil, now); len(ps) != 1 || ps[0].Field != "scheduleWeekOfMonth" {
		t.Errorf("week 0: %+v", ps)
	}
	if r, ps := announcements.Resolve(monthly(-1), nil, now); len(ps) != 0 || r.NextPostAt != "2026-06-27T19:00:00Z" {
		t.Errorf("last Saturday: next=%q problems %+v", r.NextPostAt, ps)
	}
	if _, ps := announcements.Resolve(weekly(func(f *announcements.Fields) { f.ScheduleKind = "hourly" }), nil, now); len(ps) != 1 || ps[0].Field != "scheduleKind" {
		t.Errorf("bad kind: %+v", ps)
	}
}

func TestResolveOnce(t *testing.T) {
	now := mustParse(t, "2026-06-13T10:00:00Z")
	once := func(local string) announcements.Fields {
		return fields(func(f *announcements.Fields) { f.ScheduleKind, f.Timezone, f.OnceLocal = "once", "UTC", local })
	}
	if _, ps := announcements.Resolve(once(""), nil, now); len(ps) != 1 || ps[0].Field != "onceLocal" {
		t.Errorf("no date: %+v", ps)
	}
	if _, ps := announcements.Resolve(once("whenever"), nil, now); len(ps) != 1 || ps[0].Field != "onceLocal" {
		t.Errorf("bad date: %+v", ps)
	}

	// A create keeps a past date, so it goes out on the next sweep.
	r, ps := announcements.Resolve(once("2026-06-01T12:00"), nil, now)
	if len(ps) != 0 || r.NextPostAt != "2026-06-01T12:00:00Z" || !r.Active {
		t.Errorf("create in the past: next=%q active=%v problems %+v", r.NextPostAt, r.Active, ps)
	}

	// An edit of one that already went out (no next post, inactive) stays spent...
	spent := &announcements.Cursor{}
	r, _ = announcements.Resolve(once("2026-06-01T12:00"), spent, now)
	if r.NextPostAt != "" || r.Active {
		t.Errorf("edit of a spent one-time: next=%q active=%v; want it left spent", r.NextPostAt, r.Active)
	}
	// ...unless its date moves to the future, a deliberate re-arm.
	r, _ = announcements.Resolve(once("2026-06-20T12:00"), spent, now)
	if r.NextPostAt != "2026-06-20T12:00:00Z" || !r.Active {
		t.Errorf("edit to the future: next=%q active=%v; want it re-armed", r.NextPostAt, r.Active)
	}
	// A pending one keeps its own cursor when the form's date is in the past.
	pending := &announcements.Cursor{NextPostAt: "2026-06-15T00:00:00Z", Active: true}
	r, _ = announcements.Resolve(once("2026-06-01T12:00"), pending, now)
	if r.NextPostAt != pending.NextPostAt || !r.Active {
		t.Errorf("edit of a pending one: next=%q active=%v; want its cursor kept", r.NextPostAt, r.Active)
	}
}
