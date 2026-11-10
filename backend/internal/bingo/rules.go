package bingo

import "math"

// A pattern grid is 5x5, row by row from the top: true marks a cell the win needs.
// The centre [2][2] is the free space, which never needs a number called.

// MatchesPattern reports whether board wins pattern with the numbers in called: every
// cell the pattern marks holds a called number, or is the free space.
func MatchesPattern(board [][]int, pattern [][]bool, called map[int]bool) bool {
	for r := range 5 {
		if r >= len(board) || r >= len(pattern) {
			return false
		}
		for c := range 5 {
			if c >= len(board[r]) || c >= len(pattern[r]) { //nolint:gosec // r is checked against both slices just above
				return false
			}
			if !pattern[r][c] {
				continue
			}
			// The free space always counts.
			if r == 2 && c == 2 {
				continue
			}
			if !called[board[r][c]] { //nolint:gosec // r and c are checked against both slices above
				return false
			}
		}
	}
	return true
}

// PatternColumns reports which of the five columns (B, I, N, G, O as 0-4) hold a cell
// one of the patterns needs, leaving out the free centre. A game draws only from
// these: a column no pattern uses (N, for postage stamps) can't help anyone win. A
// column any pattern uses is in, so every pattern stays winnable. When no pattern
// marks a real cell (or there are none), every column is in, so the game can still
// draw.
func PatternColumns(patterns [][][]bool) [5]bool {
	var cols [5]bool
	for _, p := range patterns {
		for r := 0; r < 5 && r < len(p); r++ {
			for c := 0; c < 5 && c < len(p[r]); c++ {
				centre := r == 2 && c == 2
				if p[r][c] && !centre {
					cols[c] = true
				}
			}
		}
	}
	if cols == ([5]bool{}) {
		return [5]bool{true, true, true, true, true}
	}
	return cols
}

// HalftimeThreshold is the call at which the half-time mini-game prompt fires: the
// classic 35 of 75 scaled to the game's callable numbers (15 for each column its
// patterns use), rounded, and at least 1. The SPA (lib/halftime.ts) and the plugin
// work it out the same way.
func HalftimeThreshold(patterns [][][]bool) int {
	active := 0
	for _, on := range PatternColumns(patterns) {
		if on {
			active++
		}
	}
	threshold := int(math.Round(35.0 / 75.0 * float64(active*15)))
	return max(threshold, 1)
}

// The seconds between automatic draws: DefaultAutoInterval when none is chosen, and
// always within [MinAutoInterval, MaxAutoInterval], so the loop can neither spin nor
// stall.
const (
	DefaultAutoInterval = 30
	MinAutoInterval     = 5
	MaxAutoInterval     = 600
)

// ClampAutoInterval holds an interval (seconds) to the supported range, taking the
// default for none (0 or less).
func ClampAutoInterval(sec int) int {
	if sec <= 0 {
		return DefaultAutoInterval
	}
	return min(max(sec, MinAutoInterval), MaxAutoInterval)
}

// CalledSet is the numbers called, for MatchesPattern.
func CalledSet(called []int) map[int]bool {
	s := make(map[int]bool, len(called))
	for _, n := range called {
		s[n] = true
	}
	return s
}
