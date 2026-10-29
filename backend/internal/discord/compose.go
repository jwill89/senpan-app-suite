package discord

import (
	"net/url"
	"strconv"
	"strings"
)

// This file builds the embeds and link-button rows the app posts. Feature code assembles a
// message with NewEmbed()... and LinkButtons; the builder enforces Discord's limits so callers
// can pass raw content without measuring it first.

// AccentColor is the brand accent (pink), every embed's default color.
const AccentColor = 0xE53170

// Discord's per-field length caps. The builder enforces them, so callers pass raw content.
const (
	TitleMax       = 256
	DescriptionMax = 4096
	FieldValueMax  = 1024
	MaxFields      = 25 // Discord's hard cap on fields per embed
	// TotalMax is Discord's cap on the SUM of title + description + every field name and
	// value + footer. Over it, Discord rejects the whole message with a 400 - so a long
	// enough announcement used to simply fail to post.
	TotalMax = 6000
)

// TruncatedNotice is appended as a final field when content had to be left out, so a
// clipped embed says so in the channel instead of quietly arriving short. It is counted
// against the budget like any other field.
const TruncatedNotice = "Too long to post in full - see the site for the rest."

// NoHeading is a zero-width space (U+200B), the field name for a field that should show no
// heading (Discord requires a non-empty one).
var NoHeading = string(rune(0x200B))

// Link buttons: Discord's caps on one action row.
const (
	maxButtonsPerRow = 5
	buttonLabelMax   = 80
)

// LinkButton is one link button as an admin enters it: a label, an optional emoji (a unicode
// emoji, or a custom one written "<:name:id>" / "<a:name:id>") and the URL it opens.
type LinkButton struct {
	Label string
	Emoji string
	URL   string
}

// LinkButtons builds one action row of link buttons, skipping any with a blank label or a
// URL that isn't http(s), and keeping at most five. Returns nil when none remain, so the
// message carries no components at all.
func LinkButtons(buttons []LinkButton) []Component {
	row := make([]Component, 0, maxButtonsPerRow)
	for _, b := range buttons {
		label := strings.TrimSpace(b.Label)
		link := strings.TrimSpace(b.URL)
		if label == "" || !IsHTTPURL(link) {
			continue
		}
		row = append(row, Component{
			Type:  ComponentButton,
			Style: ButtonStyleLink,
			Label: Truncate(label, buttonLabelMax),
			URL:   link,
			Emoji: ParseEmoji(b.Emoji),
		})
		if len(row) >= maxButtonsPerRow {
			break
		}
	}
	if len(row) == 0 {
		return nil
	}
	return []Component{{Type: ComponentActionRow, Components: row}}
}

// ParseEmoji turns an emoji string into Discord's emoji object: "<:name:id>" / "<a:name:id>"
// become a custom emoji, anything else a unicode one. Nil for a blank input, or a custom
// token that is malformed.
func ParseEmoji(s string) *Emoji {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		inner := strings.TrimSuffix(strings.TrimPrefix(s, "<"), ">")
		animated := strings.HasPrefix(inner, "a:")
		inner = strings.TrimPrefix(inner, "a")
		inner = strings.TrimPrefix(inner, ":")
		parts := strings.SplitN(inner, ":", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			return &Emoji{Name: parts[0], ID: parts[1], Animated: animated}
		}
		return nil
	}
	return &Emoji{Name: s}
}

// EmbedBuilder assembles an Embed. Empty values are skipped and text is cut to Discord's
// limits, so callers can chain raw content without checking it.
type EmbedBuilder struct {
	embed Embed
	// dropped records that a field was left out - the 25-field cap was reached, or it would
	// have blown the 6000-character total. Build turns it into a visible notice.
	dropped bool
}

// NewEmbed starts an embed in the brand accent color.
func NewEmbed() *EmbedBuilder {
	return &EmbedBuilder{embed: Embed{Color: AccentColor}}
}

// Title sets the title.
func (b *EmbedBuilder) Title(s string) *EmbedBuilder {
	b.embed.Title = Truncate(strings.TrimSpace(s), TitleMax)
	return b
}

// Description sets the body text.
func (b *EmbedBuilder) Description(s string) *EmbedBuilder {
	b.embed.Description = Truncate(strings.TrimSpace(s), DescriptionMax)
	return b
}

// ColorHex sets the color from "#rrggbb" (or "rrggbb"), keeping the brand accent when the
// value is empty or invalid.
func (b *EmbedBuilder) ColorHex(hex string) *EmbedBuilder {
	b.embed.Color = ColorFromHex(hex, AccentColor)
	return b
}

