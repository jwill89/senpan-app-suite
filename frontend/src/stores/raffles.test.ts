import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type {
  FestivalMap,
  FestivalStallOccupant,
  Raffle,
  RaffleEntry,
  RaffleLookupEntry,
} from '@/types/api'
import { RAFFLE_MAX_ENTRIES } from '@/lib/constants'

// Mock the typed endpoint layer so the admin add-entry flow can be exercised
// without touching the network. Only the endpoints the tested paths call are
// stubbed; `detail` backs the loadRaffleDetail() refresh that runs after a
// successful add. `vi.hoisted` lets the spies be referenced in the mock factory.
const { addEntry, detail, create, list, markEntryPaid, lookup, setStatus, mapList, mapDetail } =
  vi.hoisted(() => ({
    mapList: vi.fn(async () => ({ maps: [] as FestivalMap[] })),
    mapDetail: vi.fn(async () => ({ map: { stalls: [] } as unknown as FestivalMap })),
    setStatus: vi.fn(async () => ({ ok: true, status: 'closed' })),
    // Typed so a test can resolve it with real hits; a bare [] would infer never[].
    lookup: vi.fn(async () => ({ entries: [] as RaffleLookupEntry[], truncated: false })),
    addEntry: vi.fn(async () => ({ entry: {} })),
    markEntryPaid: vi.fn(async () => ({ entry: {} })),
    detail: vi.fn(async () => ({
      raffle: { id: 1, status: 'open', max_entries: 5, cost_per_entry: 0 },
      entries: [],
    })),
    create: vi.fn(async () => ({ raffle: {} })),
    list: vi.fn(async () => ({ raffles: [] })),
  }))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    raffles: { addEntry, detail, create, list, markEntryPaid, lookup, setStatus },
    festivalMaps: { list: mapList, detail: mapDetail },
  },
}))

import { useUiStore } from './ui'
import {
  useRafflesStore,
  isRaffleEnterable,
  raffleAcceptsSignups,
  raffleCostLabel,
  raffleEntryCost,
  raffleHasCost,
  raffleMode,
  entryAmountCollected,
  entryAmountOutstanding,
  entryPaymentState,
  entryTicketPrice,
} from './raffles'

/** A raffle entry; each test overrides only the settlement fields it cares about. */
function makeEntry(overrides: Partial<RaffleEntry> = {}): RaffleEntry {
  return {
    id: 1,
    raffle_id: 1,
    character_name: 'Aria',
    world: 'Gilgamesh',
    num_entries: 1,
    paid_entries: 0,
    amount_waived: 0,
    paid: false,
    created_at: '',
    ...overrides,
  }
}

/** A complete open raffle; each test overrides only the fields it cares about. */
function makeRaffle(overrides: Partial<Raffle> = {}): Raffle {
  return {
    id: 1,
    title: 'Test Raffle',
    description: '',
    rules: '',
    max_entries: 1,
    signup_instructions: '',
    entry_mode: 'single',
    cost_per_entry: 0,
    tier_costs: [],
    available_from: '',
    available_to: '',
    prize_image: '',
    pay_image: '',
    status: 'open',
    winner_entry_id: undefined,
    created_at: '',
    ...overrides,
  }
}

/** Raffle charging its 1st/2nd/3rd entry a different price. */
function tiered(...costs: number[]): Raffle {
  return makeRaffle({ entry_mode: 'custom', max_entries: costs.length, tier_costs: costs })
}

