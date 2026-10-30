import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { FestivalDashboard, PublicFestivalMap, PublicStampCard } from '@/types/api'

const ep = vi.hoisted(() => ({
  publicDetail: vi.fn(),
  dashboard: vi.fn(),
  enter: vi.fn(),
  lookup: vi.fn(),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    festivalMaps: { publicDetail: ep.publicDetail },
    festivalDashboard: { get: ep.dashboard },
    raffles: { enter: ep.enter },
    stampSignup: { lookup: ep.lookup },
  },
}))

import { festivalConnectState, useFestivalDashboardStore } from './festivalDashboard'
import { useGaraponsStore } from './garapons'
import { useStampRalliesStore } from './stampRallies'
import {
  saveFestivalVisit,
  saveRallySignup,
  savedFestivalVisit,
  savedRaffleSignup,
  savedRallySignup,
} from '@/lib/signups'

function map(over: Partial<PublicFestivalMap> = {}): PublicFestivalMap {
  return { id: 7, title: 'Obon', slug: 'obon', stalls: [], times: [], ...over } as PublicFestivalMap
}

function card(): PublicStampCard {
  return {
    participant_name: 'Aria @ Gilgamesh',
    completed: false,
    rally: { id: 3, title: 'Obon Rally', completion_mode: 'all' },
    stamps: [
      { id: 1, occupant_id: 11, collected: true, stamp_type: 'food' },
      { id: 2, occupant_id: 12, collected: false, stamp_type: 'game' },
      { id: 3, collected: true, stamp_type: 'food' },
    ],
    prizes: [],
  } as unknown as PublicStampCard
}

function dashboard(over: Partial<FestivalDashboard> = {}): FestivalDashboard {
  return { participant_name: '', world: '', raffles: [], ...over }
}

/** What the last dashboard read sent. */
function lastSent(): { card_token: string; garapon_token: string; name: string; world: string } {
  return ep.dashboard.mock.calls.at(-1)![1]
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  window.history.replaceState(null, '')
  Object.values(ep).forEach((fn) => fn.mockReset())
  ep.publicDetail.mockResolvedValue(map({ stamp_rally_id: 3 }))
})

describe('opening a festival', () => {
  it('sends the links this device holds for the festival rally', async () => {
    saveRallySignup({
      rallyId: 3,
      rallyTitle: 'Obon Rally',
      name: 'Aria',
      world: 'Gilgamesh',
      cardToken: 'card-a',
      garaponToken: 'draw-a',
    })
    ep.dashboard.mockResolvedValue(dashboard())
    await useFestivalDashboardStore().open('obon')

    expect(ep.dashboard).toHaveBeenCalledWith('obon', {
      card_token: 'card-a',
      garapon_token: 'draw-a',
      name: 'Aria',
      world: 'Gilgamesh',
    })
  })

  it("hands the card to the stamp-rally store, so the card page's panel renders it", async () => {
    ep.dashboard.mockResolvedValue(
      dashboard({
        participant_name: 'Aria',
        world: 'Gilgamesh',
        card: card(),
        card_token: 'card-a',
        rally: { id: 3, title: 'Obon Rally', card_image: '', details: '', signup_open: true },
      }),
    )
    const store = useFestivalDashboardStore()
    await store.open('obon')

    expect(useStampRalliesStore().publicCard?.rally.title).toBe('Obon Rally')
    // Collected stamps that sit on a map occupant - the third is on none.
    expect([...store.collectedOccupantIds]).toEqual([11])
    // What the read confirmed is kept for next time.
    expect(savedRallySignup(3)?.cardToken).toBe('card-a')
    expect(savedFestivalVisit(7)).toMatchObject({ name: 'Aria', world: 'Gilgamesh', rallyId: 3 })
  })

  it('shows the drum without a drawing link, and only draws with one', async () => {
    const garapon = { id: 5, title: 'Drum', status: 'open', prizes: [] }
    ep.dashboard.mockResolvedValue(
      dashboard({
        garapon: {
          garapon,
          default_draws: 2,
          draws: [],
          has_paired_link: true,
          paired_draws_left: 2,
        } as unknown as FestivalDashboard['garapon'],
      }),
    )
    const store = useFestivalDashboardStore()
    await store.open('obon')

    expect(useGaraponsStore().publicGarapon?.title).toBe('Drum')
    expect(useGaraponsStore().publicPlayer).toBeNull()
    expect(store.garaponToken).toBe('')
  })

  it('reports a festival that is not published', async () => {
    ep.publicDetail.mockRejectedValue(new Error('404'))
    const store = useFestivalDashboardStore()
    await store.open('draft')
    expect(store.notFound).toBe(true)
    expect(ep.dashboard).not.toHaveBeenCalled()
  })
})

