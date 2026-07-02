/**
 * Half-time minigame timing.
 *
 * The game only draws from the bingo columns (B-I-N-G-O) whose numbers can
 * complete one of the active win patterns, so the number of balls a game can
 * actually call is `activeColumns × 15` rather than always 75 (see the backend
 * `bingo.PatternColumns`). The half-time prompt used to fire at a hard-coded 35
 * of 75 calls; this scales that same ratio to the game's real callable pool, so a
 * three-column game (45 callable) prompts at ~21 instead of never reaching 35.
 */
import type { BingoGamePattern } from '@/types/api'

/** Numbers per bingo column (B 1–15, I 16–30, …). */
export const NUMBERS_PER_COLUMN = 15

/** The classic half-way mark: 35 of a full 75-ball game. */
export const HALFTIME_RATIO = 35 / 75

/**
 * Counts the active bingo columns for a set of patterns — a column is active if
 * any pattern requires a non-FREE cell in it (the FREE centre never needs a
 * draw). With no marked cells (or no patterns) all five columns are active, so
 * the game can still draw. Mirrors the backend `bingo.PatternColumns`.
 */
export function activeColumnCount(patterns: BingoGamePattern[] | undefined | null): number {
  const cols = [false, false, false, false, false]
  for (const p of patterns ?? []) {
    const grid = p.pattern_data
    for (let r = 0; r < 5 && r < grid.length; r++) {
      const row = grid[r] ?? []
      for (let c = 0; c < 5 && c < row.length; c++) {
        if (row[c] && !(r === 2 && c === 2)) cols[c] = true
      }
    }
  }
  const count = cols.filter(Boolean).length
  return count === 0 ? 5 : count
}

/** How many balls this game can actually call (`activeColumns × 15`). */
export function maxCallableNumbers(patterns: BingoGamePattern[] | undefined | null): number {
  return activeColumnCount(patterns) * NUMBERS_PER_COLUMN
}

/**
 * The call number at which to prompt for the half-time minigame: the classic
 * 35-of-75 mark scaled to this game's callable pool, rounded and clamped to at
 * least 1. A full five-column game yields 35 (unchanged); a three-column game
 * (45 callable) yields 21.
 */
export function halftimeCallThreshold(patterns: BingoGamePattern[] | undefined | null): number {
  return Math.max(1, Math.round(HALFTIME_RATIO * maxCallableNumbers(patterns)))
}
