package model

// Stamp types - what kind of stall a stamp belongs to, so a rally can require a
// number of each. Stored in stamp_rally_stamps.stamp_type and snapshotted onto
// each collected-stamp log row.
const (
	// StampTypeFood - a food stall's stamp. The original (and only) kind of stamp,
	// so every stamp that predates types is one.
	StampTypeFood = "food"
	// StampTypeGame - a game stall's stamp.
	StampTypeGame = "game"
)

// NormalizeStampType maps a stored/received stamp type onto a known one, falling
// back to StampTypeFood. Rows written before stamp types existed hold "", and API
// clients that predate the field omit it - both mean a food stamp, which is what
// every stamp on every past rally was.
func NormalizeStampType(t string) string {
	switch t {
	case StampTypeGame:
		return StampTypeGame
	default:
		return StampTypeFood
	}
}

// Rally completion modes - what it takes to finish a card. Stored in
// stamp_rallies.completion_mode.
const (
	// RallyCompletionAll - collect every stamp on the card (any that can never be
	// collected again no longer blocks it). The original behavior and the default
	// for any rally created before completion modes existed.
	RallyCompletionAll = "all"
	// RallyCompletionCounts - collect RequiredFood food stamps and RequiredGame
	// game stamps; the rest are optional. A rally with 5 food stalls and 5 game
	// stalls can ask for 3 of each.
	RallyCompletionCounts = "counts"
)

// NormalizeRallyCompletion maps a stored/received completion mode onto a known
// one, falling back to RallyCompletionAll - so a rally written before the field
// existed (or by an older client) keeps requiring the whole card.
func NormalizeRallyCompletion(mode string) string {
	switch mode {
	case RallyCompletionCounts:
		return RallyCompletionCounts
	default:
		return RallyCompletionAll
	}
}

// DefaultStallName is the stall shown for a stamp with neither a map stall nor an
// affiliate: the festival's own venue.
const DefaultStallName = "Senpan Tea House"
