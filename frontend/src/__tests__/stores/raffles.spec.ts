import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Raffle, RaffleEntry, RaffleLookupEntry } from '@/api/client'
import type { FestivalMap, FestivalStallOccupant } from '@/api/client'
import { RAFFLE_MAX_ENTRIES } from '@/lib/constants'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

// The festival maps a raffle can be filed under are read from the same client: these
// answer its two festival-map reads.
const { mapList, mapDetail } = vi.hoisted(() => ({
  mapList: vi.fn(async () => ({ maps: [] as FestivalMap[] })),
  mapDetail: vi.fn(async () => ({ map: { stalls: [] } as unknown as FestivalMap })),
}))

import { useUiStore } from '@/stores/ui'
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
  entryTicketPrice,
} from '@/stores/raffles'

/** A raffle entry; each test overrides only the settlement fields it cares about. */
function makeEntry(overrides: Partial<RaffleEntry> = {}): RaffleEntry {
  return {
    id: 1,
    raffleId: 1,
    characterName: 'Aria',
    world: 'Gilgamesh',
    numEntries: 1,
    paidEntries: 0,
    amountWaived: 0,
    paid: false,
    paymentState: 'unpaid',
    createdAt: null,
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
    maxEntries: 1,
    signupInstructions: '',
    entryMode: 'single',
    costPerEntry: 0,
    tierCosts: [],
    availableFrom: null,
    availableTo: null,
    prizeImage: '',
    payImage: '',
    status: 'open',
    winnerEntryId: null,
    festivalMapId: null,
    festivalMapName: '',
    occupantId: null,
    stallName: '',
    createdAt: null,
    winnerName: null,
    paidTotal: null,
    ...overrides,
  }
}

/** Raffle charging its 1st/2nd/3rd entry a different price. */
function tiered(...costs: number[]): Raffle {
  return makeRaffle({ entryMode: 'custom', maxEntries: costs.length, tierCosts: costs })
}

/** Minimal open raffle with the fields the sign-up math reads. */
function raffle(maxEntries: number, costPerEntry: number): Raffle {
  return makeRaffle({ maxEntries, costPerEntry })
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) =>
    path === '/festival-maps'
      ? ok(await mapList())
      : path === '/festival-maps/{id}'
        ? ok((await mapDetail()).map)
        : ok({ raffles: [] }),
  )
  api.POST.mockResolvedValue(ok(makeRaffle(), 201))
  api.PUT.mockResolvedValue(ok(makeRaffle()))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
  mapDetail.mockClear()
})

describe('raffle entry clamping', () => {
  it('clamps the submitted entry count up to 1 and down to maxEntries', () => {
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
    expect(isRaffleEnterable(makeRaffle({ availableTo: iso(HOUR) }))).toBe(true)
  })

  it('is false for an open raffle whose end has passed', () => {
    expect(isRaffleEnterable(makeRaffle({ availableTo: iso(-HOUR) }))).toBe(false)
  })

  it('is false for an open raffle that has not started yet', () => {
    expect(isRaffleEnterable(makeRaffle({ availableFrom: iso(HOUR) }))).toBe(false)
  })

  it('is false for a closed raffle regardless of end date', () => {
    expect(isRaffleEnterable(makeRaffle({ status: 'closed', availableTo: iso(HOUR) }))).toBe(false)
  })
})