/** Minimal open raffle with the fields the sign-up math reads. */
function raffle(maxEntries: number, costPerEntry: number): Raffle {
  return makeRaffle({ max_entries: maxEntries, cost_per_entry: costPerEntry })
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

describe('isRaffleEnterable (public visibility)', () => {
  const HOUR = 3600_000
  const iso = (offsetMs: number): string => new Date(Date.now() + offsetMs).toISOString()

  it('is true for an open raffle with no end date', () => {
    expect(isRaffleEnterable(makeRaffle())).toBe(true)
  })

  it('is true for an open raffle whose end is still in the future', () => {
    expect(isRaffleEnterable(makeRaffle({ available_to: iso(HOUR) }))).toBe(true)
  })

  it('is false for an open raffle whose end has passed', () => {
    expect(isRaffleEnterable(makeRaffle({ available_to: iso(-HOUR) }))).toBe(false)
  })

  it('is false for an open raffle that has not started yet', () => {
    expect(isRaffleEnterable(makeRaffle({ available_from: iso(HOUR) }))).toBe(false)
  })

  it('is false for a closed raffle regardless of end date', () => {
    expect(isRaffleEnterable(makeRaffle({ status: 'closed', available_to: iso(HOUR) }))).toBe(false)
  })
})

describe('raffleTotalCost', () => {
  it('uses the clamped entry count so the preview can never exceed max', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 250)
    raffles.raffleSignup.numEntries = 50 // over max
    // Clamped to 3 -> 3 x 250, not 50 x 250.
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
    raffles.entryAdd = {
      characterName: '  Cloud  ',
      world: ' Gaia ',
      numEntries: 99,
      paid: true,
      amountWaived: 0,
    }

    await raffles.addRaffleEntry()

    expect(addEntry).toHaveBeenCalledWith(1, {
      character_name: 'Cloud',
      world: 'Gaia',
      num_entries: 5, // clamped down to max_entries
      paid: true,
      amount_waived: 0,
    })
    // Form is cleared on success.
    expect(raffles.entryAdd).toEqual({
      characterName: '',
      world: '',
      numEntries: 1,
      paid: false,
      amountWaived: 0,
    })
  })

  it('floors fractional / sub-1 counts to at least one entry', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(10, 0)
    raffles.entryAdd = {
      characterName: 'A',
      world: 'B',
      numEntries: 0,
      paid: false,
      amountWaived: 0,
    }

    await raffles.addRaffleEntry()

    expect(addEntry).toHaveBeenCalledWith(1, expect.objectContaining({ num_entries: 1 }))
  })

  it('does not submit when character or world is blank', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.entryAdd = {
      characterName: '   ',
      world: 'Gaia',
      numEntries: 1,
      paid: false,
      amountWaived: 0,
    }

    await raffles.addRaffleEntry()

    expect(addEntry).not.toHaveBeenCalled()
  })

  it('does nothing when no raffle is selected', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = null
    raffles.entryAdd = {
      characterName: 'Cloud',
      world: 'Gaia',
      numEntries: 1,
      paid: false,
      amountWaived: 0,
    }

    await raffles.addRaffleEntry()

    expect(addEntry).not.toHaveBeenCalled()
  })
})

