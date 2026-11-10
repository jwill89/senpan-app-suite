package discord

import (
	"net/url"
	"strconv"
	"strings"
)

// AccentColor is the brand accent (pink), an embed's colour unless it sets its own.
const AccentColor = 0xE53170

// Discord's per-field length caps; TruncateRunes enforces them so callers can pass
// raw content without trimming it first.
const (
	EmbedTitleMax       = 256
	EmbedDescriptionMax = 4096
	EmbedFieldValueMax  = 1024
	// MaxEmbedFields is Discord's cap on fields in one embed.
	MaxEmbedFields = 25
	// EmbedTotalMax is Discord's cap on the sum of title, description, every field
	// name and value, and the footer. Past it Discord rejects the whole payload.
	EmbedTotalMax = 6000
)

// TruncatedNotice is added as a final field when content had to be left out, so a
// clipped embed says so in the channel instead of quietly arriving short.
const TruncatedNotice = "Too long to post in full - see the site for the rest."

// NoHeading is a zero-width space, the field name for a field that should render
// without a visible heading (Discord requires a non-empty name).
var NoHeading = string(rune(0x200B))

// EmbedField is one name and value in an embed.
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// EmbedImage is an image in an embed: the small top-right thumbnail, or the large
// one at the bottom.
type EmbedImage struct {
	URL string `json:"url"`
}

// EmbedFooter is the small line at the bottom of an embed.
type EmbedFooter struct {
	Text string `json:"text"`
}

// Embed is the subset of Discord's embed schema Senpan sends.
type Embed struct {
	Title       string       `json:"title,omitempty"`
	Description string       `json:"description,omitempty"`
	Color       int          `json:"color,omitempty"`
	Fields      []EmbedField `json:"fields,omitempty"`
	Thumbnail   *EmbedImage  `json:"thumbnail,omitempty"`
	Image       *EmbedImage  `json:"image,omitempty"`
	Footer      *EmbedFooter `json:"footer,omitempty"`
}

// Payload is a webhook message.
type Payload struct {
	// Content is the plain text above the embed. It carries a role mention
	// (@everyone or <@&id>), since a mention inside an embed doesn't notify.
	Content    string      `json:"content,omitempty"`
	Embeds     []Embed     `json:"embeds"`
	Components []Component `json:"components,omitempty"`
	// AllowedMentions says which mentions in Content ping; without it Discord may
	// suppress a webhook's mention.
	AllowedMentions *AllowedMentions `json:"allowed_mentions,omitempty"`
	// Attachments names the files sent with the payload; PostFile sets it.
	Attachments []attachment `json:"attachments,omitempty"`
}

// attachment names one uploaded file: id is its index among the multipart files.
type attachment struct {
	ID       int    `json:"id"`
	Filename string `json:"filename"`
}

// File is a file posted with a message. An embed shows it as
// attachment://<Name>.
type File struct {
	Name        string
	ContentType string
	Data        []byte
}

// AllowedMentions lists which mentions in a message's content ping: "everyone"
// in Parse enables @everyone and @here, Roles lists role ids that may ping.
type AllowedMentions struct {
	Parse []string `json:"parse"`
	Roles []string `json:"roles,omitempty"`
}

// The message components Senpan sends: an action row holding link buttons.
const (
	ComponentActionRow = 1
	ComponentButton    = 2
	ButtonStyleLink    = 5
	// MaxButtonsPerRow is Discord's cap on buttons in one action row.
	MaxButtonsPerRow = 5
	// ButtonLabelMax is Discord's cap on a button's label.
	ButtonLabelMax = 80
)

// Emoji is the emoji on a button: a unicode character in Name, or a custom guild
// emoji with its ID.
type Emoji struct {
	Name     string `json:"name,omitempty"`
	ID       string `json:"id,omitempty"`
	Animated bool   `json:"animated,omitempty"`
}

// Component is an action row (Components set) or a link button.
type Component struct {
	Type       int         `json:"type"`
	Style      int         `json:"style,omitempty"`
	Label      string      `json:"label,omitempty"`
	URL        string      `json:"url,omitempty"`
	Emoji      *Emoji      `json:"emoji,omitempty"`
	Components []Component `json:"components,omitempty"`
}

// Target labels a post for the log: the feature that posted and which record went
// out.
type Target struct {
	// Kind is the posting feature: "announcement", "announcement_scheduled",
	// "bookclub_item", "affiliate", "tea_room", "ema_plaque" or "ema_request".
	Kind string
	// Name is the record's own name, empty when it has none worth logging.
	Name string
}

// LinkButtonRow builds one action row of link buttons from (label, emoji, url)
// triples, skipping any with a blank label or a URL that isn't http(s) and keeping
// the first five. It returns nil when no button remains, so a caller can leave
// Components out.
func LinkButtonRow(buttons []struct{ Label, Emoji, URL string }) []Component {
	row := make([]Component, 0, MaxButtonsPerRow)
	for _, b := range buttons {
		label := strings.TrimSpace(b.Label)
		link := strings.TrimSpace(b.URL)
		if label == "" || !IsHTTPURL(link) {
			continue
		}
		btn := Component{
			Type:  ComponentButton,
			Style: ButtonStyleLink,
			Label: TruncateRunes(label, ButtonLabelMax),
			URL:   link,
		}
		if e := ParseEmoji(b.Emoji); e != nil {
			btn.Emoji = e
		}
		row = append(row, btn)
		if len(row) >= MaxButtonsPerRow {
			break
		}
	}
	if len(row) == 0 {
		return nil
	}
	return []Component{{Type: ComponentActionRow, Components: row}}
}