describe('loading', () => {
  it('loadRaffles keeps every raffle the server sent', async () => {
    api.GET.mockResolvedValueOnce(ok({ raffles: [makeRaffle({ id: 1 }), makeRaffle({ id: 2 })] }))
    const raffles = useRafflesStore()
    await raffles.loadRaffles()
    expect(api.GET).toHaveBeenCalledWith('/raffles')
    expect(raffles.raffles.map((r) => r.id)).toEqual([1, 2])
  })

  it('loadHomeRaffles keeps only what the public can enter now', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        raffles: [
          makeRaffle({ id: 1 }),
          makeRaffle({ id: 2, status: 'closed' }),
          makeRaffle({ id: 3, availableFrom: new Date(Date.now() + 3600_000).toISOString() }),
        ],
      }),
    )
    const raffles = useRafflesStore()
    await raffles.loadHomeRaffles()
    expect(raffles.homeRaffles.map((r) => r.id)).toEqual([1])
  })

  it('loadRaffleDetail reads the raffle and its entries and finds the pending winner', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = makeRaffle({ id: 4, winnerName: 'Aria @ Gilgamesh', paidTotal: 700 })
    api.GET.mockImplementation(async (path: string) =>
      path === '/raffles/{id}'
        ? ok({
            raffle: makeRaffle({ id: 4, winnerEntryId: 2 }),
            totalEntries: 3,
            winnerEntry: null,
          })
        : ok({ entries: [makeEntry({ id: 1 }), makeEntry({ id: 2 })] }),
    )

    await raffles.loadRaffleDetail(4)

    expect(api.GET).toHaveBeenCalledWith('/raffles/{id}', { params: { path: { id: 4 } } })
    expect(api.GET).toHaveBeenCalledWith('/raffles/{id}/entries', { params: { path: { id: 4 } } })
    expect(raffles.raffleEntries).toHaveLength(2)
    expect(raffles.raffleWinner?.id).toBe(2)
    // The detail leaves the staff totals null; the list's are kept.
    expect(raffles.selectedRaffle).toMatchObject({ winnerName: 'Aria @ Gilgamesh', paidTotal: 700 })
  })

  it('loadPublicRaffleById answers false for a raffle that cannot be read', async () => {
    api.GET.mockResolvedValueOnce({
      error: { title: 'Not Found', detail: 'There is no such raffle.' },
      response: new Response(null, { status: 404 }),
    })
    const raffles = useRafflesStore()
    expect(await raffles.loadPublicRaffleById(9)).toBe(false)
    expect(raffles.selectedRaffle).toBeNull()
  })

  it('loadPublicRaffleById keeps the total and the winner for the page', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        raffle: makeRaffle({ id: 9, status: 'closed' }),
        totalEntries: 12,
        winnerEntry: { characterName: 'Aria', world: 'Gilgamesh', numEntries: 3 },
      }),
    )
    const raffles = useRafflesStore()
    expect(await raffles.loadPublicRaffleById(9)).toBe(true)
    expect(raffles.raffleTotalEntryCount).toBe(12)
    expect(raffles.raffleWinnerEntry?.characterName).toBe('Aria')
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

describe('enterRaffle', () => {
  it('signs up under the trimmed spelling and keeps the confirmation', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.raffleSignup = { characterName: ' Aria ', world: ' Gilgamesh ', numEntries: 99 }
    raffles.signupTurnstileToken = 'ts'
    api.POST.mockResolvedValueOnce(
      ok(
        { message: 'Signed up successfully', totalEntries: 5, totalCost: 500, amountDue: 500 },
        201,
      ),
    )

    await raffles.enterRaffle()

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/enter', {
      params: { path: { id: 1 } },
      body: { characterName: 'Aria', world: 'Gilgamesh', numEntries: 5, turnstileToken: 'ts' },
    })
    expect(raffles.raffleSignupResult?.amountDue).toBe(500)
    // Turnstile tokens are single-use.
    expect(raffles.signupTurnstileToken).toBe('')
  })

  it('does not call the server without a name and world', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.raffleSignup = { characterName: ' ', world: 'Gilgamesh', numEntries: 1 }
    await raffles.enterRaffle()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('shows the server problem and records no confirmation when refused', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = raffle(5, 100)
    raffles.raffleSignup = { characterName: 'Aria', world: 'Gilgamesh', numEntries: 1 }
    api.POST.mockResolvedValueOnce({
      error: { title: 'Conflict', detail: 'This raffle is no longer accepting entries.' },
      response: new Response(null, { status: 409 }),
    })

    await raffles.enterRaffle()

    expect(raffles.raffleSignupResult).toBeNull()
    expect(ui.toast.message).toContain('no longer accepting')
  })
})

