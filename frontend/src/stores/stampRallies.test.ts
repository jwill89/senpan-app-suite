import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type {
  FestivalMap,
  FestivalStallOccupant,
  PublicStamp,
  PublicStampCard,
  SignupRally,
  StampLookupEntry,
  StampRally,
  StampRallyLogEntry,
  StampSignupResponse,
} from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ stamp_rallies: [] as StampRally[] })),
  create: vi.fn(async () => ({ ok: true })),
  update: vi.fn(async () => ({ ok: true })),
  stamp: vi.fn(async () => ({ card: {} as PublicStampCard, collected_stamp_id: 5 })),
  get: vi.fn(async () => ({}) as PublicStampCard),
  setStatus: vi.fn(async () => ({ ok: true })),
  signupList: vi.fn(async () => ({ rallies: [] as SignupRally[] })),
  // Typed as the full response so a test can override it with the optional garapon
  // fields, which a rally with a linked garapon returns.
  signUp: vi.fn(async (): Promise<StampSignupResponse> => ({
    participant_name: 'Yao Ming',
    rally_title: 'Festival',
    card_token: 'tok_card',
  })),
  signupLookup: vi.fn(async () => ({ entries: [] as StampLookupEntry[] })),
  mapList: vi.fn(async () => ({ maps: [] as FestivalMap[] })),
  mapDetail: vi.fn(async () => ({ map: { stalls: [] } as unknown as FestivalMap })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    stampRallies: {
      list: ep.list,
      detail: vi.fn(),
      logs: vi.fn(),
      create: ep.create,
      update: ep.update,
      delete: vi.fn(),
      setStatus: ep.setStatus,
      setStampPaused: vi.fn(),
      createCard: vi.fn(),
      deleteCard: vi.fn(),
    },
    stampCard: { get: ep.get, stamp: ep.stamp },
    stampSignup: { list: ep.signupList, signUp: ep.signUp, lookup: ep.signupLookup },
    affiliates: { list: vi.fn(async () => ({ affiliates: [] })) },
    festivalMaps: { list: ep.mapList, detail: ep.mapDetail },
  },
}))

import { useStampRalliesStore, groupedByParticipant } from './stampRallies'
import { useUiStore } from './ui'

function logRow(over: Partial<StampRallyLogEntry>): StampRallyLogEntry {
  return {
    card_id: 1,
    participant_name: 'A',
    stamp_id: 1,
    stall_name: 'X',
    stamp_type: 'food',
    stamped_at: '',
    ...over,
  }
}

function publicCard(over: Partial<PublicStampCard> = {}): PublicStampCard {
  return {
    rally: {
      id: 1,
      title: 'R',
      card_image: '',
      not_stamped_image: '',
      details: '',
      redeem_instructions: '',
      redeem_image: '',
      available_from: '',
      available_to: '',
      is_active: true,
      completion_mode: 'all',
      required_food: 0,
      required_game: 0,
    },
    participant_name: 'Tataru',
    completed: false,
    completed_at: '',
    stamps: [],
    prizes: [],
    prizes_revealed: false,
    ...over,
  }
}

