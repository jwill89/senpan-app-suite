import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type {
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
