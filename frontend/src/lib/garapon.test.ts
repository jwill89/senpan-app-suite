import { describe, it, expect } from 'vitest'
import { rateTotal, ratePct } from './garapon'

describe('rateTotal', () => {
  it('sums positive weights', () => {
    expect(rateTotal([{ rate: 1 }, { rate: 9 }, { rate: 40 }])).toBe(50)
  })

  it('ignores non-positive weights (floored at 0)', () => {
    expect(rateTotal([{ rate: 5 }, { rate: 0 }, { rate: -3 }])).toBe(5)
  })

  it('is zero for an empty list', () => {
    expect(rateTotal([])).toBe(0)
  })
})

describe('ratePct', () => {
  it("formats a weight's share to one decimal place", () => {
    expect(ratePct(1, 3)).toBe('33.3%')
    expect(ratePct(50, 100)).toBe('50.0%')
  })

  it('returns an em dash when the total is non-positive', () => {
    expect(ratePct(5, 0)).toBe('—')
    expect(ratePct(5, -2)).toBe('—')
  })

  it('floors a negative weight to 0% rather than going negative', () => {
    expect(ratePct(-5, 10)).toBe('0.0%')
  })
})