function publicStamp(over: Partial<PublicStamp> = {}): PublicStamp {
  return {
    id: 1,
    affiliate_name: '',
    stamp_type: 'food',
    image: '',
    placement: { x: 0, y: 0, width: 10, height: 10, rotation: 0 },
    active_from: '',
    active_to: '',
    available: true,
    expired: false,
    collected: false,
    collected_at: '',
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('groupedByParticipant', () => {
  it('keeps each participant rows contiguous, groups ordered by first appearance', () => {
    const rows = [
      logRow({ participant_name: 'Bo', stamp_id: 1 }),
      logRow({ participant_name: 'Aria', stamp_id: 2 }),
      logRow({ participant_name: 'Bo', stamp_id: 3 }), // belongs with the first "Bo" block
      logRow({ participant_name: 'Aria', stamp_id: 4 }),
    ]
    const out = groupedByParticipant(rows)
    // Groups keyed on participant name (the snapshot, which survives card deletion).
    expect(out.map((r) => r.participant_name)).toEqual(['Bo', 'Bo', 'Aria', 'Aria'])
    // permutation: same length, same stamp ids present
    expect(out.map((r) => r.stamp_id).sort()).toEqual([1, 2, 3, 4])
  })
})

describe('admin', () => {
  it('loadRallies populates the list', async () => {
    ep.list.mockResolvedValueOnce({
      stamp_rallies: [{ id: 1 } as StampRally, { id: 2 } as StampRally],
    })
    const s = useStampRalliesStore()
    await s.loadRallies()
    expect(s.rallies).toHaveLength(2)
  })

  it('splits rallies into open and closed by status', () => {
    const s = useStampRalliesStore()
    s.rallies = [
      { id: 1, status: 'open' } as StampRally,
      { id: 2, status: 'closed' } as StampRally,
      { id: 3, status: 'open' } as StampRally,
    ]
    expect(s.openRallies.map((r) => r.id)).toEqual([1, 3])
    expect(s.closedRallies.map((r) => r.id)).toEqual([2])
  })

  it('setRallyStatus updates the selected + listed rally', async () => {
    const s = useStampRalliesStore()
    s.rallies = [{ id: 1, status: 'open' } as StampRally]
    s.selectedRally = s.rallies[0]
    await s.setRallyStatus(1, 'closed')
    expect(ep.setStatus).toHaveBeenCalledWith(1, 'closed')
    expect(s.rallies[0].status).toBe('closed')
    expect(s.selectedRally.status).toBe('closed')
  })

  it('setStampPausedInList toggles the loaded stall and adjusts the active count', async () => {
    const s = useStampRalliesStore()
    s.rallies = [{ id: 1, stamp_count: 2, active_stamp_count: 2 } as StampRally]
    s.cardStamps[1] = [
      { id: 10, paused: false },
      { id: 11, paused: false },
    ] as never
    await s.setStampPausedInList(1, 10, true)
    expect(s.cardStamps[1][0].paused).toBe(true)
    expect(s.rallies[0].active_stamp_count).toBe(1)
  })

  it('saveRally requires a title', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    s.newRallyForm()
    expect(await s.saveRally()).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('saveRally refuses per-type completion that requires nothing', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Festival'
    s.rallyForm!.completion_mode = 'counts'

    expect(await s.saveRally()).toBe(false)
    // Never reaches the API: 0/0 would finish every card at its first stamp.
    expect(ep.create).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalledWith(
      expect.stringContaining('how many food or game'),
      'error',
    )
  })

  it('saveRally settles the requirement fields into whole counts', async () => {
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Festival'
    s.rallyForm!.completion_mode = 'counts'
    // What an emptied / half-typed number input can leave behind.
    s.rallyForm!.required_food = 2.7
    s.rallyForm!.required_game = '' as unknown as number

    expect(await s.saveRally()).toBe(true)
    expect(ep.create).toHaveBeenCalledWith(
      expect.objectContaining({ required_food: 2, required_game: 0 }),
    )
  })

  it('saveRally creates and clears the form', async () => {
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Summer'
    expect(await s.saveRally()).toBe(true)
    expect(ep.create).toHaveBeenCalledTimes(1)
    expect(s.rallyForm).toBeNull()
  })
})

describe('public', () => {
  it('submitPassword announces completion only when this stamp finished the card', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    const done = publicCard({ completed: true })

    // The stamp that completes the card announces it...
    s.publicCard = publicCard({ completed: false })
    ep.stamp.mockResolvedValueOnce({ card: done, collected_stamp_id: 1 })
    await s.submitPassword('tok', 'alpha')
    expect(ui.notify).toHaveBeenCalledWith(expect.stringContaining('Card complete'), 'success')

    // ...an optional stall collected afterwards does not say it again.
    ui.notify = vi.fn()
    ep.stamp.mockResolvedValueOnce({ card: done, collected_stamp_id: 2 })
    await s.submitPassword('tok', 'bravo')
    expect(ui.notify).toHaveBeenCalledWith('Stamp collected!', 'success')
    expect(ui.notify).not.toHaveBeenCalledWith(expect.stringContaining('Card complete'), 'success')
  })

  it('submitPassword commits the refreshed card + last collected id', async () => {
    const card = publicCard({ completed: false })
    ep.stamp.mockResolvedValueOnce({ card, collected_stamp_id: 9 })
    const s = useStampRalliesStore()
    const ok = await s.submitPassword('tok', 'alpha')
    expect(ok).toBe(true)
    expect(s.publicCard).toEqual(card)
    expect(s.lastCollectedId).toBe(9)
  })

  it('submitPassword rejects an empty password without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    expect(await s.submitPassword('tok', '   ')).toBe(false)
    expect(ep.stamp).not.toHaveBeenCalled()
  })

  it('cardProgress counts the whole card on an "all" rally', () => {
    const s = useStampRalliesStore()
    s.publicCard = publicCard({
      stamps: [publicStamp({ id: 1, collected: true }), publicStamp({ id: 2, stamp_type: 'game' })],
    })
    expect(s.cardProgress.byType).toBe(false)
    expect(s.cardProgress.collected).toBe(1)
    expect(s.cardProgress.total).toBe(2)
  })

  it('cardProgress flags a requirement that closed stalls put out of reach', () => {
    const s = useStampRalliesStore()
    const card = publicCard({
      stamps: [
        publicStamp({ id: 1, collected: true }),
        // The only game stall closed for good with nothing collected from it.
        publicStamp({ id: 2, stamp_type: 'game', available: false, expired: true }),
      ],
    })
    card.rally.completion_mode = 'counts'
    card.rally.required_food = 1
    card.rally.required_game = 1
    s.publicCard = card

    expect(s.cardProgress.food.unreachable).toBe(false)
    expect(s.cardProgress.game.unreachable).toBe(true)
    // A stall that is merely closed right now can still be collected later.
    // (Mutate through the store, so the computed sees the change.)
    s.publicCard.stamps[1].expired = false
    expect(s.cardProgress.game.unreachable).toBe(false)
  })

  it('cardProgress splits by type against the requirement on a "counts" rally', () => {
    const s = useStampRalliesStore()
    const card = publicCard({
      stamps: [
        publicStamp({ id: 1, collected: true }),
        publicStamp({ id: 2 }),
        publicStamp({ id: 3, stamp_type: 'game', collected: true }),
        publicStamp({ id: 4, stamp_type: 'game' }),
      ],
    })
    card.rally.completion_mode = 'counts'
    card.rally.required_food = 2
    card.rally.required_game = 1
    s.publicCard = card

    expect(s.cardProgress.byType).toBe(true)
    expect(s.cardProgress.food).toEqual({
      collected: 1,
      open: 1,
      required: 2,
      total: 2,
      unreachable: false,
    })
    expect(s.cardProgress.game).toEqual({
      collected: 1,
      open: 1,
      required: 1,
      total: 2,
      unreachable: false,
    })
  })
})