describe('addRaffleEntry (admin)', () => {
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
    api.POST.mockResolvedValueOnce(ok(makeEntry(), 201))
    api.GET.mockResolvedValue(ok({ raffle: makeRaffle(), totalEntries: 0, entries: [] }))

    await raffles.addRaffleEntry()

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/entries', {
      params: { path: { id: 1 } },
      body: {
        characterName: 'Cloud',
        world: 'Gaia',
        numEntries: 5, // clamped down to maxEntries
        paid: true,
        amountWaived: 0,
      },
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
    api.POST.mockResolvedValueOnce(ok(makeEntry(), 201))
    api.GET.mockResolvedValue(ok({ raffle: makeRaffle(), totalEntries: 0, entries: [] }))

    await raffles.addRaffleEntry()

    expect(api.POST).toHaveBeenCalledWith(
      '/raffles/{id}/entries',
      expect.objectContaining({ body: expect.objectContaining({ numEntries: 1 }) }),
    )
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

    expect(api.POST).not.toHaveBeenCalled()
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

    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('raffle entry modes', () => {
  it('treats an unknown or missing mode as the original flat cost', () => {
    // A raffle with no entry mode carries ''. That means "flat costPerEntry", so it
    // must never read as free or as no-sign-ups.
    const legacy = { entryMode: '', costPerEntry: 500, tierCosts: [] }
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
    const details = makeRaffle({ entryMode: 'details', maxEntries: 500_000_000 })
    const started = Date.now()
    expect(raffleEntryCost(details, 500_000_000)).toBe(0)
    expect(raffleEntryCost(tiered(50_000, 100_000), 500_000_000)).toBe(150_000)
    expect(Date.now() - started).toBeLessThan(1_000)
  })

  it('takes no sign-ups and costs nothing in details mode', () => {
    const r = makeRaffle({ entryMode: 'details', costPerEntry: 500, tierCosts: [9] })
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
    // This label lands in a card and an admin badge; a raffle may carry up to 100
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
    raffles.selectedRaffle = makeRaffle({ entryMode: 'details' })
    expect(isRaffleEnterable(raffles.selectedRaffle)).toBe(true) // still browsable
    expect(raffles.selectedRaffleEnterable).toBe(false)
  })
})

describe('saveRaffle payload', () => {
  it('sends only the costs the chosen mode uses', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Tiered'
    raffles.raffleForm!.entryMode = 'custom'
    raffles.raffleForm!.costPerEntry = 777
    raffles.raffleForm!.tierCosts = [50_000, 100_000]

    await raffles.saveRaffle()

    expect(api.POST).toHaveBeenCalledWith('/raffles', {
      body: expect.objectContaining({
        entryMode: 'custom',
        maxEntries: 2, // the ladder's length
        costPerEntry: 0,
        tierCosts: [50_000, 100_000],
      }),
    })
  })

  it('clears a stale ladder when saving in single mode', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Flat'
    raffles.raffleForm!.entryMode = 'single'
    raffles.raffleForm!.costPerEntry = 250
    raffles.raffleForm!.tierCosts = [50_000, 100_000] // left over from a mode switch

    await raffles.saveRaffle()

    expect(api.POST).toHaveBeenCalledWith('/raffles', {
      body: expect.objectContaining({ entryMode: 'single', costPerEntry: 250, tierCosts: [] }),
    })
  })

  it('sends the window as UTC instants, null when unset, and replaces by id', async () => {
    const raffles = useRafflesStore()
    raffles.editRaffleForm(
      makeRaffle({ id: 7, title: 'Edited', availableFrom: '2026-06-13T20:00:00Z' }),
    )
    raffles.raffleForm!.availableFrom = '2026-06-13T20:00'
    raffles.raffleForm!.availableTo = ''

    await raffles.saveRaffle()

    const [path, init] = api.PUT.mock.calls[0]!
    expect(path).toBe('/raffles/{id}')
    expect(init.params).toEqual({ path: { id: 7 } })
    expect(init.body.availableFrom).toBe(new Date('2026-06-13T20:00').toISOString())
    expect(init.body.availableTo).toBeNull()
    expect(init.body).not.toHaveProperty('id')
  })

  it('rejects a negative price rather than sending it', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Bad'
    raffles.raffleForm!.entryMode = 'custom'
    raffles.raffleForm!.tierCosts = [50_000, -1]

    expect(await raffles.saveRaffle()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('rejects a price that is not whole gil, and an allowance past the cap', async () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Odd'
    raffles.raffleForm!.costPerEntry = 10.5
    expect(await raffles.saveRaffle()).toBe(false)

    raffles.raffleForm!.costPerEntry = 10
    raffles.raffleForm!.maxEntries = RAFFLE_MAX_ENTRIES + 1
    expect(await raffles.saveRaffle()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('entry settlement', () => {
  it('nets waivers off what was collected, priced through the ladder', () => {
    const r = tiered(50_000, 100_000, 150_000)
    // All three settled (300,000), 100,000 forgiven.
    const e = makeEntry({ numEntries: 3, paidEntries: 3, amountWaived: 100_000, paid: true })
    expect(entryTicketPrice(r, e)).toBe(300_000)
    expect(entryAmountCollected(r, e)).toBe(200_000)
    expect(entryAmountOutstanding(r, e)).toBe(0)
  })

  it('quotes only the unsettled tickets as outstanding', () => {
    const r = tiered(50_000, 100_000, 150_000)
    // First ticket settled; two bought since.
    const e = makeEntry({ numEntries: 3, paidEntries: 1 })
    expect(entryAmountCollected(r, e)).toBe(50_000)
    expect(entryAmountOutstanding(r, e)).toBe(250_000)
  })

  it('never reports a negative collection when more was waived than owed', () => {
    const r = raffle(1, 1_000)
    const e = makeEntry({ numEntries: 1, paidEntries: 1, amountWaived: 9_999, paid: true })
    expect(entryAmountCollected(r, e)).toBe(0)
  })
})

describe('setEntryPaid', () => {
  const settle = '/raffles/{id}/entries/{entryId}/settle'
  const params = { path: { id: 1, entryId: 1 } }

  it('sends only what is being waived now, never a running total', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    // The entry already had 10,000 forgiven; this settlement forgives 5,000 more.
    const entry = makeEntry({ numEntries: 3, paidEntries: 1, amountWaived: 10_000 })

    await raffles.setEntryPaid(entry, true, 5_000)

    // No ticket count: left out, it settles every ticket.
    expect(api.POST).toHaveBeenCalledWith(settle, { params, body: { waived: 5_000 } })
  })

  it('passes the ticket count for a part payment', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    const entry = makeEntry({ numEntries: 3 })

    // Gil for two of the three entries now, the third later.
    await raffles.setEntryPaid(entry, true, 0, 2)

    expect(api.POST).toHaveBeenCalledWith(settle, { params, body: { paidEntries: 2 } })
  })

  it('leaves the count out when it covers every ticket', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)

    await raffles.setEntryPaid(makeEntry({ numEntries: 3 }), true, 0, 3)

    expect(api.POST).toHaveBeenCalledWith(settle, { params, body: {} })
  })

  it('clears a payment through the unsettle operation, forwarding nothing else', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)

    await raffles.setEntryPaid(makeEntry({ paid: true, paidEntries: 3 }), false, 5_000, 2)

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/entries/{entryId}/unsettle', { params })
  })

  it('adopts the row the server returns rather than guessing the new state', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    const entry = makeEntry({ numEntries: 3 })
    api.POST.mockResolvedValueOnce(
      ok(
        makeEntry({
          numEntries: 3,
          paidEntries: 3,
          amountWaived: 15_000,
          paid: true,
          paymentState: 'paid',
        }),
      ),
    )

    await raffles.setEntryPaid(entry, true, 5_000)

    // The accumulated waiver is the server's to compute - the local row takes it.
    expect(entry.amountWaived).toBe(15_000)
    expect(entry.paidEntries).toBe(3)
    expect(entry.paid).toBe(true)
    expect(entry.paymentState).toBe('paid')
  })
})