// Field appends a field. Fields render in call order, each with its own inline flag (Discord
// puts up to three inline fields side by side and breaks the row at the next non-inline one).
// A blank value is skipped, and a field past the 25-field cap or the 6000-character total is
// dropped, so callers can append freely.
func (b *EmbedBuilder) Field(name, value string, inline bool) *EmbedBuilder {
	value = strings.TrimSpace(value)
	if value == "" {
		return b
	}
	if len(b.embed.Fields) >= MaxFields {
		b.dropped = true
		return b
	}
	value = Truncate(value, FieldValueMax)
	// Leave room for the trailing notice, so a clipped embed can still say it was clipped
	// without itself pushing the message over the limit.
	if b.charCount()+len([]rune(name))+len([]rune(value)) > TotalMax-len([]rune(TruncatedNotice))-len([]rune(NoHeading)) {
		b.dropped = true
		return b
	}
	b.embed.Fields = append(b.embed.Fields, EmbedField{Name: name, Value: value, Inline: inline})
	return b
}

// charCount is Discord's count toward the 6000-character total: title, description, every
// field name and value, and the footer.
func (b *EmbedBuilder) charCount() int {
	n := len([]rune(b.embed.Title)) + len([]rune(b.embed.Description))
	for _, f := range b.embed.Fields {
		n += len([]rune(f.Name)) + len([]rune(f.Value))
	}
	if b.embed.Footer != nil {
		n += len([]rune(b.embed.Footer.Text))
	}
	return n
}

// Thumbnail sets the small top-right image. Only an absolute http(s) URL is used, with
// unsafe characters (a space in a file name) percent-encoded - see NormalizeURL.
func (b *EmbedBuilder) Thumbnail(rawURL string) *EmbedBuilder {
	if u, ok := NormalizeURL(rawURL); ok {
		b.embed.Thumbnail = &EmbedImage{URL: u}
	}
	return b
}

// Image sets the large image at the bottom, under the same rules as Thumbnail.
func (b *EmbedBuilder) Image(rawURL string) *EmbedBuilder {
	if u, ok := NormalizeURL(rawURL); ok {
		b.embed.Image = &EmbedImage{URL: u}
	}
	return b
}

// Footer sets the footer line, skipping a blank one.
func (b *EmbedBuilder) Footer(text string) *EmbedBuilder {
	if strings.TrimSpace(text) != "" {
		b.embed.Footer = &EmbedFooter{Text: text}
	}
	return b
}

// Build returns the embed, with a visible notice appended when content had to be left out.
// Dropping silently is what made this a bug rather than a limit: an announcement arrived
// looking complete while its later details were simply gone.
func (b *EmbedBuilder) Build() Embed {
	if b.dropped && len(b.embed.Fields) < MaxFields {
		b.embed.Fields = append(b.embed.Fields, EmbedField{Name: NoHeading, Value: TruncatedNotice})
	}
	return b.embed
}

// ColorFromHex parses "#rrggbb" (or "rrggbb") into Discord's integer color, returning def
// when the input is empty or not a 24-bit hex value.
func ColorFromHex(hex string, def int) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if hex == "" {
		return def
	}
	v, err := strconv.ParseInt(hex, 16, 32)
	if err != nil || v < 0 || v > 0xFFFFFF {
		return def
	}
	return int(v)
}

// IsHTTPURL reports whether u is an http(s) URL (Discord needs absolute URLs for images and
// buttons).
func IsHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// NormalizeURL trims raw and, when it is an http(s) URL, percent-encodes whatever isn't
// URL-safe - a space in a file name becomes %20, "café.png" becomes "caf%C3%A9.png". Discord
// rejects the ENTIRE embed (400, "embeds" invalid) for one malformed image URL, so this
// repairs URLs stored raw. Parsing then re-stringifying is idempotent, so an already-encoded
// URL comes back unchanged. ("", false) when raw isn't a usable absolute http(s) URL.
func NormalizeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if !IsHTTPURL(raw) {
		return "", false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	return u.String(), true
}

// Truncate caps s at n runes, ending in "..." when it had to cut.
func Truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	// The ellipsis counts toward the limit. Keeping n-1 runes and appending three more
	// returned n+2, so every truncated field came back OVER the cap it was cut to satisfy -
	// and Discord rejects the whole message with a 400.
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}
