package festival

import (
	"time"

	"app-suite/internal/model"
	"app-suite/internal/raffles"
)

// Placement positions an item on a stamp card, in percent of the card's box.
type Placement struct {
	X, Y, Width, Height, Rotation float64
}

// ClampPercent constrains a percentage to 0..100.
func ClampPercent(v float64) float64 { return min(max(v, 0), 100) }

// defaultSize is the width or height of an item saved with none.
const defaultSize = 10

// SanitizePlacement clamps a placement into the card's box; a width or height of
// nothing becomes defaultSize so an item can't collapse. Rotation is kept.
func SanitizePlacement(p Placement) Placement {
	p.X = ClampPercent(p.X)
	p.Y = ClampPercent(p.Y)
	p.Width = ClampPercent(p.Width)
	p.Height = ClampPercent(p.Height)
	if p.Width <= 0 {
		p.Width = defaultSize
	}
	if p.Height <= 0 {
		p.Height = defaultSize
	}
	return p
}

// Rally is what a stamp rally's rules read of it.
type Rally struct {
	// Status is "open" or "closed".
	Status string
	// AvailableFrom and AvailableTo bound when it runs, RFC 3339; empty is unbounded.
	AvailableFrom, AvailableTo string
	// CompletionMode is model.RallyCompletionAll or model.RallyCompletionCounts.
	CompletionMode string
	// RequiredFood and RequiredGame are the stamps of each type a "counts" card needs.
	RequiredFood, RequiredGame int
}

// Stamp is what the rules read of a collectable stamp.
type Stamp struct {
	ID int64
	// Type is model.StampTypeFood or model.StampTypeGame.
	Type   string
	Paused bool
	// ActiveFrom and ActiveTo bound when it can be collected, RFC 3339; empty is the
	// whole event.
	ActiveFrom, ActiveTo string
}

// WithinWindow reports whether now falls inside from..to; an empty or unreadable
// bound is no bound.
func WithinWindow(from, to string, now time.Time) bool {
	if f, ok := raffles.ParseTime(from); ok && now.Before(f) {
		return false
	}
	if t, ok := raffles.ParseTime(to); ok && now.After(t) {
		return false
	}
	return true
}

// RallyOpen reports whether the rally accepts stamping now: it is not closed and now
// is inside its window.
func RallyOpen(r Rally, now time.Time) bool {
	if r.Status == "closed" {
		return false
	}
	return WithinWindow(r.AvailableFrom, r.AvailableTo, now)
}

// StampAvailable reports whether a stamp can be collected now: the rally is open,
// the stamp is inside its own window and it isn't paused.
func StampAvailable(r Rally, s Stamp, now time.Time) bool {
	if s.Paused || !RallyOpen(r, now) {
		return false
	}
	return WithinWindow(s.ActiveFrom, s.ActiveTo, now)
}

// StampExpired reports whether a stamp can never be collected again: its own window
// ended, or the whole rally did. Such a stamp no longer blocks completion.
func StampExpired(r Rally, s Stamp, now time.Time) bool {
	if to, ok := raffles.ParseTime(s.ActiveTo); ok && now.After(to) {
		return true
	}
	if to, ok := raffles.ParseTime(r.AvailableTo); ok && now.After(to) {
		return true
	}
	return false
}

// requiresCounts reports whether completion is measured per type: the counts mode
// with at least one type required.
func requiresCounts(r Rally) bool {
	return model.NormalizeRallyCompletion(r.CompletionMode) == model.RallyCompletionCounts &&
		(r.RequiredFood > 0 || r.RequiredGame > 0)
}

// collectedByType tallies a card's collected stamps by type. Ids are resolved against
// the rally's current stamps, so a deleted stamp counts for nothing.
func collectedByType(stamps []Stamp, collected map[int64]string) (food, game int) {
	for _, s := range stamps {
		if _, ok := collected[s.ID]; !ok {
			continue
		}
		if model.NormalizeStampType(s.Type) == model.StampTypeGame {
			game++
		} else {
			food++
		}
	}
	return food, game
}

// CardComplete reports whether a card is complete.
//
// In counts mode that is only the per-type tallies: the required food and game
// stamps, the rest optional. Expiry does not finish such a card: running out of time
// short of the requirement is a miss.
//
// In all mode every stamp must be collected or expired for good, and at least one
// collected. A paused stamp still inside its window keeps the card incomplete.
//
// A counts rally requiring nothing of either type follows the all rule: taken
// literally it would finish every card at the first stamp and reveal its prizes.
func CardComplete(r Rally, stamps []Stamp, collected map[int64]string, now time.Time) bool {
	if len(collected) == 0 {
		return false
	}
	if requiresCounts(r) {
		food, game := collectedByType(stamps, collected)
		return food >= r.RequiredFood && game >= r.RequiredGame
	}
	for _, s := range stamps {
		if _, ok := collected[s.ID]; ok {
			continue
		}
		if !StampExpired(r, s, now) {
			return false
		}
	}
	return true
}

// CompletionRuleError is the message refusing a counts rally that requires nothing
// of either type, or "" when the rule is usable. Zero of each is the state a form
// starts in when the mode is first picked, so it is refused rather than stored.
func CompletionRuleError(mode string, food, game int) string {
	if mode == model.RallyCompletionCounts && food == 0 && game == 0 {
		return "Set how many food or game stamps a card needs, or switch completion back to every stamp."
	}
	return ""
}

// ClampRequired constrains a per-type requirement to the stamps of that type the
// rally carries: never negative, never more than exist.
func ClampRequired(want, available int) int { return min(max(want, 0), available) }