describe('winner commands', () => {
  it('pick-winner records the winner it is told', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    api.POST.mockResolvedValueOnce(ok(makeEntry({ id: 8 })))

    await raffles.pickRaffleWinner()

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/pick-winner', {
      params: { path: { id: 1 } },
    })
    expect(raffles.raffleWinner?.id).toBe(8)
    expect(raffles.selectedRaffle.winnerEntryId).toBe(8)
  })

  it('pick-another names the pending winner it replaces', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    raffles.raffleWinner = makeEntry({ id: 8 })
    api.POST.mockResolvedValueOnce(ok(makeEntry({ id: 9 })))

    await raffles.pickAnotherWinner()

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/pick-another', {
      params: { path: { id: 1 } },
      body: { replaces: 8 },
    })
    expect(raffles.raffleWinner.id).toBe(9)
  })

  it('reads the raffle again when a pick is refused, since another console may have moved it', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    raffles.raffleWinner = makeEntry({ id: 8 })
    api.POST.mockResolvedValueOnce({
      error: { title: 'Conflict', detail: 'The pending winner changed.' },
      response: new Response(null, { status: 409 }),
    })
    api.GET.mockResolvedValue(ok({ raffle: makeRaffle(), totalEntries: 0, entries: [] }))

    await raffles.pickAnotherWinner()

    expect(api.GET).toHaveBeenCalledWith('/raffles/{id}', { params: { path: { id: 1 } } })
    expect(raffles.raffleWinner).toBeNull()
  })

  it('verify names the winner it confirms and takes the closed status', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 50_000)
    raffles.raffleWinner = makeEntry({ id: 8 })
    api.POST.mockResolvedValueOnce(ok(makeRaffle({ status: 'closed', winnerEntryId: 8 })))

    await raffles.verifyRaffleWinner()

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/verify-winner', {
      params: { path: { id: 1 } },
      body: { winnerEntryId: 8 },
    })
    expect(raffles.selectedRaffle.status).toBe('closed')
  })

  it('deleting the pending winner clears it', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const entry = makeEntry({ id: 8 })
    raffles.selectedRaffle = makeRaffle({ winnerEntryId: 8 })
    raffles.raffleEntries = [entry]
    raffles.raffleWinner = entry

    await raffles.deleteEntry(entry)

    expect(api.DELETE).toHaveBeenCalledWith('/raffles/{id}/entries/{entryId}', {
      params: { path: { id: 1, entryId: 8 } },
    })
    expect(raffles.raffleEntries).toEqual([])
    expect(raffles.raffleWinner).toBeNull()
    expect(raffles.selectedRaffle.winnerEntryId).toBeNull()
  })
})