describe('links handed over from the card and drawing pages', () => {
  it("takes a drawing link in place of the device's own, and lets the server find its card", async () => {
    saveRallySignup({
      rallyId: 3,
      rallyTitle: 'Obon Rally',
      name: 'Aria',
      world: 'Gilgamesh',
      cardToken: 'card-a',
      garaponToken: 'draw-a',
    })
    window.history.replaceState(festivalConnectState({ garaponToken: 'draw-b' }), '')
    ep.dashboard.mockResolvedValue(dashboard())
    await useFestivalDashboardStore().open('obon')

    expect(lastSent().garapon_token).toBe('draw-b')
    // Card A belongs with draw A; the server returns draw B's own card.
    expect(lastSent().card_token).toBe('')
    // Taken off the history entry, so a reload can't replay it after a "forget".
    expect((window.history.state as Record<string, unknown>).festivalConnect).toBeNull()
  })

  it('drops the drawing link kept for a different card', async () => {
    saveRallySignup({
      rallyId: 3,
      rallyTitle: 'Obon Rally',
      name: 'Aria',
      world: 'Gilgamesh',
      cardToken: 'card-a',
      garaponToken: 'draw-a',
    })
    window.history.replaceState(festivalConnectState({ cardToken: 'card-b' }), '')
    ep.dashboard.mockResolvedValue(dashboard())
    await useFestivalDashboardStore().open('obon')

    expect(lastSent()).toMatchObject({ card_token: 'card-b', garapon_token: '' })
  })
})

describe('keeping the device honest', () => {
  it('forgets a saved card the festival no longer knows', async () => {
    saveRallySignup({
      rallyId: 3,
      rallyTitle: 'Obon Rally',
      name: 'Aria',
      world: 'Gilgamesh',
      cardToken: 'gone',
    })
    ep.dashboard.mockResolvedValue(dashboard())
    await useFestivalDashboardStore().open('obon')
    expect(savedRallySignup(3)).toBeUndefined()
  })

  it('reads again with the card when the map did not name the rally', async () => {
    // Outside its window the public map stops naming the rally, so the first read
    // goes out without the card the device holds for it.
    ep.publicDetail.mockResolvedValue(map({ stamp_rally_id: undefined }))
    saveRallySignup({
      rallyId: 3,
      rallyTitle: 'Obon Rally',
      name: 'Aria',
      world: 'Gilgamesh',
      cardToken: 'card-a',
    })
    const rally = { id: 3, title: 'Obon Rally', card_image: '', details: '', signup_open: false }
    ep.dashboard
      .mockResolvedValueOnce(dashboard({ rally }))
      .mockResolvedValueOnce(dashboard({ rally, card: card(), card_token: 'card-a' }))
    const store = useFestivalDashboardStore()
    await store.open('obon')

    expect(ep.dashboard).toHaveBeenCalledTimes(2)
    expect(lastSent().card_token).toBe('card-a')
    expect(store.dashboard?.card).toBeTruthy()
  })
})

describe('entering a raffle', () => {
  it('enters under the visitor the dashboard knows and keeps the spelling', async () => {
    saveFestivalVisit({ mapId: 7, name: 'Aria', world: 'Gilgamesh' })
    const raffle = { id: 9, title: 'Gaelicat Raffle', max_entries: 3 }
    ep.dashboard.mockResolvedValue(
      dashboard({
        participant_name: 'Aria',
        world: 'Gilgamesh',
        raffles: [
          { raffle, running: true, total_entries: 0 } as unknown as FestivalDashboard['raffles'][0],
        ],
      }),
    )
    ep.enter.mockResolvedValue({ message: 'Entered', amount_due: 50000 })
    const store = useFestivalDashboardStore()
    await store.open('obon')

    expect(await store.enterRaffle(9, 2, 'ts')).toBe(true)
    expect(ep.enter).toHaveBeenCalledWith(9, {
      character_name: 'Aria',
      world: 'Gilgamesh',
      num_entries: 2,
      turnstile_token: 'ts',
    })
    expect(store.entered[9]?.amount_due).toBe(50000)
    expect(savedRaffleSignup(9)).toMatchObject({ name: 'Aria', world: 'Gilgamesh' })
  })
})