describe('raffle entry modes', () => {
  it('treats an unknown or missing mode as the original flat cost', () => {
    // Raffles saved before entry modes existed carry ''. That has always meant
    // "flat cost_per_entry", so it must never read as free or as no-sign-ups.
    const legacy = makeRaffle({ entry_mode: '', cost_per_entry: 500 })
    expect(raffleMode(legacy)).toBe('single')
    expect(raffleAcceptsSignups(legacy)).toBe(true)
    expect(raffleEntryCost(legacy, 2)).toBe(1000)
  })

  it('climbs the ladder in custom mode instead of multiplying one price', () => {
    const r = tiered(50_000, 100_000, 150_000)
    expect(raffleEntryCost(r, 1)).toBe(50_000)
    expect(raffleEntryCost(r, 2)).toBe(150_000)
    expect(raffleEntryCost(r, 3)).toBe(300_000)
  })

  it('charges nothing for an entry past the end of the ladder', () => {
    expect(raffleEntryCost(tiered(50_000, 100_000), 5)).toBe(150_000)
    expect(raffleEntryCost(tiered(50_000), 0)).toBe(0)
  })

  it('prices a huge ticket count off the ladder, not one pass per ticket', () => {
    // A details raffle prices nothing, and a custom one only has prices for the
    // rungs it declares - neither may walk the ticket count, which an admin sets.
    const details = makeRaffle({ entry_mode: 'details', max_entries: 500_000_000 })
    const started = Date.now()
    expect(raffleEntryCost(details, 500_000_000)).toBe(0)
    expect(raffleEntryCost(tiered(50_000, 100_000), 500_000_000)).toBe(150_000)
    expect(Date.now() - started).toBeLessThan(1_000)
  })

  it('takes no sign-ups and costs nothing in details mode', () => {
    const r = makeRaffle({ entry_mode: 'details', cost_per_entry: 500, tier_costs: [9] })
    expect(raffleAcceptsSignups(r)).toBe(false)
    expect(raffleEntryCost(r, 3)).toBe(0)
    expect(raffleHasCost(r)).toBe(false)
    expect(raffleCostLabel(r)).toBe('')
  })

  it('labels a flat cost per entry and a ladder differently', () => {
    expect(raffleCostLabel(raffle(3, 50_000))).toBe('50,000 gil per entry')
    expect(raffleCostLabel(tiered(50_000, 100_000, 150_000))).toBe('50,000 / 100,000 / 150,000 gil')
    expect(raffleCostLabel(raffle(3, 0))).toBe('')
  })

  it('summarises a long ladder instead of spelling it out', () => {
    // This label lands in a card and an admin badge; a raffle may carry up to 50
    // rungs, which would run off the tile.
    const long = tiered(...Array.from({ length: 12 }, (_, i) => (i + 1) * 1_000))
    expect(raffleCostLabel(long)).toBe('12 entries, 78,000 gil for all')
    // Four rungs still fit, so they stay spelled out.
    expect(raffleCostLabel(tiered(1, 2, 3, 4))).toBe('1 / 2 / 3 / 4 gil')
  })
})

describe('raffleTotalCost with a custom ladder', () => {
  it('prices the clamped count through the ladder', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = tiered(50_000, 100_000, 150_000)
    raffles.raffleSignup.numEntries = 3
    expect(raffles.raffleTotalCost()).toBe(300_000)

    // Over the cap clamps to the ladder length, never past its last rung.
    raffles.raffleSignup.numEntries = 99
    expect(raffles.raffleTotalCost()).toBe(300_000)
  })
})

describe('selectedRaffleEnterable', () => {
  it('is false for a live details-only raffle (it has no form here)', () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = makeRaffle({ entry_mode: 'details' })
    expect(isRaffleEnterable(raffles.selectedRaffle)).toBe(true) // still browsable
    expect(raffles.selectedRaffleEnterable).toBe(false)
  })
})

describe('saveRaffle payload', () => {
  beforeEach(() => {
    create.mockClear()
  })

  it('sends only the costs the chosen mode uses', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Tiered'
    raffles.raffleForm!.entry_mode = 'custom'
    raffles.raffleForm!.cost_per_entry = 777
    raffles.raffleForm!.tier_costs = [50_000, 100_000]

    await raffles.saveRaffle()

    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({
        entry_mode: 'custom',
        cost_per_entry: 0,
        tier_costs: [50_000, 100_000],
      }),
    )
  })

  it('clears a stale ladder when saving in single mode', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Flat'
    raffles.raffleForm!.entry_mode = 'single'
    raffles.raffleForm!.cost_per_entry = 250
    raffles.raffleForm!.tier_costs = [50_000, 100_000] // left over from a mode switch

    await raffles.saveRaffle()

    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ entry_mode: 'single', cost_per_entry: 250, tier_costs: [] }),
    )
  })

  it('rejects a negative price rather than sending it', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Bad'
    raffles.raffleForm!.entry_mode = 'custom'
    raffles.raffleForm!.tier_costs = [50_000, -1]

    expect(await raffles.saveRaffle()).toBe(false)
    expect(create).not.toHaveBeenCalled()
  })
})

