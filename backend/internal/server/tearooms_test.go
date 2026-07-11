package server

import (
	"strings"
	"testing"

	"app-suite/internal/model"
)

func TestNormalizeHashtags(t *testing.T) {
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
		if got := normalizeHashtags(c.in); got != c.want {
			t.Errorf("normalizeHashtags(%q) = %q; want %q", c.in, got, c.want)
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
		if got := formatGil(c.in); got != c.want {
			t.Errorf("formatGil(%d) = %q; want %q", c.in, got, c.want)
		}
	}
}

// fieldByName returns the embed field with the given name, or nil.
func fieldByName(e discordEmbed, name string) *discordEmbedField {
	for i := range e.Fields {
		if e.Fields[i].Name == name {
			return &e.Fields[i]
		}
	}
	return nil
}

func TestBuildTeaRoomEmbed(t *testing.T) {
	embed := buildTeaRoomEmbed(model.TeaRoom{
		Name:            "Jasmine Room",
		RoomNumber:      "1",
		CostPerHalfHour: 125000,
		Hashtags:        "#cozy #private",
		Description:     "A quiet room.",
		Seasonal:        true,
		Open:            true,
		Lockable:        true,
		Color:           "#abcdef",
	})

	if embed.Title != "Jasmine Room" {
		t.Errorf("title = %q", embed.Title)
	}
	if !strings.Contains(embed.Description, "A quiet room.") {
		t.Errorf("description missing body: %q", embed.Description)
	}
	// Hashtags render in the footer, capitalized — not in the description.
	if strings.Contains(embed.Description, "#cozy") {
		t.Errorf("hashtags should not be in the description: %q", embed.Description)
	}
	if embed.Footer == nil || embed.Footer.Text != "#Cozy #Private" {
		t.Errorf("footer = %+v; want #Cozy #Private", embed.Footer)
	}

	// Cost + room number are inline so they sit side by side.
	if f := fieldByName(embed, "💰 Cost"); f == nil || f.Value != "125,000 gil/half hour" || !f.Inline {
		t.Errorf("cost field = %+v", f)
	}
	if f := fieldByName(embed, "🔢 Room Number"); f == nil || f.Value != "1" || !f.Inline {
		t.Errorf("room number field = %+v", f)
	}
	if f := fieldByName(embed, "🔒 Privacy"); f == nil ||
		f.Value != "This room can be locked for an additional 30,000 gil fee." {
		t.Errorf("privacy field = %+v", f)
	}
	if f := fieldByName(embed, "🍂 Availability"); f == nil || f.Value != "Seasonal" || !f.Inline {
		t.Errorf("availability field = %+v", f)
	}
	if f := fieldByName(embed, "🚪 Status"); f == nil || f.Value != "Open" || !f.Inline {
		t.Errorf("status field = %+v", f)
	}
}

func TestBuildTeaRoomEmbedDiscountedAndClosed(t *testing.T) {
	embed := buildTeaRoomEmbed(model.TeaRoom{
		Name:            "Sale Room",
		CostPerHalfHour: 125000,
		Discounted:      true,
		Seasonal:        false,
		Open:            false,
		Lockable:        false,
	})

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
	if f := fieldByName(embed, "🔒 Privacy"); f == nil || f.Value != "This room cannot be locked." {
		t.Errorf("privacy field = %+v", f)
	}
	if f := fieldByName(embed, "🍂 Availability"); f == nil || f.Value != "Permanent" {
		t.Errorf("availability field = %+v", f)
	}
	if f := fieldByName(embed, "🚪 Status"); f == nil || f.Value != "Closed" {
		t.Errorf("status field = %+v", f)
	}
}