describe('addRaffleTier', () => {
  it('stops the ladder at the per-player entry cap', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.entryMode = 'custom'
    // The ladder length IS the allowance, so it can't grow past what the server
    // will accept.
    raffles.raffleForm!.tierCosts = Array.from({ length: RAFFLE_MAX_ENTRIES }, () => 100)

    raffles.addRaffleTier()

    expect(raffles.raffleForm!.tierCosts).toHaveLength(RAFFLE_MAX_ENTRIES)
  })

  it('seeds a new rung from the last one', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.tierCosts = [50_000, 100_000]

    raffles.addRaffleTier()

    expect(raffles.raffleForm!.tierCosts).toEqual([50_000, 100_000, 100_000])
  })
})

describe('lookupEntries', () => {
  it('refuses a query shorter than two characters without calling the server', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)

    await raffles.lookupEntries('a')

    expect(api.POST).not.toHaveBeenCalled()
    // No search ran, so the page must not show a "nothing matched" state.
    expect(raffles.entryLookupResults).toBeNull()
  })

  it('trims the query and keeps the results plus the truncated flag', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)
    const hit: RaffleLookupEntry = {
      characterName: 'Aria Fairwind',
      world: 'Gilgamesh',
      numEntries: 3,
      paidEntries: 1,
      paymentState: 'partial',
    }
    api.POST.mockResolvedValueOnce(ok({ entries: [hit], truncated: true }))

    await raffles.lookupEntries('  fairwind  ')

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/lookup', {
      params: { path: { id: 1 } },
      body: { name: 'fairwind' },
    })
    expect(raffles.entryLookupResults).toEqual([hit])
    expect(raffles.entryLookupTruncated).toBe(true)
  })

  it('distinguishes a fruitless search from one that never ran', async () => {
    const raffles = useRafflesStore()
    raffles.selectedRaffle = raffle(3, 100)
    expect(raffles.entryLookupResults).toBeNull() // no search yet

    api.POST.mockResolvedValueOnce(ok({ entries: [], truncated: false }))
    await raffles.lookupEntries('nobody')
    expect(raffles.entryLookupResults).toEqual([]) // searched, matched nothing

    // A FAILED search goes back to "never ran" - reporting "nothing matched"
    // would tell someone they had not entered when the search simply broke.
    api.POST.mockRejectedValueOnce(new Error('network'))
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
      signupInstructions: 'Send the gil',
      entryMode: 'custom',
      maxEntries: 3,
      tierCosts: [50_000, 100_000, 150_000],
      prizeImage: 'images/prize.png',
      payImage: 'images/where.png',
      availableFrom: '2026-08-01T00:00:00Z',
      availableTo: '2026-08-07T00:00:00Z',
      status: 'closed',
    })

    raffles.copyRaffleForm(source)
    const f = raffles.raffleForm!

    expect(f.title).toBe('Obon Prize Draw (Copy)')
    expect(f.entryMode).toBe('custom')
    expect(f.tierCosts).toEqual([50_000, 100_000, 150_000])
    expect(f.prizeImage).toBe('images/prize.png')
    expect(f.payImage).toBe('images/where.png')
    // A new raffle opens on its own schedule, not the finished one's.
    expect(f.availableFrom).toBe('')
    expect(f.availableTo).toBe('')
    // Saving must create rather than overwrite the raffle it was copied from.
    expect(f.id).toBe(0)
  })

  it('leaves the original untouched', () => {
    const raffles = useRafflesStore()
    const source = makeRaffle({ title: 'Original', tierCosts: [1, 2] })

    raffles.copyRaffleForm(source)
    raffles.raffleForm!.tierCosts[0] = 999

    expect(source.title).toBe('Original')
    expect(source.tierCosts).toEqual([1, 2])
  })
})