describe('entry settlement', () => {
  it('reports unpaid, partial and paid', () => {
    expect(entryPaymentState(makeEntry({ num_entries: 3, paid_entries: 0 }))).toBe('unpaid')
    expect(entryPaymentState(makeEntry({ num_entries: 3, paid_entries: 1 }))).toBe('partial')
    expect(entryPaymentState(makeEntry({ num_entries: 3, paid_entries: 3 }))).toBe('paid')
  })

  it('nets waivers off what was collected, priced through the ladder', () => {
    const r = tiered(50_000, 100_000, 150_000)
    // All three settled (300,000), 100,000 forgiven.
    const e = makeEntry({ num_entries: 3, paid_entries: 3, amount_waived: 100_000, paid: true })
    expect(entryTicketPrice(r, e)).toBe(300_000)
    expect(entryAmountCollected(r, e)).toBe(200_000)
    expect(entryAmountOutstanding(r, e)).toBe(0)
  })

  it('quotes only the unsettled tickets as outstanding', () => {
    const r = tiered(50_000, 100_000, 150_000)
    // First ticket settled; two bought since.
    const e = makeEntry({ num_entries: 3, paid_entries: 1 })
    expect(entryAmountCollected(r, e)).toBe(50_000)
    expect(entryAmountOutstanding(r, e)).toBe(250_000)
  })

  it('never reports a negative collection when more was waived than owed', () => {
    const r = raffle(1, 1_000)
    const e = makeEntry({ num_entries: 1, paid_entries: 1, amount_waived: 9_999, paid: true })
    expect(entryAmountCollected(r, e)).toBe(0)
  })
})

describe('setEntryPaid', () => {
  beforeEach(() => {
    markEntryPaid.mockClear()
  })

  it('sends only what is being waived now, never a running total', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    // The entry already had 10,000 forgiven; this settlement forgives 5,000 more.
    const entry = makeEntry({ num_entries: 3, paid_entries: 1, amount_waived: 10_000 })

    await raffles.setEntryPaid(entry, true, 5_000)

    expect(markEntryPaid).toHaveBeenCalledWith(1, 1, true, 5_000, 0)
  })

  it('passes the ticket count for a part payment', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    const entry = makeEntry({ num_entries: 3 })

    // Gil for two of the three entries now, the third later.
    await raffles.setEntryPaid(entry, true, 0, 2)

    expect(markEntryPaid).toHaveBeenCalledWith(1, 1, true, 0, 2)
  })

  it('sends no waiver when clearing a payment', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)

    await raffles.setEntryPaid(makeEntry({ paid: true, paid_entries: 3 }), false, 5_000, 2)

    // Clearing resets the row outright, so neither figure is forwarded.
    expect(markEntryPaid).toHaveBeenCalledWith(1, 1, false, 0, 0)
  })

  it('adopts the row the server returns rather than guessing the new state', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    const entry = makeEntry({ num_entries: 3 })
    markEntryPaid.mockResolvedValueOnce({
      entry: makeEntry({ num_entries: 3, paid_entries: 3, amount_waived: 15_000, paid: true }),
    })

    await raffles.setEntryPaid(entry, true, 5_000)

    // The accumulated waiver is the server's to compute - the local row takes it.
    expect(entry.amount_waived).toBe(15_000)
    expect(entry.paid_entries).toBe(3)
    expect(entry.paid).toBe(true)
  })
})

describe('addRaffleTier', () => {
  it('stops the ladder at the per-player entry cap', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.entry_mode = 'custom'
    // The ladder length IS the allowance, so it can't grow past what the server
    // will accept.
    raffles.raffleForm!.tier_costs = Array.from({ length: RAFFLE_MAX_ENTRIES }, () => 100)

    raffles.addRaffleTier()

    expect(raffles.raffleForm!.tier_costs).toHaveLength(RAFFLE_MAX_ENTRIES)
  })

  it('seeds a new rung from the last one', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.tier_costs = [50_000, 100_000]

    raffles.addRaffleTier()

    expect(raffles.raffleForm!.tier_costs).toEqual([50_000, 100_000, 100_000])
  })
})

