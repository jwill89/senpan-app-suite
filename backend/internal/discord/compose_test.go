package discord_test

import (
	"strings"
	"testing"

	"app-suite/internal/discord"
)

// TestTruncateRespectsTheLimit pins the arithmetic. The ellipsis counts toward the cap, and
// keeping n-1 runes before appending three more returned n+2 - so every truncated field came
// back OVER the Discord limit it was cut to satisfy, and Discord rejects the whole message.
func TestTruncateRespectsTheLimit(t *testing.T) {
	cases := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under the limit is untouched", "hello", 10, "hello"},
		{"exactly at the limit is untouched", "hello", 5, "hello"},
		{"truncates to exactly n runes", "abcdefghij", 8, "abcde..."},
		{"no room for an ellipsis", "abcdefghij", 3, "abc"},
		{"zero", "abcdefghij", 0, ""},
		// Multi-byte: the cap is in RUNES, and slicing by byte would split these.
		{"japanese truncates by rune", "あいうえおかきくけこ", 6, "あいう..."},
	}
	for _, c := range cases {
		got := discord.Truncate(c.in, c.n)
		if got != c.want {
			t.Errorf("%s: Truncate(%q, %d) = %q; want %q", c.name, c.in, c.n, got, c.want)
		}
		if n := len([]rune(got)); n > c.n {
			t.Errorf("%s: result is %d runes, over the limit of %d", c.name, n, c.n)
		}
	}
}

// TestEmbedRespectsDiscordTotalBudget pins the 6000-character cap. Discord sums title,
// description, every field name and value, and the footer, and rejects the WHOLE message over
// 6000. The 25-field cap was enforced but dropped silently, which is worse than failing: the
// announcement arrived looking complete.
func TestEmbedRespectsDiscordTotalBudget(t *testing.T) {
	b := discord.NewEmbed().Title("A long announcement")
	big := strings.Repeat("x", discord.FieldValueMax)
	for range 20 {
		b.Field(discord.NoHeading, big, false)
	}
	e := b.Build()

	total := len([]rune(e.Title)) + len([]rune(e.Description))
	for _, f := range e.Fields {
		total += len([]rune(f.Name)) + len([]rune(f.Value))
	}
	if total > discord.TotalMax {
		t.Errorf("embed totals %d characters; Discord rejects anything over %d", total, discord.TotalMax)
	}
	if len(e.Fields) > discord.MaxFields {
		t.Errorf("embed carries %d fields; Discord caps them at %d", len(e.Fields), discord.MaxFields)
	}
	// And it must SAY it was clipped rather than quietly arriving short.
	if last := e.Fields[len(e.Fields)-1]; last.Value != discord.TruncatedNotice {
		t.Errorf("last field = %q; want the truncation notice so the clip is visible", last.Value)
	}
}

// TestEmbedWithinBudgetIsUntouched guards the other direction: an ordinary embed must not
// gain a truncation notice it did not earn.
func TestEmbedWithinBudgetIsUntouched(t *testing.T) {
	e := discord.NewEmbed().Title("Tea time").Field("When", "Friday", false).Build()
	if len(e.Fields) != 1 || e.Fields[0].Value != "Friday" {
		t.Fatalf("fields = %+v; want the one field left alone", e.Fields)
	}
}

// TestNormalizeURL covers the Discord 400 "embeds invalid" bug: an image URL whose file name
// has a space (or non-ASCII) must be percent-encoded, encoding must be idempotent, and a
// non-http value must be refused so the caller leaves the image off.
func TestNormalizeURL(t *testing.T) {
	cases := []struct {
		name   string
		in     string
		want   string
		wantOK bool
	}{
		{"space in filename", "https://x.co/images/a/Summer Event.png", "https://x.co/images/a/Summer%20Event.png", true},
		{"non-ascii filename", "https://x.co/images/a/café.png", "https://x.co/images/a/caf%C3%A9.png", true},
		{"already encoded is unchanged", "https://x.co/images/a/Summer%20Event.png", "https://x.co/images/a/Summer%20Event.png", true},
		{"plain url untouched", "https://x.co/images/a/normal.png", "https://x.co/images/a/normal.png", true},
		{"relative path rejected", "images/a/pic.png", "", false},
		{"blank rejected", "   ", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := discord.NormalizeURL(tc.in)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("NormalizeURL(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
			}
		})
	}
}

// TestBuildEncodesImageURLs is the end-to-end guard: an embed built with spaced image URLs
// carries the encoded form.
func TestBuildEncodesImageURLs(t *testing.T) {
	e := discord.NewEmbed().
		Title("Hi").
		Image("https://x.co/images/a/My Photo.png").
		Thumbnail("https://x.co/images/a/My Logo.png").
		Build()
	if e.Image == nil || e.Image.URL != "https://x.co/images/a/My%20Photo.png" {
		t.Errorf("image URL = %v, want encoded", e.Image)
	}
	if e.Thumbnail == nil || e.Thumbnail.URL != "https://x.co/images/a/My%20Logo.png" {
		t.Errorf("thumbnail URL = %v, want encoded", e.Thumbnail)
	}
}

// TestLinkButtons keeps only usable buttons, at most five, and reads custom emoji tokens.
func TestLinkButtons(t *testing.T) {
	if got := discord.LinkButtons([]discord.LinkButton{{Label: "", URL: "https://x.co"}, {Label: "Bad", URL: "ftp://x"}}); got != nil {
		t.Fatalf("no usable buttons = %+v; want nil", got)
	}
	var many []discord.LinkButton
	for range 7 {
		many = append(many, discord.LinkButton{Label: "Go", URL: "https://x.co"})
	}
	many[0].Emoji = "<a:dance:123>"
	row := discord.LinkButtons(many)
	if len(row) != 1 || len(row[0].Components) != 5 {
		t.Fatalf("row = %+v; want one row of five", row)
	}
	if e := row[0].Components[0].Emoji; e == nil || e.Name != "dance" || e.ID != "123" || !e.Animated {
		t.Errorf("emoji = %+v; want the animated custom emoji", e)
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
		{"", discord.AccentColor},            // empty -> default
		{"not-a-color", discord.AccentColor}, // invalid hex -> default
		{"#ffffffff", discord.AccentColor},   // out of 24-bit range -> default
	}
	for _, c := range cases {
		if got := discord.ColorFromHex(c.in, discord.AccentColor); got != c.want {
			t.Errorf("ColorFromHex(%q) = %#x, want %#x", c.in, got, c.want)
		}
	}
}

func TestParseEmoji(t *testing.T) {
	if e := discord.ParseEmoji("  "); e != nil {
		t.Errorf("blank -> nil, got %+v", e)
	}
	if e := discord.ParseEmoji("🎉"); e == nil || e.Name != "🎉" || e.ID != "" {
		t.Errorf("unicode emoji: %+v", e)
	}
	if e := discord.ParseEmoji("<:wave:123>"); e == nil || e.Name != "wave" || e.ID != "123" || e.Animated {
		t.Errorf("custom emoji: %+v", e)
	}
	if e := discord.ParseEmoji("<a:spin:456>"); e == nil || e.Name != "spin" || e.ID != "456" || !e.Animated {
		t.Errorf("animated custom emoji: %+v", e)
	}
}