describe('setRaffleClosed', () => {
  it('closes a raffle that has no winner, once confirmed', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    // A details-only raffle never reaches verify-winner, so this is its only exit.
    raffles.selectedRaffle = makeRaffle({ entryMode: 'details', status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    api.POST.mockResolvedValueOnce(ok(makeRaffle({ entryMode: 'details', status: 'closed' })))

    await raffles.setRaffleClosed(true)

    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/close', { params: { path: { id: 1 } } })
    expect(raffles.selectedRaffle.status).toBe('closed')
  })

  it('does nothing when the confirmation is declined', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(false)

    await raffles.setRaffleClosed(true)

    expect(api.POST).not.toHaveBeenCalled()
    expect(raffles.selectedRaffle.status).toBe('open')
  })

  it('reopens without asking - it is the undo, not the destructive direction', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'closed' })
    const confirm = vi.spyOn(ui, 'confirm')
    api.POST.mockResolvedValueOnce(ok(makeRaffle({ status: 'open' })))

    await raffles.setRaffleClosed(false)

    expect(confirm).not.toHaveBeenCalled()
    expect(api.POST).toHaveBeenCalledWith('/raffles/{id}/reopen', { params: { path: { id: 1 } } })
    expect(raffles.selectedRaffle.status).toBe('open')
  })

  it('leaves the status alone when the request fails', async () => {
    const raffles = useRafflesStore()
    const ui = useUiStore()
    raffles.selectedRaffle = makeRaffle({ status: 'open' })
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    api.POST.mockRejectedValueOnce(new Error('offline'))

    await raffles.setRaffleClosed(true)

    expect(raffles.selectedRaffle.status).toBe('open')
  })
})

