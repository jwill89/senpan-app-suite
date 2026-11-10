// Package announcements holds the rules for announcements that need no database or
// network: checking what a form sends and working out the schedule it implies, and
// rendering an announcement as the Discord message it posts.
package announcements

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"app-suite/internal/discord"
	"app-suite/internal/schedule"
)

const (
	// MaxSkipCount is the most upcoming occurrences one skip can pass over: a year
	// of a weekly announcement. Past that, deactivating it is the right tool.
	MaxSkipCount = 52
	// MaxButtons is how many link buttons an announcement carries, the most that fit
	// one Discord action row.
	MaxButtons = discord.MaxButtonsPerRow
	// DefaultStartFormat and DefaultEndFormat are the Discord timestamp styles used
	// when an announcement picks none: a full date and time for the start, and just
	// the time for the end, which usually falls on the same day.
	DefaultStartFormat = "F"
	DefaultEndFormat   = "t"
)

// Mention values: none, everyone, or a managed role by id.
const (
	MentionNone     = ""
	MentionEveryone = "everyone"
	mentionRole     = "role:"
)

// IsSnowflake reports whether s is a Discord id: one or more digits.
func IsSnowflake(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// Button is a Discord link button under the embed.
type Button struct {
	Label string
	Emoji string
	URL   string
}

// SanitizeButtons trims each button and keeps those with a label and an http(s) URL,
// at most MaxButtons, in order.
func SanitizeButtons(in []Button) []Button {
	out := make([]Button, 0, min(len(in), MaxButtons))
	for _, b := range in {
		label, link := strings.TrimSpace(b.Label), strings.TrimSpace(b.URL)
		if label == "" || !discord.IsHTTPURL(link) {
			continue
		}
		out = append(out, Button{Label: label, Emoji: strings.TrimSpace(b.Emoji), URL: link})
		if len(out) >= MaxButtons {
			break
		}
	}
	return out
}

// RoleMention is the mention that tags the managed role with the given id.
func RoleMention(id int64) string {
	return mentionRole + strconv.FormatInt(id, 10)
}

// ParseMention reads a mention value. everyone says it tags @everyone; role is the
// managed role's id when it tags one, 0 otherwise; ok is false for a value that is
// none of the three forms.
func ParseMention(mention string) (everyone bool, role int64, ok bool) {
	switch mention {
	case MentionNone:
		return false, 0, true
	case MentionEveryone:
		return true, 0, true
	}
	rest, isRole := strings.CutPrefix(mention, mentionRole)
	id, err := strconv.ParseInt(rest, 10, 64)
	if !isRole || err != nil || id <= 0 {
		return false, 0, false
	}
	return false, id, true
}

// Problem is something wrong with one field of an announcement.
type Problem struct {
	// Field is the JSON name of the field.
	Field   string
	Value   any
	Message string
}

// Fields are the editable fields of an announcement that the rules read.
type Fields struct {
	Title               string
	Details             string
	Image               string
	Thumbnail           string
	Color               string
	Location            string
	StartFormat         string
	EndFormat           string
	StartLocal          string
	EndLocal            string
	OnceLocal           string
	Buttons             []Button
	Mention             string
	ScheduleKind        string
	Timezone            string
	ScheduleMinutes     int
	ScheduleWeekdays    string
	ScheduleWeekOfMonth int
}

// Cursor is a stored announcement's schedule position.
type Cursor struct {
	NextPostAt string
	Active     bool
}

// Resolved is a checked announcement: its fields as they are stored, and the
// absolute instants worked out from them.
type Resolved struct {
	Fields
	// StartAt and EndAt are the event window as UTC instants, "" for none.
	StartAt string
	EndAt   string
	// NextPostAt is when the schedule next posts, "" for none, and Active whether the
	// schedule is live.
	NextPostAt string
	Active     bool
}

// Resolve checks f and resolves every wall-clock time against its one timezone,
// computing the event window and the next post. existing is the stored schedule
// position on an edit, nil on a create.
//
// The position belongs to the scheduler, not the form, as a pending skip does: a
// one-time announcement that has gone out is left with no next post and inactive,
// and an edit that re-armed it from the date still in the form would have it posted
// again on the next sweep. So an edit arms a one-time schedule only when its date is
// in the future; moving it there is still a deliberate re-arm. A create keeps the
// date whatever it is, so a past date posts on the next sweep.
func Resolve(f Fields, existing *Cursor, now time.Time) (Resolved, []Problem) {
	var problems []Problem
	bad := func(field string, value any, msg string) {
		problems = append(problems, Problem{Field: field, Value: value, Message: msg})
	}

	f.Title = strings.TrimSpace(f.Title)
	f.Details = strings.TrimSpace(f.Details)
	f.Image = strings.TrimSpace(f.Image)
	f.Thumbnail = strings.TrimSpace(f.Thumbnail)
	f.Color = strings.TrimSpace(f.Color)
	f.Location = strings.TrimSpace(f.Location)
	f.StartFormat = discord.TimeStyle(f.StartFormat, DefaultStartFormat)
	f.EndFormat = discord.TimeStyle(f.EndFormat, DefaultEndFormat)
	f.StartLocal = strings.TrimSpace(f.StartLocal)
	f.EndLocal = strings.TrimSpace(f.EndLocal)
	f.OnceLocal = strings.TrimSpace(f.OnceLocal)
	f.Buttons = SanitizeButtons(f.Buttons)
	f.Mention = strings.TrimSpace(f.Mention)
	f.ScheduleKind = strings.TrimSpace(f.ScheduleKind)
	f.Timezone = strings.TrimSpace(f.Timezone)
	f.ScheduleWeekdays = strings.TrimSpace(f.ScheduleWeekdays)

	if f.Title == "" {
		bad("title", f.Title, "A title is required.")
	}
	if f.Details == "" {
		bad("details", f.Details, "Details are required.")
	}
	if _, _, ok := ParseMention(f.Mention); !ok {
		bad("mention", f.Mention, `The role tag must be empty, "everyone", or "role:" and a role id.`)
	}
	if !schedule.ValidKind(f.ScheduleKind) {
		bad("scheduleKind", f.ScheduleKind, "The schedule must be once, daily, weekly, monthly or empty.")
		return Resolved{Fields: f}, problems
	}

	// One timezone anchors every wall-clock time, so it is required whenever there is
	// one (the event window or a schedule).
	r := Resolved{Fields: f}
	loc := time.UTC
	if f.StartLocal != "" || f.EndLocal != "" || f.ScheduleKind != "" {
		l, err := time.LoadLocation(f.Timezone)
		switch {
		case f.Timezone == "":
			bad("timezone", f.Timezone, "A timezone is required when an announcement has any times.")
			return r, problems
		case err != nil:
			bad("timezone", f.Timezone, "That isn't an IANA timezone, such as America/Chicago.")
			return r, problems
		}
		loc = l
	}

	var err error
	if r.StartAt, err = schedule.ResolveLocal(f.StartLocal, loc); err != nil {
		bad("startLocal", f.StartLocal, "That isn't a date and time.")
	}
	if r.EndAt, err = schedule.ResolveLocal(f.EndLocal, loc); err != nil {
		bad("endLocal", f.EndLocal, "That isn't a date and time.")
	}

	switch f.ScheduleKind {
	case "":
		r.NextPostAt, r.Active = "", false
	case schedule.Once:
		if f.OnceLocal == "" {
			bad("onceLocal", f.OnceLocal, "A date and time is required for a one-time schedule.")
			break
		}
		next, err := schedule.ResolveLocal(f.OnceLocal, loc)
		if err != nil {
			bad("onceLocal", f.OnceLocal, "That isn't a date and time.")
			break
		}
		r.NextPostAt, r.Active = next, true
		if existing != nil {
			if t, perr := time.Parse(time.RFC3339, next); perr != nil || !t.After(now) {
				r.NextPostAt, r.Active = existing.NextPostAt, existing.Active
			}
		}
	default:
		// The time of day is checked first so it can't wrap through the split into
		// hours and minutes and land at the wrong hour.
		if f.ScheduleMinutes < 0 || f.ScheduleMinutes > schedule.MaxMinutes {
			bad("scheduleMinutes", f.ScheduleMinutes, "The time of day must be within a day (0 to 1439 minutes).")
		}
		if f.ScheduleKind == schedule.Monthly && !schedule.ValidWeekOfMonth(f.ScheduleWeekOfMonth) {
			bad("scheduleWeekOfMonth", f.ScheduleWeekOfMonth, "The week of the month must be 1 to 5, or -1 for the last.")
		}
		if len(problems) > 0 {
			break
		}
		next := schedule.Next(f.Rule(), now)
		if next == "" {
			bad("scheduleWeekdays", f.ScheduleWeekdays, "Pick the day or days the schedule recurs on.")
			break
		}
		r.NextPostAt, r.Active = next, true
	}
	return r, problems
}

// Rule is the recurrence these fields describe.
func (f Fields) Rule() schedule.Rule {
	return schedule.Rule{
		Kind: f.ScheduleKind, Timezone: f.Timezone, Minutes: f.ScheduleMinutes,
		Weekdays: f.ScheduleWeekdays, WeekOfMonth: f.ScheduleWeekOfMonth,
	}
}

// Announcement is what a post of an announcement shows.
type Announcement struct {
	Title        string
	Details      string
	Image        string
	Thumbnail    string
	Color        string
	Location     string
	StartLocal   string
	EndLocal     string
	StartAt      string
	EndAt        string
	StartFormat  string
	EndFormat    string
	DynamicDates bool
	Timezone     string
	Buttons      []Button
	Mention      string
}

// MentionContent is the message text that tags who the mention names, and the
// allowed-mentions list that lets it ping. roleID is the Discord id of the managed
// role a "role:" mention names, "" when it no longer resolves: the announcement then
// posts without a tag.
func MentionContent(mention, roleID string) (string, *discord.AllowedMentions) {
	everyone, role, _ := ParseMention(mention)
	switch {
	case everyone:
		return "@everyone", &discord.AllowedMentions{Parse: []string{"everyone"}}
	case role > 0 && strings.TrimSpace(roleID) != "":
		return "<@&" + roleID + ">", &discord.AllowedMentions{Parse: []string{}, Roles: []string{roleID}}
	}
	return "", nil
}

// Message is the webhook payload for a: the role tag in the message text above the
// embed, where a mention notifies, the embed, and the link buttons in an action row.
// now re-anchors a dynamic-dates event onto the day of the post.
func Message(a Announcement, roleID string, now time.Time) discord.Payload {
	content, allowed := MentionContent(a.Mention, roleID)
	a.StartAt, a.EndAt = EventTimes(a, now)
	return discord.Payload{
		Content:         content,
		Embeds:          []discord.Embed{Embed(a)},
		Components:      components(a.Buttons),
		AllowedMentions: allowed,
	}
}

// components is the buttons as one action row, nil when there are none.
func components(buttons []Button) []discord.Component {
	if len(buttons) == 0 {
		return nil
	}
	triples := make([]struct{ Label, Emoji, URL string }, 0, len(buttons))
	for _, b := range buttons {
		triples = append(triples, struct{ Label, Emoji, URL string }{b.Label, b.Emoji, b.URL})
	}
	return discord.LinkButtonRow(triples)
}

// EventTimes are the start and end instants the embed shows. With dynamic dates on
// and a start set, the local start and end are a template: their time of day, and
// how many days the end runs past the start, move onto the day of ref in the
// announcement's timezone. Otherwise, or if a time doesn't read, the stored instants
// stand.
func EventTimes(a Announcement, ref time.Time) (startAt, endAt string) {
	if !a.DynamicDates || strings.TrimSpace(a.StartLocal) == "" {
		return a.StartAt, a.EndAt
	}
	loc := schedule.Location(a.Timezone)
	start, err := schedule.ParseLocal(a.StartLocal, loc)
	if err != nil {
		return a.StartAt, a.EndAt
	}
	day := ref.In(loc)
	newStart := time.Date(day.Year(), day.Month(), day.Day(), start.Hour(), start.Minute(), 0, 0, loc)
	startAt = newStart.UTC().Format(time.RFC3339)

	if strings.TrimSpace(a.EndLocal) != "" {
		if end, perr := schedule.ParseLocal(a.EndLocal, loc); perr == nil {
			// An end at 1am the next day keeps running one day past the start.
			endDay := newStart.AddDate(0, 0, daysBetween(start, end))
			newEnd := time.Date(endDay.Year(), endDay.Month(), endDay.Day(), end.Hour(), end.Minute(), 0, 0, loc)
			endAt = newEnd.UTC().Format(time.RFC3339)
		}
	}
	return startAt, endAt
}

// daysBetween is the whole days from the calendar date of a to that of b, negative
// when b is earlier. Both are read at noon so daylight saving can't shift a day.
func daysBetween(a, b time.Time) int {
	da := time.Date(a.Year(), a.Month(), a.Day(), 12, 0, 0, 0, a.Location())
	db := time.Date(b.Year(), b.Month(), b.Day(), 12, 0, 0, 0, b.Location())
	return int(math.Round(db.Sub(da).Hours() / 24))
}

// Embed renders a as a Discord embed. The times come first, as one full-width field
// of Discord timestamp tokens in the styles the author chose, so each viewer sees
// their own zone; then the location on its own line; then the Markdown details, in
// Discord's flavor, as headingless full-width fields. Details past a field's cap are
// split across consecutive fields rather than cut, which keeps the times above them
// (a description would always draw above the fields). The thumbnail sits small at
// the top right and the image full width at the bottom.
func Embed(a Announcement) discord.Embed {
	b := discord.NewEmbed().Title(a.Title).ColorHex(a.Color)

	hasTime := false
	if startT, err := time.Parse(time.RFC3339, a.StartAt); err == nil {
		value := fmt.Sprintf("<t:%d:%s>", startT.Unix(), discord.TimeStyle(a.StartFormat, DefaultStartFormat))
		if endT, err := time.Parse(time.RFC3339, a.EndAt); err == nil {
			value += fmt.Sprintf(" to <t:%d:%s>", endT.Unix(), discord.TimeStyle(a.EndFormat, DefaultEndFormat))
		}
		b.Field("🗓️ When", value, false)
		hasTime = true
	}
	// Not inline, so "When" and "Where" stack rather than sit side by side.
	b.Field("📍 Where", a.Location, false)
	for _, chunk := range SplitForFields(discord.Markdown(a.Details), discord.EmbedFieldValueMax) {
		b.Field(discord.NoHeading, chunk, false)
	}
	b.Thumbnail(a.Thumbnail)
	b.Image(a.Image)
	if hasTime {
		b.Footer("Times shown in your local time zone.")
	}
	return b.Build()
}

// SplitForFields splits text into chunks of at most limit runes, the cap on an
// embed field's value. A chunk breaks at the last newline in its window, else the
// last space, else mid-word, and is trimmed of the whitespace it broke on. Empty
// text gives nil, and text within the cap one chunk.
//
// The SPA mirrors this in lib/announcementDetails.ts to warn when details will be
// split; the two must agree.
func SplitForFields(text string, limit int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > 0 {
		if len(runes) <= limit {
			if c := strings.TrimSpace(string(runes)); c != "" {
				chunks = append(chunks, c)
			}
			break
		}
		// A break at index 0 makes no progress, so it falls through to a hard cut.
		window := runes[:limit]
		cut := lastIndex(window, '\n')
		if cut <= 0 {
			cut = lastIndex(window, ' ')
		}
		if cut <= 0 {
			cut = limit
		}
		if c := strings.TrimSpace(string(runes[:cut])); c != "" {
			chunks = append(chunks, c)
		}
		runes = runes[cut:]
	}
	return chunks
}

// lastIndex is the index of the last r in runes, or -1.
func lastIndex(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i
		}
	}
	return -1
}