describe('lookupEntries', () => {
  beforeEach(() => {
    lookup.mockClear()
    lookup.mockResolvedValue({ entries: [], truncated: false })
  })

  it('refuses a query shorter than two characters without calling the server', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)

    await raffles.lookupEntries('a')

    expect(lookup).not.toHaveBeenCalled()
    // No search ran, so the page must not show a "nothing matched" state.
    expect(raffles.entryLookupResults).toBeNull()
  })

  it('trims the query and keeps the results plus the truncated flag', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)
    const hit = {
      character_name: 'Aria Fairwind',
      world: 'Gilgamesh',
      num_entries: 3,
      paid_entries: 1,
      payment_state: 'partial',
    }
    lookup.mockResolvedValue({ entries: [hit], truncated: true })

    await raffles.lookupEntries('  fairwind  ')

    expect(lookup).toHaveBeenCalledWith(1, 'fairwind')
    expect(raffles.entryLookupResults).toEqual([hit])
    expect(raffles.entryLookupTruncated).toBe(true)
  })

  it('distinguishes a fruitless search from one that never ran', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)
    expect(raffles.entryLookupResults).toBeNull() // no search yet

    await raffles.lookupEntries('nobody')
    expect(raffles.entryLookupResults).toEqual([]) // searched, matched nothing

    // A FAILED search goes back to "never ran" - reporting "nothing matched"
    // would tell someone they had not entered when the search simply broke.
    lookup.mockRejectedValueOnce(new Error('network'))
    await raffles.lookupEntries('aria')
    expect(raffles.entryLookupResults).toBeNull()
    expect(raffles.entryLookupTruncated).toBe(false)
  })
})

describe('copyRaffleForm', () => {
  it('carries the reusable content, clears the window, and marks the title', () => {
    const raffles = useRafflesStore()
    const source = makeRaffle({
      id: 9,
      title: 'Obon Prize Draw',
      description: 'Win a house',
      rules: 'One per person',
      signup_instructions: 'Send the gil',
      entry_mode: 'custom',
      max_entries: 3,
      tier_costs: [50_000, 100_000, 150_000],
      prize_image: 'images/prize.png',
      pay_image: 'images/where.png',
      available_from: '2026-08-01T00:00:00.000Z',
      available_to: '2026-08-07T00:00:00.000Z',
      status: 'closed',
    })

    raffles.copyRaffleForm(source)
    const f = raffles.raffleForm!

    expect(f.title).toBe('Obon Prize Draw (Copy)')
    expect(f.entry_mode).toBe('custom')
    expect(f.tier_costs).toEqual([50_000, 100_000, 150_000])
    expect(f.prize_image).toBe('images/prize.png')
    expect(f.pay_image).toBe('images/where.png')
    // A new raffle opens on its own schedule, not the finished one's.
    expect(f.available_from).toBe('')
    expect(f.available_to).toBe('')
    // Saving must create rather than overwrite the raffle it was copied from.
    expect(f.id).toBe(0)
  })

  it('leaves the original untouched', () => {
    const raffles = useRafflesStore()
    const source = makeRaffle({ title: 'Original', tier_costs: [1, 2] })

    raffles.copyRaffleForm(source)
    raffles.raffleForm!.tier_costs[0] = 999

    expect(source.title).toBe('Original')
    expect(source.tier_costs).toEqual([1, 2])
  })
})

