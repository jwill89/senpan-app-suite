package festival

import (
	"regexp"
	"strings"
	"time"
)

// Map statuses.
const (
	MapInProgress = "in_progress"
	MapPublished  = "published"
	MapClosed     = "closed"
)

// NormalizeMapStatus reads a stored status, treating anything unknown as in progress.
func NormalizeMapStatus(status string) string {
	switch status {
	case MapPublished:
		return MapPublished
	case MapClosed:
		return MapClosed
	default:
		return MapInProgress
	}
}

// Stall shapes.
const (
	ShapeCircle = "circle"
	ShapeRect   = "rect"
)

// NormalizeShape reads a stall's shape, treating anything unknown as a rectangle.
func NormalizeShape(shape string) string {
	if shape == ShapeCircle {
		return ShapeCircle
	}
	return ShapeRect
}

// What a stall offers.
const (
	StallGame  = "game"
	StallFood  = "food"
	StallBoth  = "both"
	StallOther = "other"
)

// NormalizeStallType reads what a stall offers, treating anything unknown as other.
func NormalizeStallType(t string) string {
	switch t {
	case StallGame, StallFood, StallBoth:
		return t
	default:
		return StallOther
	}
}

// EventTime is one range a festival or one of its occupants runs.
type EventTime struct {
	// Label is an optional descriptor such as "Day 1".
	Label string
	// Start is the range's first instant, RFC 3339.
	Start string
	// End is its last, RFC 3339; empty for no stated end.
	End string
}

var (
	hexColor       = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	mapSlug        = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	allDigits      = regexp.MustCompile(`^[0-9]+$`)
	slugSeparators = regexp.MustCompile(`[\s_-]+`)
)

// SanitizeColor keeps a "#rrggbb" color, lowercased, and discards anything else, so
// a stall's color can only be a literal the stylesheet can use (or empty).
func SanitizeColor(color string) string {
	color = strings.TrimSpace(color)
	if hexColor.MatchString(color) {
		return strings.ToLower(color)
	}
	return ""
}

// NormalizeSlug settles whatever was typed into a storable shortcode: trimmed,
// lowercased, with runs of spaces, underscores and dashes folded into the single
// dash the pattern allows.
func NormalizeSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = slugSeparators.ReplaceAllString(slug, "-")
	return strings.Trim(slug, "-")
}

// SlugError is the message refusing a normalized shortcode, or "" when it is usable.
// An empty one is always fine: the map is then reachable by id only. All digits is
// refused because the shortcode shares a path segment with the id.
func SlugError(slug string) string {
	if slug == "" {
		return ""
	}
	if allDigits.MatchString(slug) {
		return "A shortcode can't be only numbers - it would be indistinguishable from a map id."
	}
	if len(slug) < 2 || len(slug) > 64 || !mapSlug.MatchString(slug) {
		return "A shortcode may use letters, numbers and dashes only (2-64 characters), e.g. \"obon-2026\"."
	}
	return ""
}

// SanitizeEventTimes trims the ranges and drops the ones with no start, which say
// nothing. The result is never nil.
func SanitizeEventTimes(times []EventTime) []EventTime {
	out := make([]EventTime, 0, len(times))
	for _, t := range times {
		t.Label = strings.TrimSpace(t.Label)
		t.Start = strings.TrimSpace(t.Start)
		t.End = strings.TrimSpace(t.End)
		if t.Start == "" {
			continue
		}
		out = append(out, t)
	}
	return out
}

// WithinAnyEventTime reports whether now falls inside one of the ranges. An empty
// list is unbounded: nothing was stated, so nothing is excluded.
func WithinAnyEventTime(times []EventTime, now time.Time) bool {
	if len(times) == 0 {
		return true
	}
	for _, t := range times {
		if WithinWindow(t.Start, t.End, now) {
			return true
		}
	}
	return false
}

// KeepOccupant reports whether a pitch occupant saved with this identity is kept. A
// new one (id 0) naming neither a title nor an affiliate is the blank row a form
// leaves behind and is dropped; a stored one is kept even if blank, since an
// affiliate it named may have been deleted underneath it, and dropping it would take
// the whole pitch with it on an unrelated save.
func KeepOccupant(id int64, title string, hasAffiliate bool) bool {
	return id != 0 || strings.TrimSpace(title) != "" || hasAffiliate
}
