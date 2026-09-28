import { describe, it, expect } from 'vitest'
import { stallName } from './stampcard'

describe('stallName', () => {
  /**
   * Mirrors Go's StampRallyStamp.DisplayStall: first non-blank of the map
   * occupant's stall name, then the affiliate, then the venue's own default.
   *
   * The order is the whole point. A rally linked to a festival map takes its
   * stamps from pitch OCCUPANTS and the server joins that occupant's name, so
   * preferring the affiliate labelled a map-linked stamp by the partner behind the
   * booth rather than the booth - and fell all the way back to "Senpan Tea House"
   * for an occupant with no affiliate at all, which is precisely the case a
   * festival map exists to describe.
   */
  it('prefers the occupant stall name over the affiliate', () => {
    expect(stallName('Flora Teahouse', 'Lunaria')).toBe('Flora Teahouse')
  })

  it('falls back to the affiliate when there is no stall name', () => {
    expect(stallName('', 'Lunaria')).toBe('Lunaria')
    expect(stallName('   ', 'Lunaria')).toBe('Lunaria')
  })

  it('falls back to the venue when neither is set', () => {
    expect(stallName('', '')).toBe('Senpan Tea House')
    expect(stallName('')).toBe('Senpan Tea House')
  })

  it('trims what it returns', () => {
    expect(stallName('  Flora Teahouse  ')).toBe('Flora Teahouse')
  })

  /**
   * The public card view passes a single already-resolved name (the server sends
   * DisplayStall in PublicStamp.affiliate_name), so the one-argument form has to
   * keep behaving as a plain "this name, or the default".
   */
  it('works with a single already-resolved name', () => {
    expect(stallName('Whoever Is Here Today')).toBe('Whoever Is Here Today')
  })
})
