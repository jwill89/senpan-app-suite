package festival

import (
	"testing"
	"time"
)

func TestNormalizeSlug(t *testing.T) {
	for in, want := range map[string]string{
		"obon-2026":               "obon-2026",
		"  Obon _ Matsuri  2026 ": "obon-matsuri-2026",
		"--Obon--":                "obon",
		"OBON__2026":              "obon-2026",
		"":                        "",
		" _ - ":                   "",
	} {
		if got := NormalizeSlug(in); got != want {
			t.Errorf("NormalizeSlug(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSlugError(t *testing.T) {
	for slug, bad := range map[string]bool{
		"":           false,
		"obon-2026":  false,
		"ab":         false,
		"a":          true, // too short
		"2026":       true, // it would be a map id
		"obon!2026":  true,
		"obon--2026": true, // folded away before it gets here
		"-obon":      true,
		"café":       true,
	} {
		if got := SlugError(slug) != ""; got != bad {
			t.Errorf("SlugError(%q) refused = %v; want %v", slug, got, bad)
		}
	}
	long := make([]byte, 65)
	for i := range long {
		long[i] = 'a'
	}
	if SlugError(string(long)) == "" {
		t.Error("a 65-character shortcode was accepted")
	}
}

func TestSanitizeColor(t *testing.T) {
	for in, want := range map[string]string{
		"#AABBCC":                 "#aabbcc",
		" #aabbcc ":               "#aabbcc",
		"#abc":                    "",
		"red":                     "",
		"red; background: url(x)": "",
		"#aabbccdd":               "",
		"":                        "",
	} {
		if got := SanitizeColor(in); got != want {
			t.Errorf("SanitizeColor(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestSanitizeEventTimes(t *testing.T) {
	got := SanitizeEventTimes([]EventTime{
		{Label: " Day 1 ", Start: " 2026-08-01T18:00:00Z ", End: ""},
		{Label: "Ghost", Start: "  "},
	})
	if len(got) != 1 || got[0].Label != "Day 1" || got[0].Start != "2026-08-01T18:00:00Z" {
		t.Errorf("got %+v; want the one range, trimmed, and the startless one dropped", got)
	}
	if got := SanitizeEventTimes(nil); got == nil {
		t.Error("nil in, nil out; want an empty list so the column holds []")
	}
}

func TestWithinAnyEventTime(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	day1 := EventTime{Start: "2026-08-01T00:00:00Z", End: "2026-08-01T23:59:00Z"}
	day2 := EventTime{Start: "2026-08-02T00:00:00Z", End: "2026-08-02T23:59:00Z"}
	open := EventTime{Start: "2026-08-02T00:00:00Z"}
	for name, c := range map[string]struct {
		times []EventTime
		want  bool
	}{
		"none stated is unbounded":   {nil, true},
		"inside one of several":      {[]EventTime{day1, day2}, true},
		"between ranges":             {[]EventTime{day1}, false},
		"started with no stated end": {[]EventTime{open}, true},
	} {
		if got := WithinAnyEventTime(c.times, now); got != c.want {
			t.Errorf("%s: %v; want %v", name, got, c.want)
		}
	}
}

func TestKeepOccupant(t *testing.T) {
	for name, c := range map[string]struct {
		id           int64
		title        string
		hasAffiliate bool
		want         bool
	}{
		"a new blank row goes":            {0, "   ", false, false},
		"a new row with a title":          {0, "Tea Bar", false, true},
		"a new row with an affiliate":     {0, "", true, true},
		"a stored row is kept even blank": {7, "", false, true},
	} {
		if got := KeepOccupant(c.id, c.title, c.hasAffiliate); got != c.want {
			t.Errorf("%s: %v; want %v", name, got, c.want)
		}
	}
}

func TestNormalizers(t *testing.T) {
	if NormalizeShape("triangle") != ShapeRect || NormalizeShape(ShapeCircle) != ShapeCircle {
		t.Error("a shape is a circle or, failing that, a rectangle")
	}
	if NormalizeStallType("hovercraft") != StallOther || NormalizeStallType(StallBoth) != StallBoth {
		t.Error("a stall type is a known one or other")
	}
	if NormalizeMapStatus("") != MapInProgress || NormalizeMapStatus(MapClosed) != MapClosed {
		t.Error("a map status is a known one or in progress")
	}
}