describe('setRaffleClosed', () => {
  beforeEach(() => {
    setStatus.mockClear()
    list.mockClear()
  })

  it('closes a raffle that has no winner, once confirmed', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    // A details-only raffle never reaches verify-winner, so this is its only exit.
    raffles.selectedRaffle = makeRaffle({ entry_mode: 'details', status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)

    await raffles.setRaffleClosed(true)

    expect(setStatus).toHaveBeenCalledWith(1, true)
    expect(raffles.selectedRaffle.status).toBe('closed')
  })

  it('does nothing when the confirmation is declined', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(false)

    await raffles.setRaffleClosed(true)

    expect(setStatus).not.toHaveBeenCalled()
    expect(raffles.selectedRaffle.status).toBe('open')
  })

  it('reopens without asking - it is the undo, not the destructive direction', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'closed' })
    const confirm = vi.spyOn(ui, 'confirm')

    await raffles.setRaffleClosed(false)

    expect(confirm).not.toHaveBeenCalled()
    expect(setStatus).toHaveBeenCalledWith(1, false)
    expect(raffles.selectedRaffle.status).toBe('open')
  })

  it('leaves the status alone when the request fails', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    setStatus.mockRejectedValueOnce(new Error('offline'))

    await raffles.setRaffleClosed(true)

    expect(raffles.selectedRaffle.status).toBe('open')
  })
})

// -- Festival Map linkage -----------------------------------------------------
//
// A raffle can be filed under a Festival Map and pinned to one stall on it, so
// the stall's panel on the public plan links to the raffle while it is running.

describe('festival map linkage', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  function mapOccupant(over: Partial<FestivalStallOccupant> = {}): FestivalStallOccupant {
    return {
      id: 21,
      stall_id: 4,
      affiliate_id: 9,
      affiliate_name: 'The Green Gaelicat',
      title: 'The Green Gaelicat',
      description: '',
      stall_type: 'game',
      type_label: '',
      times: [],
      sort_order: 0,
      ...over,
    }
  }

  /** A map detail response wrapping the given occupants in one pitch. */
  function mapWith(occupants: FestivalStallOccupant[]) {
    return {
      map: {
        stalls: [
          {
            id: 4,
            map_id: 4,
            shape: 'rect',
            color: '',
            placement: { x: 0, y: 0, width: 10, height: 10, rotation: 0 },
            sort_order: 0,
            occupants,
          },
        ],
      } as unknown as FestivalMap,
    }
  }

  it('starts a new raffle belonging to no festival', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    expect(raffles.raffleForm!.festival_map_id).toBeNull()
    expect(raffles.raffleForm!.occupant_id).toBeNull()
  })

  it('flattens the linked map into a stall list', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant(), mapOccupant({ id: 22 })]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()

    await raffles.setFestivalMap(4)

    expect(raffles.raffleForm!.festival_map_id).toBe(4)
    expect(raffles.mapStalls.map((o) => o.id)).toEqual([21, 22])
  })

  it('clears the pinned stall when the map changes', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    await raffles.setFestivalMap(4)
    raffles.raffleForm!.occupant_id = 21

    await raffles.setFestivalMap(9)

    // The id belonged to the map that was linked before.
    expect(raffles.raffleForm!.occupant_id).toBeNull()
  })

  it('unlinking the map empties the stall list', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    await raffles.setFestivalMap(4)

    await raffles.setFestivalMap(null)

    expect(raffles.mapStalls).toEqual([])
    expect(raffles.raffleForm!.occupant_id).toBeNull()
  })

  it('a copy is not filed under the original festival', () => {
    const raffles = useRafflesStore()
    raffles.copyRaffleForm(
      makeRaffle({ title: 'Obon Raffle', festival_map_id: 4, occupant_id: 21 }),
    )
    // A copy is almost always next year's; keeping the link would quietly file it
    // under the finished festival.
    expect(raffles.raffleForm!.festival_map_id).toBeNull()
    expect(raffles.raffleForm!.occupant_id).toBeNull()
    expect(raffles.raffleForm!.title).toBe('Obon Raffle (Copy)')
  })

  it('sends both links on save', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Obon Raffle'
    await raffles.setFestivalMap(4)
    raffles.raffleForm!.occupant_id = 21

    expect(await raffles.saveRaffle()).toBe(true)
    expect(create).toHaveBeenCalledWith(
      expect.objectContaining({ festival_map_id: 4, occupant_id: 21 }),
    )
  })
})
