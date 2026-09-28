import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest'
import {
  savedSignups,
  saveRallySignup,
  saveRaffleSignup,
  savedRallySignup,
  savedRaffleSignup,
  forgetSignup,
} from './signups'

const KEY = 'senpan_signups'

function rally(over: Partial<Parameters<typeof saveRallySignup>[0]> = {}) {
  return saveRallySignup({
    rallyId: 1,
    rallyTitle: 'Festival Rally',
    name: 'Aria Ashwood',
    world: 'Gilgamesh',
    cardToken: 'tok_card',
    garaponToken: 'tok_draw',
    garaponTitle: 'Festival Garapon',
    ...over,
  })
}

beforeEach(() => localStorage.clear())
afterEach(() => vi.useRealTimers())

describe('saved sign-ups', () => {
  it('keeps the drawing token, which nothing else can give back', () => {
    rally()
    // The whole reason this module exists: the server will report how many draws
    // remain but never the token that spends them, so sign-up is the only moment
    // it can be captured.
    expect(savedRallySignup(1)?.garaponToken).toBe('tok_draw')
    expect(savedRallySignup(1)?.cardToken).toBe('tok_card')
  })

  it('keeps the exact name and world a raffle entry was made under', () => {
    // Entries merge on name+world, so a different spelling silently starts a
    // second entry and splits the person's tickets.
    saveRaffleSignup({
      raffleId: 7,
      raffleTitle: 'Prize Draw',
      name: 'Aria Ashwood',
      world: 'Gilgamesh',
    })
    expect(savedRaffleSignup(7)).toMatchObject({ name: 'Aria Ashwood', world: 'Gilgamesh' })
  })

  it('refreshes rather than duplicates when the same thing is signed up for again', () => {
    rally({ cardToken: 'first' })
    rally({ cardToken: 'second' })
    expect(savedSignups().filter((e) => e.kind === 'rally')).toHaveLength(1)
    expect(savedRallySignup(1)?.cardToken).toBe('second')
  })

  it('keeps rallies and raffles with the same id apart', () => {
    rally({ rallyId: 3 })
    saveRaffleSignup({ raffleId: 3, raffleTitle: 'R', name: 'Someone', world: 'Balmung' })
    expect(savedRallySignup(3)?.kind).toBe('rally')
    expect(savedRaffleSignup(3)?.kind).toBe('raffle')

    // ...including when one is forgotten.
    forgetSignup('rally', 3)
    expect(savedRallySignup(3)).toBeUndefined()
    expect(savedRaffleSignup(3)).toBeDefined()
  })

  it('forgets a sign-up, so a shared device can be cleared of a drawing link', () => {
    rally()
    forgetSignup('rally', 1)
    expect(savedRallySignup(1)).toBeUndefined()
  })

  /**
   * The upgrade path. A browser that signed up BEFORE the home world had its own
   * field holds the whole thing in `name` and no `world` key at all. Left alone it
   * would be a type lie - `world` typed string, undefined at runtime - and the name
   * would no longer match what the server now stores, so the person would look like
   * a stranger to their own records.
   */
  it('splits a sign-up saved before worlds had their own field', () => {
    localStorage.setItem(
      KEY,
      JSON.stringify([
        {
          kind: 'rally',
          rallyId: 1,
          rallyTitle: 'Festival Rally',
          name: 'Aria Ashwood @ Gilgamesh',
          cardToken: 'tok_card',
          garaponToken: 'tok_draw',
          savedAt: Date.now(),
        },
        {
          kind: 'raffle',
          raffleId: 2,
          raffleTitle: 'Prize Draw',
          name: 'Borin Stoneheart @ Balmung',
          savedAt: Date.now(),
        },
      ]),
    )

    expect(savedRallySignup(1)).toMatchObject({ name: 'Aria Ashwood', world: 'Gilgamesh' })
    expect(savedRaffleSignup(2)).toMatchObject({ name: 'Borin Stoneheart', world: 'Balmung' })
    // The tokens are what the record exists for - splitting must not disturb them.
    expect(savedRallySignup(1)?.garaponToken).toBe('tok_draw')
  })

  it('leaves a legacy name with no world alone rather than inventing one', () => {
    localStorage.setItem(
      KEY,
      JSON.stringify([
        { kind: 'rally', rallyId: 1, name: 'Solo Name', cardToken: 't', savedAt: Date.now() },
      ]),
    )
    expect(savedRallySignup(1)).toMatchObject({ name: 'Solo Name', world: '' })
  })

  it('drops entries past the retention window', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-01-01T00:00:00Z'))
    rally()
    vi.setSystemTime(new Date('2026-01-20T00:00:00Z'))
    expect(savedRallySignup(1)).toBeDefined() // well inside a festival's life
    vi.setSystemTime(new Date('2026-06-01T00:00:00Z'))
    expect(savedRallySignup(1)).toBeUndefined()
    expect(savedSignups()).toHaveLength(0)
  })

  /**
   * Everything here is user-editable and survives across app versions, so the
   * reader has to treat the stored blob as untrusted input rather than as
   * something it wrote itself.
   */
  it('survives corrupt, foreign or malformed stored data', () => {
    for (const junk of [
      'not json at all',
      '{"not":"an array"}',
      '[null, 3, "text"]',
      '[{"kind":"rally"}]', // no id or token
      '[{"kind":"rally","rallyId":1,"cardToken":"","savedAt":1}]', // blank token
      `[{"kind":"whatever","savedAt":${Date.now()}}]`, // unknown kind
      '[{"kind":"rally","rallyId":1,"cardToken":"t"}]', // no savedAt
    ]) {
      localStorage.setItem(KEY, junk)
      expect(savedSignups()).toEqual([])
      expect(savedRallySignup(1)).toBeUndefined()
    }
  })

  it('degrades to empty when localStorage itself throws', () => {
    const getItem = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new DOMException('denied')
    })
    expect(savedSignups()).toEqual([])
    getItem.mockRestore()

    // A failed WRITE must not break the sign-up that just succeeded, either - the
    // links are already on screen; this copy is only a convenience.
    const setItem = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('quota')
    })
    expect(() => rally()).not.toThrow()
    setItem.mockRestore()
  })
})