describe('public sign-up + lookup', () => {
  it('signUp trims the name and keeps the issued tokens', async () => {
    ep.signUp.mockResolvedValueOnce({
      participant_name: 'Yao Ming @ Balmung',
      rally_title: 'Festival',
      card_token: 'tok_card',
      garapon_token: 'tok_card',
      garapon_title: 'Festival Garapon',
    })
    const s = useStampRalliesStore()
    expect(await s.signUp(3, '  Yao Ming @ Balmung  ')).toBe(true)
    expect(ep.signUp).toHaveBeenCalledWith(3, 'Yao Ming @ Balmung', '')
    expect(s.signupResult?.card_token).toBe('tok_card')
    // A paired garapon shares the card's token, so both links resolve from it.
    expect(s.garaponUrl('tok_card')).toContain('/garapon/tok_card')
    expect(s.stampCardUrl('tok_card')).toContain('/stamp-card/tok_card')
  })

  it('signUp rejects a blank name without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    expect(await s.signUp(3, '   ')).toBe(false)
    expect(ep.signUp).not.toHaveBeenCalled()
  })

  it('signUp surfaces the server message on a taken name and keeps no result', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    ep.signUp.mockRejectedValueOnce(new Error('Someone has already signed up under that name.'))
    const s = useStampRalliesStore()
    expect(await s.signUp(3, 'Yao Ming')).toBe(false)
    expect(s.signupResult).toBeNull()
    expect(ui.notify).toHaveBeenCalledWith(
      'Someone has already signed up under that name.',
      'error',
    )
  })

  it('lookupLinks distinguishes "not searched" from "searched, found nothing"', async () => {
    const s = useStampRalliesStore()
    // Nothing searched yet -> null, so the view claims nothing either way.
    expect(s.lookupResults).toBeNull()

    ep.signupLookup.mockResolvedValueOnce({ entries: [] })
    await s.lookupLinks('Nobody')
    expect(s.lookupResults).toEqual([])

    s.resetLookup()
    expect(s.lookupResults).toBeNull()
  })

  it('lookupLinks rejects a blank name without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    await s.lookupLinks('  ')
    expect(ep.signupLookup).not.toHaveBeenCalled()
  })
})

