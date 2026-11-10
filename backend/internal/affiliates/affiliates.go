// Package affiliates holds the rules for a partner establishment that need no
// database or network: cleaning what a form sends, and rendering one as a Discord
// embed.
package affiliates

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/discord"
)

// Hour is one opening-hours row: an optional label, a start and an optional end,
// both wall-clock "HH:MM" in the affiliate's timezone.
type Hour struct {
	Label string
	Start string
	End   string
}

// Profile is what an embed shows of an affiliate.
type Profile struct {
	Name        string
	Location    string
	Timezone    string
	Hours       []Hour
	Details     string
	Logo        string
	Screenshot  string
	EmbedColor  string
	DiscordLink string
	CarrdLink   string
}

// SanitizeOwners trims each owner name and drops blanks, keeping the order.
func SanitizeOwners(in []string) []string {
	out := make([]string, 0, len(in))
	for _, o := range in {
		if o = strings.TrimSpace(o); o != "" {
			out = append(out, o)
		}
	}
	return out
}

// SanitizeHours trims each row and drops those without a start time, keeping the
// order.
func SanitizeHours(in []Hour) []Hour {
	out := make([]Hour, 0, len(in))
	for _, h := range in {
		start := strings.TrimSpace(h.Start)
		if start == "" {
			continue
		}
		out = append(out, Hour{
			Label: strings.TrimSpace(h.Label),
			Start: start,
			End:   strings.TrimSpace(h.End),
		})
	}
	return out
}

// NormalizeExternalURL trims a link and puts https:// before one with no http(s)
// scheme, so a stored link is never a script or data URL: it is rendered as an href
// on the public festival map. Blank stays blank.
func NormalizeExternalURL(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || discord.IsHTTPURL(u) {
		return u
	}
	return "https://" + u
}

// Embed renders an affiliate as a Discord embed: its color, name and Markdown
// details, the logo as the thumbnail and the screenshot as the image, then in order,
// each skipped when absent, the location, the opening hours (as Discord time tokens,
// so each viewer sees their own zone) and the Discord and Carrd links side by side.
// baseURL (scheme://host) makes the root-relative image paths absolute; now anchors
// the recurring wall-clock hours on a date.
func Embed(a Profile, baseURL string, now time.Time) discord.Embed {
	b := discord.NewEmbed().Title(a.Name).ColorHex(a.EmbedColor)
	b.Description(discord.Markdown(strings.TrimSpace(a.Details)))

	b.Field("📍 Location", strings.TrimSpace(a.Location), false)

	if hours := formatHours(a, now); hours != "" {
		b.Field("🕒 Open Times", hours, false)
		b.Footer("Times are displayed in your local time zone.")
	}

	if link := NormalizeExternalURL(a.DiscordLink); link != "" {
		b.Field("💬 Discord", "[Open Link]("+link+")", true)
	}
	if link := NormalizeExternalURL(a.CarrdLink); link != "" {
		b.Field("🔗 Carrd", "[Open Link]("+link+")", true)
	}

	b.Thumbnail(absoluteAssetURL(baseURL, a.Logo))
	b.Image(absoluteAssetURL(baseURL, a.Screenshot))
	return b.Build()
}

// formatHours renders the opening hours as newline-separated lines of Discord
// "Short Time" tokens in the affiliate's timezone (UTC when it is blank or
// invalid): "" when no row is usable.
func formatHours(a Profile, now time.Time) string {
	if len(a.Hours) == 0 {
		return ""
	}
	loc, err := time.LoadLocation(strings.TrimSpace(a.Timezone))
	if err != nil || loc == nil {
		loc = time.UTC
	}
	lines := make([]string, 0, len(a.Hours))
	for _, h := range a.Hours {
		start, ok := hhmmToUnix(h.Start, loc, now)
		if !ok {
			continue
		}
		var line strings.Builder
		if label := strings.TrimSpace(h.Label); label != "" {
			line.WriteString("**" + label + "** - ")
		}
		fmt.Fprintf(&line, "<t:%d:t>", start)
		if end, ok := hhmmToUnix(h.End, loc, now); ok {
			fmt.Fprintf(&line, " - <t:%d:t>", end)
		}
		lines = append(lines, line.String())
	}
	return strings.Join(lines, "\n")
}

// hhmmToUnix converts an "HH:MM" wall-clock time in loc, on now's date, to a Unix
// timestamp. The date doesn't show in a short-time token, but anchoring on now keeps
// the daylight-saving offset current.
func hhmmToUnix(hhmm string, loc *time.Location, now time.Time) (int64, bool) {
	hs, ms, found := strings.Cut(strings.TrimSpace(hhmm), ":")
	if !found {
		return 0, false
	}
	h, err1 := strconv.Atoi(hs)
	m, err2 := strconv.Atoi(ms)
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	ref := now.In(loc)
	return time.Date(ref.Year(), ref.Month(), ref.Day(), h, m, 0, 0, loc).Unix(), true
}

// absoluteAssetURL puts baseURL before a root-relative asset path. An absolute
// http(s) URL passes through, and a blank stays blank.
func absoluteAssetURL(baseURL, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || discord.IsHTTPURL(path) {
		return path
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}
