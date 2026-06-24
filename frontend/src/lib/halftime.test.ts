import { describe, it, expect } from 'vitest'
import { activeColumnCount, maxCallableNumbers, halftimeCallThreshold } from './halftime'
import type { BingoGamePattern } from '@/types/api'

/** Builds a pattern whose required cells are in the given [row,col] positions. */
function pattern(cells: [number, number][]): BingoGamePattern {
  const grid = Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => false))
  for (const [r, c] of cells) grid[r][c] = true
  return { id: 1, name: 'p', pattern_data: grid }
}

describe('activeColumnCount', () => {
  it('treats no patterns as all five columns active', () => {
    expect(activeColumnCount([])).toBe(5)
    expect(activeColumnCount(undefined)).toBe(5)
  })

  it('counts distinct columns with a required cell', () => {
    // Columns B, I, N (0,1,2) marked → 3 active.
    expect(activeColumnCount([pattern([[0, 0], [1, 1], [4, 2]])])).toBe(3)
  })

  it('ignores the FREE centre cell', () => {
    // Only the FREE centre [2][2] is marked → no real column → all five active.
    expect(activeColumnCount([pattern([[2, 2]])])).toBe(5)
  })

  it('unions columns across multiple patterns', () => {
    const a = pattern([[0, 0]]) // B
    const b = pattern([[0, 4]]) // O
    expect(activeColumnCount([a, b])).toBe(2)
  })
})

describe('maxCallableNumbers', () => {
  it('is activeColumns × 15', () => {
    expect(maxCallableNumbers([])).toBe(75)
    expect(maxCallableNumbers([pattern([[0, 0], [0, 1], [0, 2]])])).toBe(45)
  })
})

describe('halftimeCallThreshold', () => {
  it('is the classic 35 for a full five-column game', () => {
    expect(halftimeCallThreshold([])).toBe(35)
  })

  it('scales the 35/75 ratio to the callable pool', () => {
    // 3 columns → 45 callable → round(35/75 * 45) = 21.
    expect(halftimeCallThreshold([pattern([[0, 0], [0, 1], [0, 2]])])).toBe(21)
    // 4 columns → 60 callable → round(35/75 * 60) = 28.
    expect(halftimeCallThreshold([pattern([[0, 0], [0, 1], [0, 2], [0, 3]])])).toBe(28)
    // 1 column → 15 callable → round(35/75 * 15) = 7.
    expect(halftimeCallThreshold([pattern([[0, 0]])])).toBe(7)
  })

  it('never returns less than 1', () => {
    expect(halftimeCallThreshold([pattern([[0, 0]])])).toBeGreaterThanOrEqual(1)
  })
})
