package festival

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"app-suite/internal/discord"
)

// How a plaque was made, and where it stands.
const (
	EmaKindSite   = "site"
	EmaKindArtist = "artist"

	EmaPending   = "pending"
	EmaPosting   = "posting"
	EmaPosted    = "posted"
	EmaRejected  = "rejected"
	EmaRequested = "requested"
	EmaComplete  = "complete"
)

// Limits of a plaque board.
const (
	// MaxArtistLimit bounds a day's artist request limit, well past any one artist's
	// day, so a typo of an extra zero is caught rather than stored.
	MaxArtistLimit = 1000
	// DefaultArtistName is the artist a board names when it names none.
	DefaultArtistName = "Tara"
)

// EmaWindow is one day a board takes plaques on, as a save sends it.
type EmaWindow struct {
	ID          int64
	Label       string
	StartAt     string
	EndAt       string
	WebhookURL  string
	ArtistLimit int64
}

// EmaBoard is a board as a save sends it.
type EmaBoard struct {
	FestivalMapID int64
	TemplateImage string
	DiscordLink   string
	ArtistName    string
	AllowSelf     bool
	AllowSite     bool
	AllowArtist   bool
	Windows       []EmaWindow
}

// FieldError is the reason a board can't be saved and the field to point at,
// written as a path within the board ("windows[1].endAt").
type FieldError struct {
	Field   string
	Message string
}

func (e *FieldError) Error() string { return e.Message }

// CheckBoard validates a board and puts it as it is stored: text trimmed, the artist
// defaulted, a blank day label becoming "Day N", and the times in UTC to the second.
// It returns the first thing wrong.
func CheckBoard(b *EmaBoard) *FieldError {
	b.TemplateImage = strings.TrimSpace(b.TemplateImage)
	b.DiscordLink = strings.TrimSpace(b.DiscordLink)
	b.ArtistName = strings.TrimSpace(b.ArtistName)
	if b.FestivalMapID <= 0 {
		return &FieldError{"festivalMapId", "Pick the festival this board belongs to"}
	}
	if b.ArtistName == "" {
		b.ArtistName = DefaultArtistName
	}
	if !b.AllowSelf && !b.AllowSite && !b.AllowArtist {
		return &FieldError{"allowSelf", "Offer at least one way to write a plaque"}
	}
	// Both of the template's uses, downloading it and designing on it, need it.
	if (b.AllowSelf || b.AllowSite) && b.TemplateImage == "" {
		return &FieldError{"templateImage", "Pick the plaque template image"}
	}
	if b.DiscordLink != "" && !discord.IsHTTPURL(b.DiscordLink) {
		return &FieldError{"discordLink", "The Discord link must be a web address"}
	}
	if len(b.Windows) == 0 {
		return &FieldError{"windows", "Add at least one day to take plaques on"}
	}
	type span struct {
		label      string
		start, end time.Time
	}
	spans := make([]span, 0, len(b.Windows))
	for i := range b.Windows {
		w := &b.Windows[i]
		at := func(field string) string { return fmt.Sprintf("windows[%d].%s", i, field) }
		w.Label = strings.TrimSpace(w.Label)
		w.WebhookURL = strings.TrimSpace(w.WebhookURL)
		if w.Label == "" {
			w.Label = fmt.Sprintf("Day %d", i+1)
		}
		start, err1 := time.Parse(time.RFC3339, strings.TrimSpace(w.StartAt))
		end, err2 := time.Parse(time.RFC3339, strings.TrimSpace(w.EndAt))
		if err1 != nil || err2 != nil {
			return &FieldError{at("startAt"), w.Label + " needs a start and an end"}
		}
		if !end.After(start) {
			return &FieldError{at("endAt"), w.Label + " ends before it starts"}
		}
		if w.WebhookURL != "" && !discord.IsWebhookURL(w.WebhookURL) {
			return &FieldError{at("webhookUrl"), w.Label + "'s webhook isn't a Discord webhook URL"}
		}
		if w.ArtistLimit < 0 || w.ArtistLimit > MaxArtistLimit {
			return &FieldError{at("artistLimit"), fmt.Sprintf("%s's artist request limit must be 0 (no limit) to %d", w.Label, MaxArtistLimit)}
		}
		w.StartAt, w.EndAt = start.UTC().Format(time.RFC3339), end.UTC().Format(time.RFC3339)
		spans = append(spans, span{w.Label, start, end})
	}
	// Overlapping days would leave "which day is this plaque for?" with two answers,
	// and the one-per-day rule with a loophole.
	sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
	for i := 1; i < len(spans); i++ {
		if spans[i].start.Before(spans[i-1].end) {
			return &FieldError{"windows", spans[i-1].label + " and " + spans[i].label + " overlap"}
		}
	}
	return nil
}

// Day is one day of a board, for finding the one that is open.
type Day struct {
	ID         int64
	Start, End time.Time
}

// CurrentDay is the index of the day now falls inside (start inclusive, end
// exclusive), or -1. Days never overlap.
func CurrentDay(days []Day, now time.Time) int {
	for i, d := range days {
		if !now.Before(d.Start) && now.Before(d.End) {
			return i
		}
	}
	return -1
}
