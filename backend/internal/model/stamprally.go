package model

// Stamp types - what kind of stall a stamp belongs to, so a rally can require a
// number of each. Stored in stamp_rally_stamps.stamp_type and snapshotted onto
// each collected-stamp log row.
const (
	// StampTypeFood - a food stall's stamp, and what a blank or unknown type means.
	StampTypeFood = "food"
	// StampTypeGame - a game stall's stamp.
	StampTypeGame = "game"
)

// NormalizeStampType maps a stored/received stamp type onto a known one, falling
// back to StampTypeFood, so a row that holds "" or an API client that omits the
// field gets a food stamp.
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
	// RallyCompletionAll - collect every stamp on the card (one that can never be
	// collected again doesn't block it). The default, and what a blank mode means.
	RallyCompletionAll = "all"
	// RallyCompletionCounts - collect RequiredFood food stamps and RequiredGame
	// game stamps; the rest are optional. A rally with 5 food stalls and 5 game
	// stalls can ask for 3 of each.
	RallyCompletionCounts = "counts"
)

// NormalizeRallyCompletion maps a stored/received completion mode onto a known
// one, falling back to RallyCompletionAll - so a row that holds "" or a client that
// omits the field requires the whole card.
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
