import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Raffle } from '@/types/api'

// Mock the typed endpoint layer so the admin add-entry flow can be exercised
// without touching the network. Only the endpoints the tested paths call are
// stubbed; `detail` backs the loadRaffleDetail() refresh that runs after a
// successful add. `vi.hoisted` lets the spies be referenced in the mock factory.
const { addEntry, detail } = vi.hoisted(() => ({
  addEntry: vi.fn(async () => ({ entry: {} })),
  detail: vi.fn(async () => ({
    raffle: { id: 1, status: 'open', max_entries: 5, cost_per_entry: 0 },
    entries: [],
  })),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { raffles: { addEntry, detail } } }))

import { useRafflesStore } from './raffles'

/** Minimal open raffle with the fields the sign-up math reads. */
function raffle(maxEntries: number, costPerEntry: number): Raffle {
  return { id: 1, status: 'open', max_entries: maxEntries, cost_per_entry: costPerEntry } as Raffle
}

beforeEach(() => {
  setActivePinia(createPinia())
})

describe('raffle entry clamping', () => {
  it('clamps the submitted entry count up to 1 and down to max_entries', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)

    raffles.raffleSignup.numEntries = 99
    raffles.clampSignupEntries()
    expect(raffles.raffleSignup.numEntries).toBe(5)

    raffles.raffleSignup.numEntries = 0
    raffles.clampSignupEntries()
    expect(raffles.raffleSignup.numEntries).toBe(1)
  })

  it('floors fractional entry counts', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(10, 50)
    raffles.raffleSignup.numEntries = 3.9
    raffles.clampSignupEntries()
    expect(raffles.raffleSignup.numEntries).toBe(3)
  })

  it('treats an empty/NaN field as a single entry', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    // Vue's .number modifier can leave a cleared field as '' / NaN.
    raffles.raffleSignup.numEntries = NaN
    raffles.clampSignupEntries()
    expect(raffles.raffleSignup.numEntries).toBe(1)
  })
})

describe('raffleTotalCost', () => {
  it('uses the clamped entry count so the preview can never exceed max', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 250)
    raffles.raffleSignup.numEntries = 50 // over max
    // Clamped to 3 → 3 × 250, not 50 × 250.
    expect(raffles.raffleTotalCost()).toBe(750)
  })

  it('is zero with no selected raffle', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = null
    expect(raffles.raffleTotalCost()).toBe(0)
  })
})

describe('addRaffleEntry (admin)', () => {
  beforeEach(() => {
    addEntry.mockClear()
    detail.mockClear()
  })

  it('trims names, clamps the count to max, forwards paid, then resets the form', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.entryAdd = { characterName: '  Cloud  ', world: ' Gaia ', numEntries: 99, paid: true }

    await raffles.addRaffleEntry()

    expect(addEntry).toHaveBeenCalledWith(1, {
      character_name: 'Cloud',
      world: 'Gaia',
      num_entries: 5, // clamped down to max_entries
      paid: true,
    })
    // Form is cleared on success.
    expect(raffles.entryAdd).toEqual({ characterName: '', world: '', numEntries: 1, paid: false })
  })

  it('floors fractional / sub-1 counts to at least one entry', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(10, 0)
    raffles.entryAdd = { characterName: 'A', world: 'B', numEntries: 0, paid: false }

    await raffles.addRaffleEntry()

    expect(addEntry).toHaveBeenCalledWith(1, expect.objectContaining({ num_entries: 1 }))
  })

  it('does not submit when character or world is blank', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.entryAdd = { characterName: '   ', world: 'Gaia', numEntries: 1, paid: false }

    await raffles.addRaffleEntry()

    expect(addEntry).not.toHaveBeenCalled()
  })

  it('does nothing when no raffle is selected', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = null
    raffles.entryAdd = { characterName: 'Cloud', world: 'Gaia', numEntries: 1, paid: false }

    await raffles.addRaffleEntry()

    expect(addEntry).not.toHaveBeenCalled()
  })
})
