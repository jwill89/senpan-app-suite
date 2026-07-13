package server

import (
	"strings"
	"testing"
	"time"

	"app-suite/internal/model"
)

func TestBuildAffiliateEmbed(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	a := model.Affiliate{
		Name:        "The Tipsy Moogle",
		Location:    "Ul'dah, Steps of Nald",
		Details:     "**Cozy** spot.",
		EmbedColor:  "#3fb950",
		DiscordLink: "discord.gg/abc", // no scheme → normalized to https
		CarrdLink:   "https://tipsy.carrd.co",
		Logo:        "images/affiliate_logos/x.png",
		Screenshot:  "images/affiliate_images/y.png",
		Timezone:    "America/New_York",
		Hours: []model.AffiliateHour{
			{Label: "Mon–Fri", Start: "18:00", End: "23:00"},
			{Label: "Sat", Start: "20:00"}, // open-ended (no end)
		},
	}
	e := buildAffiliateEmbed(a, "https://apps.senpan.cafe", now)

	if e.Title != "The Tipsy Moogle" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Color != 0x3FB950 {
		t.Errorf("color = %#x; want 0x3fb950", e.Color)
	}
	if !strings.Contains(e.Description, "Cozy") {
		t.Errorf("description = %q", e.Description)
	}
	if e.Thumbnail == nil || e.Thumbnail.URL != "https://apps.senpan.cafe/images/affiliate_logos/x.png" {
		t.Errorf("thumbnail = %+v; want absolute logo url", e.Thumbnail)
	}
	if e.Image == nil || e.Image.URL != "https://apps.senpan.cafe/images/affiliate_images/y.png" {
		t.Errorf("image = %+v; want absolute screenshot url", e.Image)
	}
	if e.Footer == nil || e.Footer.Text != "Times are displayed in your local time zone." {
		t.Errorf("footer = %+v", e.Footer)
	}

	// Field order: Location (full-width), Open Times (full-width), then Discord +
	// Carrd (inline, side by side).
	if len(e.Fields) != 4 {
		t.Fatalf("fields = %d; want 4 (location, hours, discord, carrd): %+v", len(e.Fields), e.Fields)
	}
	loc, hours, discord, carrd := e.Fields[0], e.Fields[1], e.Fields[2], e.Fields[3]

	if !strings.Contains(loc.Name, "Location") || loc.Inline || !strings.Contains(loc.Value, "Ul'dah") {
		t.Errorf("field[0] should be full-width Location: %+v", loc)
	}
	if !strings.Contains(hours.Name, "Open Times") || hours.Inline {
		t.Fatalf("field[1] should be full-width Open Times: %+v", hours)
	}
	// Local-time (t) tokens, both labelled days present, in order.
	for _, want := range []string{"**Mon–Fri**", "**Sat**", "<t:", ":t>"} {
		if !strings.Contains(hours.Value, want) {
			t.Errorf("open-times value missing %q: %q", want, hours.Value)
		}
	}
	if !strings.Contains(discord.Name, "Discord") || !discord.Inline ||
		!strings.Contains(discord.Value, "https://discord.gg/abc") {
		t.Errorf("field[2] should be inline Discord: %+v", discord)
	}
	if !strings.Contains(carrd.Name, "Carrd") || !carrd.Inline ||
		!strings.Contains(carrd.Value, "https://tipsy.carrd.co") {
		t.Errorf("field[3] should be inline Carrd: %+v", carrd)
	}
}

func TestBuildAffiliateEmbed_Minimal(t *testing.T) {
	// No hours/links/images/color → a valid embed with just a title.
	e := buildAffiliateEmbed(model.Affiliate{Name: "Bare"}, "https://x", time.Now())
	if e.Title != "Bare" {
		t.Errorf("title = %q", e.Title)
	}
	if e.Footer != nil {
		t.Errorf("footer should be nil with no hours: %+v", e.Footer)
	}
	if len(e.Fields) != 0 {
		t.Errorf("fields = %+v; want none", e.Fields)
	}
	if e.Thumbnail != nil || e.Image != nil {
		t.Errorf("no images expected: thumb=%+v img=%+v", e.Thumbnail, e.Image)
	}
	if e.Color != accentColor {
		t.Errorf("empty color should fall back to the brand accent (%#x), got %#x", accentColor, e.Color)
	}
}

func TestHHMMToUnix(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC) // summer → EDT (UTC-4)
	u, ok := hhmmToUnix("18:00", loc, now)
	if !ok {
		t.Fatal("expected ok for 18:00")
	}
	if got := time.Unix(u, 0).UTC(); got.Hour() != 22 || got.Minute() != 0 {
		t.Errorf("18:00 EDT → %02d:%02d UTC; want 22:00", got.Hour(), got.Minute())
	}
	if _, ok := hhmmToUnix("bad", loc, now); ok {
		t.Error("non-numeric time should fail")
	}
	if _, ok := hhmmToUnix("25:61", loc, now); ok {
		t.Error("out-of-range time should fail")
	}
}

func TestAbsoluteAssetURL(t *testing.T) {
	cases := map[string]string{
		"images/affiliate_logos/x.png":  "https://h/images/affiliate_logos/x.png",
		"/images/x.png":                 "https://h/images/x.png",
		"https://cdn.example.com/x.png": "https://cdn.example.com/x.png", // already absolute
		"":                             "",
	}
	for in, want := range cases {
		if got := absoluteAssetURL("https://h", in); got != want {
			t.Errorf("absoluteAssetURL(%q) = %q; want %q", in, got, want)
		}
	}
}