describe('copyRallyForm', () => {
  /** A rally with the pieces a duplicate has to carry over - and the ones it must not. */
  function sourceRally(): StampRally {
    return {
      id: 7,
      title: 'Obon Rally',
      card_image: 'images/rally/card.png',
      not_stamped_image: 'images/rally/blank.png',
      available_from: '2026-08-01T00:00:00.000Z',
      available_to: '2026-08-07T00:00:00.000Z',
      details: 'Collect them all',
      redeem_instructions: 'See staff',
      redeem_image: 'images/rally/where.png',
      public_signup: true,
      completion_mode: 'counts',
      required_food: 1,
      required_game: 1,
      status: 'closed',
      created_at: '',
      stamps: [
        {
          id: 41,
          rally_id: 7,
          affiliate_id: 3,
          affiliate_name: 'Lunaria',
          image: 'images/stamp.png',
          password: 'moon',
          stamp_type: 'game',
          placement: { x: 1, y: 2, width: 10, height: 10, rotation: 0 },
          active_from: '2026-08-01T00:00:00.000Z',
          active_to: '2026-08-02T00:00:00.000Z',
          paused: true,
        },
      ],
      prizes: [
        {
          id: 88,
          rally_id: 7,
          name: 'Grand',
          image: 'images/prize.png',
          placement: { x: 3, y: 4, width: 20, height: 20, rotation: 0 },
        },
      ],
    } as unknown as StampRally
  }

  it('carries the reusable content and marks the title as a copy', () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(sourceRally())
    const f = store.rallyForm!

    expect(f.title).toBe('Obon Rally (Copy)')
    expect(f.card_image).toBe('images/rally/card.png')
    expect(f.redeem_instructions).toBe('See staff')
    expect(f.public_signup).toBe(true)
    expect(f.stamps).toHaveLength(1)
    expect(f.stamps[0].password).toBe('moon')
    expect(f.stamps[0].affiliate_id).toBe(3)
    expect(f.stamps[0].stamp_type).toBe('game')
    // The completion rule describes the event, not the run that already happened.
    expect(f.completion_mode).toBe('counts')
    expect(f.required_food).toBe(1)
    expect(f.required_game).toBe(1)
    expect(f.prizes[0].name).toBe('Grand')
  })

  it('drops every id so saving creates instead of overwriting the original', () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(sourceRally())
    const f = store.rallyForm!

    expect(f.id).toBe(0)
    expect(f.stamps[0].id).toBe(0)
    expect(f.prizes[0].id).toBe(0)
  })

  it('clears the dates and paused flags tied to the run that already happened', () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(sourceRally())
    const f = store.rallyForm!

    expect(f.available_from).toBe('')
    expect(f.available_to).toBe('')
    // A stale per-stamp window would silently gate a stall on last event's dates.
    expect(f.stamps[0].active_from).toBe('')
    expect(f.stamps[0].active_to).toBe('')
    expect(f.stamps[0].paused).toBe(false)
  })

  it('drops the festival link, so a copy is not filed under the finished festival', () => {
    const store = useStampRalliesStore()
    const source = sourceRally()
    source.festival_map_id = 4
    source.stamps![0].occupant_id = 21
    store.copyRallyForm(source)
    const f = store.rallyForm!

    // The original's stalls still exist, so nothing on the server would clear
    // these - next year's rally would silently name last year's stalls.
    expect(f.festival_map_id).toBeNull()
    expect(f.stamps[0].occupant_id).toBeNull()
  })

  it('leaves the original untouched', () => {
    const store = useStampRalliesStore()
    const source = sourceRally()
    store.copyRallyForm(source)
    store.rallyForm!.title = 'Something else'
    store.rallyForm!.stamps[0].password = 'changed'

    expect(source.title).toBe('Obon Rally')
    expect(source.stamps?.[0].password).toBe('moon')
    expect(source.stamps?.[0].id).toBe(41)
  })
})

