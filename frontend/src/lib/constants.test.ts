import { describe, it, expect } from 'vitest'
import {
  STAMP_SHAPES,
  STAMP_COLORS,
  BINGO_LETTERS,
  DRAW_DELAY_OPTIONS,
  columnNumbers,
  emptyGrid,
  DEFAULT_APP_SETTINGS,
} from './constants'

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
