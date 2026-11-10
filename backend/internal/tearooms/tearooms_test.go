package tearooms

import (
	"strings"
	"testing"
	"time"

	"app-suite/internal/discord"
)

func TestHashtags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"cozy private", "#cozy #private"},
		{"#cozy #private", "#cozy #private"},
		{"cozy, private, vip", "#cozy #private #vip"},
		{"  cozy   #private  ", "#cozy #private"},
		{"cozy Cozy COZY", "#cozy"}, // case-insensitive dedupe, keeps first casing
		{"##double", "#double"},     // strips all leading hashes
	}
	for _, c := range cases {
		if got := Hashtags(c.in); got != c.want {
			t.Errorf("Hashtags(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestCapitalizeHashtags(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"#cozy #private", "#Cozy #Private"},
		{"#vip", "#Vip"},
		{"#Cozy #PRIVATE", "#Cozy #PRIVATE"}, // only the first letter is forced up
	}
	for _, c := range cases {
		if got := capitalizeHashtags(c.in); got != c.want {
			t.Errorf("capitalizeHashtags(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestFormatGil(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{125, "125"},
		{1000, "1,000"},
		{125000, "125,000"},
		{1234567, "1,234,567"},
		{-30000, "-30,000"},
	}
	for _, c := range cases {
		if got := FormatGil(c.in); got != c.want {
			t.Errorf("FormatGil(%d) = %q; want %q", c.in, got, c.want)
		}
	}
}

// fieldByName returns the embed field with the given name, or nil.
func fieldByName(e discord.Embed, name string) *discord.EmbedField {
	for i := range e.Fields {
		if e.Fields[i].Name == name {
			return &e.Fields[i]
		}
	}
	return nil
}

func TestEmbed(t *testing.T) {
	embed := Embed(Room{
		Name:            "Jasmine Room",
		RoomNumber:      "1",
		CostPerHalfHour: 125000,
		Hashtags:        "#cozy #private",
		Description:     "A quiet room.",
		Open:            true,
		Color:           "#abcdef",
	}, "https://example.test")

	if embed.Title != "Jasmine Room" {
		t.Errorf("title = %q", embed.Title)
	}
	if !strings.Contains(embed.Description, "A quiet room.") {
		t.Errorf("description missing body: %q", embed.Description)
	}
	// Hashtags render in the footer, capitalized - not in the description.
	if strings.Contains(embed.Description, "#cozy") {
		t.Errorf("hashtags should not be in the description: %q", embed.Description)
	}
	if embed.Footer == nil || embed.Footer.Text != "#Cozy #Private" {
		t.Errorf("footer = %+v; want #Cozy #Private", embed.Footer)
	}
	if f := fieldByName(embed, "💰 Cost"); f == nil || f.Value != "125,000 gil/half hour" || !f.Inline {
		t.Errorf("cost field = %+v", f)
	}
	if f := fieldByName(embed, "🔢 Room Number"); f == nil || f.Value != "1" || !f.Inline {
		t.Errorf("room number field = %+v", f)
	}
	if f := fieldByName(embed, "🚪 Status"); f == nil || f.Value != "Open" || !f.Inline {
		t.Errorf("status field = %+v", f)
	}
}

func TestEmbedDiscountedAndClosed(t *testing.T) {
	embed := Embed(Room{Name: "Sale Room", CostPerHalfHour: 125000, Discounted: true}, "https://example.test")

	cost := fieldByName(embed, "💰 Cost")
	if cost == nil {
		t.Fatal("missing cost field")
	}
	// Full price struck through, halved price shown, plus the discount note.
	if !strings.Contains(cost.Value, "~~125,000 gil~~") ||
		!strings.Contains(cost.Value, "62,500 gil") ||
		!strings.Contains(cost.Value, "Currently Discounted!") {
		t.Errorf("discounted cost value = %q", cost.Value)
	}
	if f := fieldByName(embed, "🚪 Status"); f == nil || f.Value != "Closed" {
		t.Errorf("status field = %+v", f)
	}
}

func TestEmbedImageIsAbsolute(t *testing.T) {
	cases := []struct{ image, want string }{
		{"images/rooms/a.png", "https://example.test/images/rooms/a.png"},
		{"/images/rooms/a.png", "https://example.test/images/rooms/a.png"},
		{"https://cdn.example/a.png", "https://cdn.example/a.png"},
	}
	for _, c := range cases {
		embed := Embed(Room{Name: "R", Image: c.image}, "https://example.test/")
		if embed.Image == nil || embed.Image.URL != c.want {
			t.Errorf("image %q: got %+v, want %s", c.image, embed.Image, c.want)
		}
	}
}

// ParseLockTime accepts RFC 3339 only, because every stored expiry has been through
// NormalizeLockUntil: a naive local-looking string is rejected rather than guessed at
// as UTC.
func TestParseLockTime(t *testing.T) {
	at, ok := ParseLockTime("2030-01-02T15:04:05Z")
	if !ok || !at.Equal(time.Date(2030, 1, 2, 15, 4, 5, 0, time.UTC)) {
		t.Errorf("RFC-3339 = %v, %v", at, ok)
	}
	// An offset is normalized to UTC.
	at, ok = ParseLockTime("2030-01-02T10:04:05-05:00")
	if !ok || !at.Equal(time.Date(2030, 1, 2, 15, 4, 5, 0, time.UTC)) {
		t.Errorf("offset form = %v, %v", at, ok)
	}
	for _, bad := range []string{"", "2030-01-02T15:04", "tomorrow", "2030-01-02"} {
		if _, ok := ParseLockTime(bad); ok {
			t.Errorf("ParseLockTime(%q) accepted", bad)
		}
	}
}

func TestNormalizeLockUntil(t *testing.T) {
	cases := []struct {
		locked    bool
		in, want  string
		wantValid bool
	}{
		{false, "2030-01-02T15:04:05Z", "", true},                        // unlocking drops the expiry
		{false, "tomorrow", "", true},                                    // ...and never reads it
		{true, "", "", true},                                             // a lock with no expiry
		{true, "  ", "", true},                                           // blank is none
		{true, "2030-01-02T15:04:05.000Z", "2030-01-02T15:04:05Z", true}, // the browser's form
		{true, "2030-01-02T10:04:05-05:00", "2030-01-02T15:04:05Z", true},
		{true, "tomorrow evening", "", false},
		{true, "2000-01-01T00:00:00Z", "2000-01-01T00:00:00Z", true}, // already past is accepted
	}
	for _, c := range cases {
		got, ok := NormalizeLockUntil(c.locked, c.in)
		if got != c.want || ok != c.wantValid {
			t.Errorf("NormalizeLockUntil(%v, %q) = %q, %v; want %q, %v", c.locked, c.in, got, ok, c.want, c.wantValid)
		}
	}
}