// -- Festival Map linkage -----------------------------------------------------
//
// A rally linked to a map names that map's STALLS instead of bare affiliates, so
// the map can badge the stalls that are part of the rally.

describe('festival map linkage', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  /** One occupant of a map pitch - what the rally's stall select actually lists. */
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

  it('flattens the linked map into an occupant list and clears stamps on change', async () => {
    ep.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    store.rallyForm!.stamps[0].occupant_id = 999 // belonged to some other map

    await store.setFestivalMap(4)

    expect(store.rallyForm!.festival_map_id).toBe(4)
    expect(store.mapStalls).toHaveLength(1)
    // The old stall id belonged to the previously-linked map, so it can't stand.
    expect(store.rallyForm!.stamps[0].occupant_id).toBeNull()
  })

  it('unlinking clears the stall list and every stamp stall', async () => {
    ep.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    await store.setFestivalMap(4)
    store.setStampStall(0, 21)

    await store.setFestivalMap(null)

    expect(store.mapStalls).toEqual([])
    expect(store.rallyForm!.stamps[0].occupant_id).toBeNull()
  })

  it("takes the stall's affiliate and seeds the stamp type from what it offers", async () => {
    ep.mapDetail.mockResolvedValue(
      mapWith([
        mapOccupant(),
        mapOccupant({ id: 22, stall_type: 'food', affiliate_id: undefined }),
      ]),
    )
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    await store.setFestivalMap(4)

    store.setStampStall(0, 21)
    expect(store.rallyForm!.stamps[0].affiliate_id).toBe(9)
    expect(store.rallyForm!.stamps[0].stamp_type).toBe('game')

    // A stall with no affiliate of its own falls back to the venue, as a food stamp.
    store.setStampStall(0, 22)
    expect(store.rallyForm!.stamps[0].affiliate_id).toBeNull()
    expect(store.rallyForm!.stamps[0].stamp_type).toBe('food')
  })

  it('sends the map link and each stamp stall on save', async () => {
    ep.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.rallyForm!.title = 'Obon Rally'
    store.addStamp('food')
    await store.setFestivalMap(4)
    store.setStampStall(0, 21)

    expect(await store.saveRally()).toBe(true)
    const payload = (ep.create.mock.calls.at(-1) as unknown[])[0] as Record<string, unknown>
    expect(payload.festival_map_id).toBe(4)
    expect((payload.stamps as Record<string, unknown>[])[0].occupant_id).toBe(21)
  })
})
