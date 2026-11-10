// Package tearooms holds the rules of the Senpan Tea House's bookable rooms that need
// no database or network: the hashtag list, the gil price, a room's timed lock and the
// Discord embed a room is posted as.
package tearooms

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"app-suite/internal/discord"
)

// MaxHashtags caps how many hashtags a room keeps.
const MaxHashtags = 30

// Room is what a room's Discord embed is made of.
type Room struct {
	Name            string
	RoomNumber      string
	CostPerHalfHour int64
	Hashtags        string
	Description     string
	Open            bool
	Discounted      bool
	Image           string
	Color           string
}

// Hashtags turns free-form input ("cozy, private" or "#cozy #private") into a
// space-separated "#tag" list: split on commas and whitespace, a leading '#' dropped
// and put back, blanks and case-insensitive repeats dropped, at most MaxHashtags kept.
func Hashtags(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || unicode.IsSpace(r)
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		tag := strings.TrimSpace(strings.TrimLeft(f, "#"))
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, "#"+tag)
		if len(out) >= MaxHashtags {
			break
		}
	}
	return strings.Join(out, " ")
}

// capitalizeHashtags upper-cases the first letter of each "#tag" in a normalized
// list; the rest of each tag is left as stored.
func capitalizeHashtags(tags string) string {
	fields := strings.Fields(tags)
	for i, f := range fields {
		tag := strings.TrimPrefix(f, "#")
		if tag == "" {
			continue
		}
		r := []rune(tag)
		r[0] = unicode.ToUpper(r[0])
		fields[i] = "#" + string(r)
	}
	return strings.Join(fields, " ")
}

// FormatGil writes a gil amount with thousands separators: 125000 is "125,000". A
// negative amount keeps its sign.
func FormatGil(n int64) string {
	s := strconv.FormatInt(n, 10)
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	var b strings.Builder
	for i := range len(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(s[i])
	}
	return sign + b.String()
}

// ParseLockTime parses a lock expiry into a UTC instant. Stored expiries are all
// RFC 3339, so that is the only form read; anything else is a value this server
// never wrote.
func ParseLockTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// NormalizeLockUntil is a lock expiry in the form it is stored in, and whether it was
// acceptable. An unlocked room has no expiry, so a stale time can't outlive its lock;
// a locked room may have none, which is a lock that stands until someone lifts it. A
// time already past is accepted: the lock is over and the next sweep lifts it, so a
// form sent a moment after its own expiry doesn't fail on an unrelated edit.
func NormalizeLockUntil(locked bool, lockedUntil string) (string, bool) {
	if !locked {
		return "", true
	}
	s := strings.TrimSpace(lockedUntil)
	if s == "" {
		return "", true
	}
	at, ok := ParseLockTime(s)
	if !ok {
		return "", false
	}
	return at.UTC().Format("2006-01-02T15:04:05Z"), true
}

// Embed renders a room as a Discord embed: its name as the title, its Markdown
// description as the body, then inline fields for the cost per half hour (halved, with
// a note, when discounted), the room number and open or closed. Hashtags go in the
// footer, capitalized, and the image runs full width at the bottom. baseURL makes a
// root-relative image address absolute.
func Embed(r Room, baseURL string) discord.Embed {
	b := discord.NewEmbed().Title(r.Name).ColorHex(r.Color)
	b.Description(discord.Markdown(strings.TrimSpace(r.Description)))

	if r.Discounted {
		b.Field("💰 Cost", fmt.Sprintf("~~%s gil~~ **%s gil**/half hour\n**Currently Discounted!**",
			FormatGil(r.CostPerHalfHour), FormatGil(r.CostPerHalfHour/2)), true)
	} else {
		b.Field("💰 Cost", fmt.Sprintf("%s gil/half hour", FormatGil(r.CostPerHalfHour)), true)
	}
	b.Field("🔢 Room Number", r.RoomNumber, true)
	status := "Closed"
	if r.Open {
		status = "Open"
	}
	b.Field("🚪 Status", status, true)

	if tags := strings.TrimSpace(r.Hashtags); tags != "" {
		b.Footer(capitalizeHashtags(tags))
	}
	b.Image(absoluteURL(baseURL, r.Image))
	return b.Build()
}

// absoluteURL makes a root-relative path absolute under baseURL; an address that is
// already absolute, or empty, is returned as it is.
func absoluteURL(baseURL, path string) string {
	path = strings.TrimSpace(path)
	if path == "" || discord.IsHTTPURL(path) {
		return path
	}
	return strings.TrimRight(baseURL, "/") + "/" + strings.TrimPrefix(path, "/")
}