// ParseEmoji turns an emoji string into Discord's emoji object: a custom-emoji token
// "<:name:id>" or "<a:name:id>" becomes name, id and animated; anything else is a
// unicode emoji. It returns nil for a blank or malformed input.
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

// EmbedBuilder assembles an Embed. Empty values are skipped and text is cut to
// Discord's limits, so callers pass raw content.
type EmbedBuilder struct {
	embed Embed
	// dropped records that a field was left out, for the 25-field cap or the
	// 6000-character total; Build then adds a notice.
	dropped bool
}

// NewEmbed starts a builder with the brand accent colour.
func NewEmbed() *EmbedBuilder {
	return &EmbedBuilder{embed: Embed{Color: AccentColor}}
}

// Title sets the title.
func (b *EmbedBuilder) Title(s string) *EmbedBuilder {
	b.embed.Title = TruncateRunes(strings.TrimSpace(s), EmbedTitleMax)
	return b
}

// Description sets the description.
func (b *EmbedBuilder) Description(s string) *EmbedBuilder {
	b.embed.Description = TruncateRunes(strings.TrimSpace(s), EmbedDescriptionMax)
	return b
}

// ColorHex sets the colour from "#rrggbb" or "rrggbb", the brand accent when empty
// or invalid.
func (b *EmbedBuilder) ColorHex(hex string) *EmbedBuilder {
	b.embed.Color = ColorFromHex(hex, AccentColor)
	return b
}

// Field appends a field. Fields render in call order; Discord lays inline fields
// side by side, up to three a row. A blank value is skipped, and a field that would
// pass the 25-field cap or the 6000-character total is left out, so callers append
// freely.
func (b *EmbedBuilder) Field(name, value string, inline bool) *EmbedBuilder {
	value = strings.TrimSpace(value)
	if value == "" {
		return b
	}
	if len(b.embed.Fields) >= MaxEmbedFields {
		b.dropped = true
		return b
	}
	value = TruncateRunes(value, EmbedFieldValueMax)
	// Room is kept for the trailing notice, so a clipped embed can say so without
	// itself passing the limit.
	if b.charCount()+len([]rune(name))+len([]rune(value)) > EmbedTotalMax-len([]rune(TruncatedNotice))-len([]rune(NoHeading)) {
		b.dropped = true
		return b
	}
	b.embed.Fields = append(b.embed.Fields, EmbedField{Name: name, Value: value, Inline: inline})
	return b
}

// charCount is Discord's accounting for the 6000-character total.
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

// Thumbnail sets the small top-right image, for an absolute http(s) URL (percent-
// encoded where it needs it, see NormalizeEmbedURL).
func (b *EmbedBuilder) Thumbnail(rawURL string) *EmbedBuilder {
	if u, ok := NormalizeEmbedURL(rawURL); ok {
		b.embed.Thumbnail = &EmbedImage{URL: u}
	}
	return b
}

// Image sets the large bottom image, as Thumbnail does.
func (b *EmbedBuilder) Image(rawURL string) *EmbedBuilder {
	if u, ok := NormalizeEmbedURL(rawURL); ok {
		b.embed.Image = &EmbedImage{URL: u}
	}
	return b
}

// Footer sets the footer line, skipping it when blank.
func (b *EmbedBuilder) Footer(text string) *EmbedBuilder { //nolint:unparam // returns the builder like every other step, so a chain can carry on past it
	if strings.TrimSpace(text) != "" {
		b.embed.Footer = &EmbedFooter{Text: text}
	}
	return b
}

// Build returns the embed, with a visible notice appended when content was left out:
// an announcement would otherwise arrive looking complete with its later details
// gone.
func (b *EmbedBuilder) Build() Embed {
	if b.dropped && len(b.embed.Fields) < MaxEmbedFields {
		b.embed.Fields = append(b.embed.Fields, EmbedField{Name: NoHeading, Value: TruncatedNotice})
	}
	return b.embed
}

// ColorFromHex parses "#rrggbb" or "rrggbb" into Discord's integer colour, def when
// the input is empty or not a 24-bit hex.
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

// IsHTTPURL reports whether u is an http(s) URL; Discord needs absolute URLs for an
// embed's images.
func IsHTTPURL(u string) bool {
	return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://")
}

// NormalizeEmbedURL trims raw and, for an http(s) URL, percent-encodes whatever
// isn't URL-safe: a space in an image's file name becomes %20. Discord rejects the
// whole embed when an image URL isn't well-formed, and a URL stored raw from a file
// named with spaces would be. An already-encoded URL comes back unchanged. It
// returns ("", false) when raw isn't a usable absolute http(s) URL.
func NormalizeEmbedURL(raw string) (string, bool) {
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

// TruncateRunes cuts s to n runes, the ellipsis counted within n, so a cut field
// never passes the limit it was cut to meet.
func TruncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	if n <= 3 {
		return string(runes[:n])
	}
	return string(runes[:n-3]) + "..."
}

// validTimeFormats are the style letters of a Discord timestamp: t short time, T
// long time, d short date, D long date, f short date and time, F long date and time,
// R relative.
var validTimeFormats = map[string]bool{
	"t": true, "T": true, "d": true, "D": true, "f": true, "F": true, "R": true,
}

// TimeStyle is format as a Discord timestamp style letter, def when it is empty or
// not one.
func TimeStyle(format, def string) string {
	format = strings.TrimSpace(format)
	if validTimeFormats[format] {
		return format
	}
	return def
}
