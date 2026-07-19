import { describe, it, expect } from 'vitest'
import {
  STAMP_SHAPES,
  STAMP_COLORS,
  BINGO_LETTERS,
  DRAW_DELAY_OPTIONS,
  columnNumbers,
  columnRange,
  emptyGrid,
  emptyNumberBoard,
  randomBoard,
  validateBoard,
  patternColumns,
  DEFAULT_APP_SETTINGS,
} from './constants'

// Build a 5×5 pattern grid marking the given columns (a true cell in row 0 of each).
function markCols(...columns: number[]): boolean[][] {
  const g = Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => false))
  for (const c of columns) g[0][c] = true
  return g
}

describe('columnNumbers', () => {
  it('returns the B column 1–15', () => {
    expect(columnNumbers(0)).toEqual([1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15])
  })

  it('returns the O column 61–75', () => {
    const o = columnNumbers(4)
    expect(o[0]).toBe(61)
    expect(o.at(-1)).toBe(75)
    expect(o).toHaveLength(15)
  })

  it('produces 15 contiguous numbers for every column with no overlap', () => {
    const all = [0, 1, 2, 3, 4].flatMap(columnNumbers)
    expect(all).toHaveLength(75)
    expect(new Set(all).size).toBe(75)
    expect(all[0]).toBe(1)
    expect(all.at(-1)).toBe(75)
  })
})

describe('emptyGrid', () => {
  it('is a 5×5 grid of false', () => {
    const g = emptyGrid()
    expect(g).toHaveLength(5)
    expect(g.every((row) => row.length === 5)).toBe(true)
    expect(g.flat().every((v) => !v)).toBe(true)
  })

  it('returns independent rows (no shared reference)', () => {
    const g = emptyGrid()
    g[0][0] = true
    expect(g[1][0]).toBe(false)
  })
})

describe('stamp metadata', () => {
  it('exposes unique shape ids starting with blank', () => {
    expect(STAMP_SHAPES[0].id).toBe('blank')
    const ids = STAMP_SHAPES.map((s) => s.id)
    expect(new Set(ids).size).toBe(ids.length)
  })

  it('exposes unique color ids with rgba values', () => {
    const ids = STAMP_COLORS.map((c) => c.id)
    expect(new Set(ids).size).toBe(ids.length)
    expect(STAMP_COLORS.every((c) => c.value.startsWith('rgba('))).toBe(true)
  })
})

describe('board constants', () => {
  it('has the five BINGO letters in order', () => {
    expect([...BINGO_LETTERS]).toEqual(['B', 'I', 'N', 'G', 'O'])
  })

  it('offers an instant (0s) draw delay', () => {
    expect(DRAW_DELAY_OPTIONS).toContain(0)
  })

  it('ships a default app title', () => {
    expect(DEFAULT_APP_SETTINGS.app_title).toBeTruthy()
  })
})

describe('patternColumns', () => {
  it('marks only the columns a pattern uses (B + O)', () => {
    expect(patternColumns([{ pattern_data: markCols(0, 4) }])).toEqual([
      true,
      false,
      false,
      false,
      true,
    ])
  })

  it('unions the columns across multiple patterns', () => {
    expect(patternColumns([{ pattern_data: markCols(0) }, { pattern_data: markCols(2) }])).toEqual([
      true,
      false,
      true,
      false,
      false,
    ])
  })

  it('ignores the FREE centre — a free-only pattern is treated as all-active', () => {
    const freeOnly = Array.from({ length: 5 }, () => Array.from({ length: 5 }, () => false))
    freeOnly[2][2] = true
    expect(patternColumns([{ pattern_data: freeOnly }])).toEqual([true, true, true, true, true])
  })

  it('reports every column active when there are no patterns', () => {
    expect(patternColumns([])).toEqual([true, true, true, true, true])
  })
})

describe('bingo card helpers', () => {
  const validBoard = () => [
    [1, 16, 31, 46, 61],
    [2, 17, 32, 47, 62],
    [3, 18, 0, 48, 63],
    [4, 19, 34, 49, 64],
    [5, 20, 35, 50, 65],
  ]

  it('columnRange returns the inclusive band per column', () => {
    expect(columnRange(0)).toEqual([1, 15])
    expect(columnRange(2)).toEqual([31, 45])
    expect(columnRange(4)).toEqual([61, 75])
  })

  it('emptyNumberBoard is a 5×5 all-zero grid', () => {
    const b = emptyNumberBoard()
    expect(b).toHaveLength(5)
    expect(b.every((r) => r.length === 5 && r.every((n) => n === 0))).toBe(true)
  })

  it('randomBoard produces a valid card with a FREE centre and in-range distinct columns', () => {
    for (let i = 0; i < 20; i++) {
      const b = randomBoard()
      expect(b[2][2]).toBe(0)
      expect(validateBoard(b)).toBe('')
    }
  })

  it('validateBoard accepts a valid board', () => {
    expect(validateBoard(validBoard())).toBe('')
  })

  it('validateBoard rejects a non-5×5 grid', () => {
    expect(validateBoard([[1, 2, 3]])).toMatch(/5×5/)
  })

  it('validateBoard requires the FREE centre', () => {
    const b = validBoard()
    b[2][2] = 33
    expect(validateBoard(b)).toMatch(/FREE/)
  })

  it('validateBoard rejects an out-of-range number', () => {
    const b = validBoard()
    b[0][0] = 99 // B column only allows 1–15
    expect(validateBoard(b)).toMatch(/Column B/)
  })

  it('validateBoard rejects a duplicate within a column', () => {
    const b = validBoard()
    b[1][0] = b[0][0] // duplicate in B
    expect(validateBoard(b)).toMatch(/more than once/)
  })
})