// -- Festival Map linkage -----------------------------------------------------
//
// A raffle can be filed under a Festival Map and pinned to one stall on it, so
// the stall's panel on the public plan links to the raffle while it is running.

describe('festival map linkage', () => {
  function mapOccupant(over: Partial<FestivalStallOccupant> = {}): FestivalStallOccupant {
    return {
      id: 21,
      affiliateId: 9,
      affiliateName: 'The Green Gaelicat',
      title: 'The Green Gaelicat',
      description: '',
      eventCarrd: '',
      stallType: 'game',
      typeLabel: '',
      times: [],
      sortOrder: 0,
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
            shape: 'rect',
            color: '',
            selectionColor: '',
            textColor: '',
            placement: { x: 0, y: 0, width: 10, height: 10, rotation: 0 },
            sortOrder: 0,
            occupants,
          },
        ],
      } as unknown as FestivalMap,
    }
  }

  it('starts a new raffle belonging to no festival', () => {
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    expect(raffles.raffleForm!.festivalMapId).toBeNull()
    expect(raffles.raffleForm!.occupantId).toBeNull()
  })

  it('flattens the linked map into a stall list', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant(), mapOccupant({ id: 22 })]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()

    await raffles.setFestivalMap(4)

    expect(raffles.raffleForm!.festivalMapId).toBe(4)
    expect(raffles.mapStalls.map((o) => o.id)).toEqual([21, 22])
  })

  it('clears the pinned stall when the map changes', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    await raffles.setFestivalMap(4)
    raffles.raffleForm!.occupantId = 21

    await raffles.setFestivalMap(9)

    // The id belongs to the map the raffle was switched away from, so it can't stand.
    expect(raffles.raffleForm!.occupantId).toBeNull()
  })

  it('unlinking the map empties the stall list', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    await raffles.setFestivalMap(4)

    await raffles.setFestivalMap(null)

    expect(raffles.mapStalls).toEqual([])
    expect(raffles.raffleForm!.occupantId).toBeNull()
  })

  it('a copy is not filed under the original festival', () => {
    const raffles = useRafflesStore()
    raffles.copyRaffleForm(makeRaffle({ title: 'Obon Raffle', festivalMapId: 4, occupantId: 21 }))
    // A copy is almost always next year's; keeping the link would quietly file it
    // under the finished festival.
    expect(raffles.raffleForm!.festivalMapId).toBeNull()
    expect(raffles.raffleForm!.occupantId).toBeNull()
    expect(raffles.raffleForm!.title).toBe('Obon Raffle (Copy)')
  })

  it('sends both links on save', async () => {
    mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const raffles = useRafflesStore()
    raffles.newRaffleForm()
    raffles.raffleForm!.title = 'Obon Raffle'
    await raffles.setFestivalMap(4)
    raffles.raffleForm!.occupantId = 21

    expect(await raffles.saveRaffle()).toBe(true)
    expect(api.POST).toHaveBeenCalledWith('/raffles', {
      body: expect.objectContaining({ festivalMapId: 4, occupantId: 21 }),
    })
  })
})
