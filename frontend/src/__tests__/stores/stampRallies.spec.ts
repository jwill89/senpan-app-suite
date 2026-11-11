import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type {
  PublicStamp,
  PublicStampCard,
  StampRally,
  StampRallyLogEntry,
  StampRallyStamp,
  StampSignup,
} from '@/api/client'
import type { FestivalMap, FestivalStallOccupant } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const failed = (status: number, detail: string) => ({
  error: { title: 'Error', detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

// The festival maps a rally can be linked to are read from the same client: these
// answer its two festival-map reads.
const maps = vi.hoisted(() => ({
  mapList: vi.fn(async () => ({ maps: [] as FestivalMap[] })),
  mapDetail: vi.fn(async () => ({ map: { stalls: [] } as unknown as FestivalMap })),
}))

import { useStampRalliesStore, groupedByParticipant } from '@/stores/stampRallies'
import { useUiStore } from '@/stores/ui'

/** What the store passes a call: the path parameters and the body. */
interface CallInit {
  params: { path: Record<string, unknown> }
  body: Record<string, unknown>
}

/** The call to `path` among a method's calls, as `[path, init]`. */
function callTo(method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE', path: string) {
  return api[method].mock.calls.find(([p]) => p === path) as [string, CallInit] | undefined
}

function logRow(over: Partial<StampRallyLogEntry>): StampRallyLogEntry {
  return {
    cardId: 1,
    participantName: 'A',
    stampId: 1,
    stallName: 'X',
    stampType: 'food',
    stampedAt: null,
    ...over,
  }
}

function publicCard(over: Partial<PublicStampCard> = {}): PublicStampCard {
  return {
    rally: {
      id: 1,
      title: 'R',
      cardImage: '',
      notStampedImage: '',
      details: '',
      redeemInstructions: '',
      redeemImage: '',
      availableFrom: null,
      availableTo: null,
      isActive: true,
      completionMode: 'all',
      requiredFood: 0,
      requiredGame: 0,
      festivalLink: '',
    },
    participantName: 'Tataru',
    completed: false,
    completedAt: null,
    stamps: [],
    prizes: [],
    prizesRevealed: false,
    ...over,
  }
}

function publicStamp(over: Partial<PublicStamp> = {}): PublicStamp {
  return {
    id: 1,
    affiliateName: '',
    occupantId: null,
    stampType: 'food',
    image: '',
    placement: { x: 0, y: 0, width: 10, height: 10, rotation: 0 },
    activeFrom: null,
    activeTo: null,
    available: true,
    expired: false,
    collected: false,
    collectedAt: null,
    ...over,
  }
}

/** A rally as the API answers it; each test overrides what it cares about. */
function rally(over: Partial<StampRally> = {}): StampRally {
  return {
    id: 1,
    title: 'R',
    cardImage: '',
    notStampedImage: '',
    redeemImage: '',
    availableFrom: null,
    availableTo: null,
    details: '',
    redeemInstructions: '',
    status: 'open',
    publicSignup: false,
    completionMode: 'all',
    requiredFood: 0,
    requiredGame: 0,
    festivalMapId: null,
    festivalMapName: '',
    createdAt: null,
    stamps: [],
    prizes: [],
    cardCount: 0,
    completedCount: 0,
    stampCount: 0,
    activeStampCount: 0,
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  Object.values(maps).forEach((fn) => fn.mockClear())
  api.GET.mockImplementation(async (path: string) =>
    path === '/festival-maps'
      ? ok(await maps.mapList())
      : path === '/festival-maps/{id}'
        ? ok((await maps.mapDetail()).map)
        : ok({}),
  )
})

describe('groupedByParticipant', () => {
  it('keeps each participant rows contiguous, groups ordered by first appearance', () => {
    const rows = [
      logRow({ participantName: 'Bo', stampId: 1 }),
      logRow({ participantName: 'Aria', stampId: 2 }),
      logRow({ participantName: 'Bo', stampId: 3 }), // belongs with the first "Bo" block
      logRow({ participantName: 'Aria', stampId: 4 }),
    ]
    const out = groupedByParticipant(rows)
    // Groups keyed on participant name (the snapshot, which survives card deletion).
    expect(out.map((r) => r.participantName)).toEqual(['Bo', 'Bo', 'Aria', 'Aria'])
    // permutation: same length, same stamp ids present
    expect(out.map((r) => r.stampId).sort()).toEqual([1, 2, 3, 4])
  })
})

describe('admin', () => {
  it('loadRallies populates the list', async () => {
    api.GET.mockResolvedValueOnce(ok({ stampRallies: [rally({ id: 1 }), rally({ id: 2 })] }))
    const s = useStampRalliesStore()
    await s.loadRallies()
    expect(s.rallies).toHaveLength(2)
  })

  it('splits rallies into open and closed by status', () => {
    const s = useStampRalliesStore()
    s.rallies = [
      rally({ id: 1, status: 'open' }),
      rally({ id: 2, status: 'closed' }),
      rally({ id: 3, status: 'open' }),
    ]
    expect(s.openRallies.map((r) => r.id)).toEqual([1, 3])
    expect(s.closedRallies.map((r) => r.id)).toEqual([2])
  })

  it('setRallyStatus updates the selected + listed rally', async () => {
    api.POST.mockResolvedValueOnce(ok(rally({ status: 'closed' })))
    const s = useStampRalliesStore()
    s.rallies = [rally({ id: 1, status: 'open' })]
    s.selectedRally = s.rallies[0]!
    await s.setRallyStatus(1, 'closed')
    expect(callTo('POST', '/stamp-rallies/{id}/close')?.[1].params.path).toEqual({ id: 1 })
    expect(s.rallies[0]!.status).toBe('closed')
    expect(s.selectedRally.status).toBe('closed')
  })

  it('setRallyStatus reopens through its own verb', async () => {
    api.POST.mockResolvedValueOnce(ok(rally({ status: 'open' })))
    const s = useStampRalliesStore()
    s.rallies = [rally({ id: 1, status: 'closed' })]
    await s.setRallyStatus(1, 'open')
    expect(callTo('POST', '/stamp-rallies/{id}/reopen')).toBeDefined()
    expect(s.rallies[0]!.status).toBe('open')
  })

  it('setStampPausedInList toggles the loaded stall and adjusts the active count', async () => {
    api.PATCH.mockResolvedValueOnce(ok({ id: 10, paused: true }))
    const s = useStampRalliesStore()
    s.rallies = [rally({ id: 1, stampCount: 2, activeStampCount: 2 })]
    s.cardStamps[1] = [
      { id: 10, paused: false },
      { id: 11, paused: false },
    ] as StampRallyStamp[]
    await s.setStampPausedInList(1, 10, true)
    const call = callTo('PATCH', '/stamp-rallies/{id}/stamps/{stampId}')
    expect(call?.[1].params.path).toEqual({ id: 1, stampId: 10 })
    expect(call?.[1].body).toEqual({ paused: true })
    expect(s.cardStamps[1][0]!.paused).toBe(true)
    expect(s.rallies[0]!.activeStampCount).toBe(1)
  })

  it('saveRally requires a title', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    s.newRallyForm()
    expect(await s.saveRally()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('saveRally refuses per-type completion that requires nothing', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Festival'
    s.rallyForm!.completionMode = 'counts'

    expect(await s.saveRally()).toBe(false)
    // Never reaches the API: 0/0 would finish every card at its first stamp.
    expect(api.POST).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalledWith(
      expect.stringContaining('how many food or game'),
      'error',
    )
  })

  it('saveRally settles the requirement fields into whole counts', async () => {
    api.POST.mockResolvedValueOnce(ok(rally(), 201))
    api.GET.mockResolvedValueOnce(ok({ stampRallies: [] }))
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Festival'
    s.rallyForm!.completionMode = 'counts'
    // What an emptied / half-typed number input can leave behind.
    s.rallyForm!.requiredFood = 2.7
    s.rallyForm!.requiredGame = '' as unknown as number

    expect(await s.saveRally()).toBe(true)
    expect(callTo('POST', '/stamp-rallies')?.[1].body).toEqual(
      expect.objectContaining({ requiredFood: 2, requiredGame: 0 }),
    )
  })

  it('saveRally creates, sends null for an empty window, and clears the form', async () => {
    api.POST.mockResolvedValueOnce(ok(rally(), 201))
    api.GET.mockResolvedValueOnce(ok({ stampRallies: [] }))
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.title = 'Summer'
    expect(await s.saveRally()).toBe(true)
    const body = callTo('POST', '/stamp-rallies')![1].body
    expect(body.availableFrom).toBeNull()
    expect(body.availableTo).toBeNull()
    expect(body).not.toHaveProperty('id')
    expect(s.rallyForm).toBeNull()
  })

  it('saveRally replaces an existing rally by its id', async () => {
    api.PUT.mockResolvedValueOnce(ok(rally({ id: 4 })))
    api.GET.mockResolvedValueOnce(ok({ stampRallies: [] }))
    const s = useStampRalliesStore()
    s.newRallyForm()
    s.rallyForm!.id = 4
    s.rallyForm!.title = 'Renamed'
    expect(await s.saveRally()).toBe(true)
    expect(callTo('PUT', '/stamp-rallies/{id}')?.[1].params.path).toEqual({ id: 4 })
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
    api.POST.mockResolvedValueOnce(ok({ card: done, collectedStampId: 1 }))
    await s.submitPassword('tok', 'alpha')
    expect(ui.notify).toHaveBeenCalledWith(expect.stringContaining('Card complete'), 'success')

    // ...an optional stall collected afterwards does not say it again.
    ui.notify = vi.fn()
    api.POST.mockResolvedValueOnce(ok({ card: done, collectedStampId: 2 }))
    await s.submitPassword('tok', 'bravo')
    expect(ui.notify).toHaveBeenCalledWith('Stamp collected!', 'success')
    expect(ui.notify).not.toHaveBeenCalledWith(expect.stringContaining('Card complete'), 'success')
  })

  it('submitPassword commits the refreshed card + last collected id', async () => {
    const card = publicCard({ completed: false })
    api.POST.mockResolvedValueOnce(ok({ card, collectedStampId: 9 }))
    const s = useStampRalliesStore()
    const done = await s.submitPassword('tok', 'alpha')
    expect(done).toBe(true)
    expect(s.publicCard).toEqual(card)
    expect(s.lastCollectedId).toBe(9)
    const call = callTo('POST', '/stamp-cards/{token}/stamp')
    expect(call?.[1].params.path).toEqual({ token: 'tok' })
    expect(call?.[1].body).toEqual({ password: 'alpha' })
  })

  it('submitPassword shows a refusal and keeps the card', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    api.POST.mockResolvedValueOnce(
      failed(422, "That password doesn't match any stamp on this card."),
    )
    const s = useStampRalliesStore()
    expect(await s.submitPassword('tok', 'nope')).toBe(false)
    expect(ui.notify).toHaveBeenCalledWith(
      "That password doesn't match any stamp on this card.",
      'error',
    )
    expect(s.submitting).toBe(false)
  })

  it('submitPassword rejects an empty password without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    expect(await s.submitPassword('tok', '   ')).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('loadByToken reads the card by its token', async () => {
    const card = publicCard()
    api.GET.mockResolvedValueOnce(ok(card))
    const s = useStampRalliesStore()
    expect(await s.loadByToken('tok')).toBe(true)
    expect(s.publicCard).toEqual(card)
    expect(callTo('GET', '/stamp-cards/{token}')?.[1].params.path).toEqual({ token: 'tok' })
  })

  it('loadByToken answers false for an unknown card', async () => {
    api.GET.mockResolvedValueOnce(failed(404, 'There is no such stamp card.'))
    const s = useStampRalliesStore()
    expect(await s.loadByToken('bad')).toBe(false)
    expect(s.publicCard).toBeNull()
  })

  it('cardProgress counts the whole card on an "all" rally', () => {
    const s = useStampRalliesStore()
    s.publicCard = publicCard({
      stamps: [publicStamp({ id: 1, collected: true }), publicStamp({ id: 2, stampType: 'game' })],
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
        publicStamp({ id: 2, stampType: 'game', available: false, expired: true }),
      ],
    })
    card.rally.completionMode = 'counts'
    card.rally.requiredFood = 1
    card.rally.requiredGame = 1
    s.publicCard = card

    expect(s.cardProgress.food.unreachable).toBe(false)
    expect(s.cardProgress.game.unreachable).toBe(true)
    // A stall that is merely closed right now can still be collected later.
    // (Mutate through the store, so the computed sees the change.)
    s.publicCard.stamps[1]!.expired = false
    expect(s.cardProgress.game.unreachable).toBe(false)
  })

  it('cardProgress splits by type against the requirement on a "counts" rally', () => {
    const s = useStampRalliesStore()
    const card = publicCard({
      stamps: [
        publicStamp({ id: 1, collected: true }),
        publicStamp({ id: 2 }),
        publicStamp({ id: 3, stampType: 'game', collected: true }),
        publicStamp({ id: 4, stampType: 'game' }),
      ],
    })
    card.rally.completionMode = 'counts'
    card.rally.requiredFood = 2
    card.rally.requiredGame = 1
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
  const issued: StampSignup = {
    participantName: 'Yao Ming',
    world: 'Balmung',
    rallyTitle: 'Festival',
    cardToken: 'tok_card',
    garaponToken: null,
    garaponTitle: null,
  }

  it('loadSignupRallies lists the rallies open to sign-up', async () => {
    api.GET.mockResolvedValueOnce(ok({ rallies: [{ id: 3, title: 'Festival' }] }))
    const s = useStampRalliesStore()
    await s.loadSignupRallies()
    expect(s.signupRallies).toHaveLength(1)
    expect(callTo('GET', '/stamp-rallies/public')).toBeDefined()
  })

  it('refuses to sign up without a world', async () => {
    // The world is what tells two players who share a character name apart. Sending
    // the name alone would create a record no other system can match.
    const s = useStampRalliesStore()
    expect(await s.signUp(3, 'Yao Ming', '   ')).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('signUp trims both fields and keeps the issued tokens', async () => {
    api.POST.mockResolvedValueOnce(
      ok(
        {
          ...issued,
          garaponToken: 'tok_draw',
          garaponTitle: 'Festival Garapon',
        },
        201,
      ),
    )
    const s = useStampRalliesStore()
    // Name and world go as two fields, the way every system stores a person.
    expect(await s.signUp(3, '  Yao Ming  ', '  Balmung  ')).toBe(true)
    const call = callTo('POST', '/stamp-rallies/{id}/sign-up')
    expect(call?.[1].params.path).toEqual({ id: 3 })
    expect(call?.[1].body).toEqual({
      characterName: 'Yao Ming',
      world: 'Balmung',
      turnstileToken: undefined,
    })
    expect(s.signupResult?.cardToken).toBe('tok_card')
    // The two tokens are SEPARATE secrets: the card link can be shared, the drawing
    // link spends draws. Sign-up is the only time the drawing token is issued, so
    // the store has to keep both rather than deriving one from the other.
    expect(s.signupResult?.garaponToken).toBe('tok_draw')
    expect(s.garaponUrl('tok_draw')).toContain('/garapon/tok_draw')
    expect(s.stampCardUrl('tok_card')).toContain('/stamp-card/tok_card')
  })

  it('signUp sends the bot check response when there is one', async () => {
    api.POST.mockResolvedValueOnce(ok(issued, 201))
    const s = useStampRalliesStore()
    await s.signUp(3, 'Yao Ming', 'Balmung', 'ts-token')
    expect(callTo('POST', '/stamp-rallies/{id}/sign-up')?.[1].body.turnstileToken).toBe('ts-token')
  })

  it('signUp rejects a blank name without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    expect(await s.signUp(3, '   ', 'Balmung', 'Balmung')).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('signUp surfaces the server message on a taken name and keeps no result', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    api.POST.mockResolvedValueOnce(failed(409, 'Someone has already signed up under that name.'))
    const s = useStampRalliesStore()
    expect(await s.signUp(3, 'Yao Ming', 'Balmung', 'Balmung')).toBe(false)
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

    api.POST.mockResolvedValueOnce(ok({ entries: [] }))
    await s.lookupLinks('Nobody', 'Balmung')
    expect(s.lookupResults).toEqual([])
    expect(callTo('POST', '/stamp-cards/lookup')?.[1].body).toEqual({
      characterName: 'Nobody',
      world: 'Balmung',
    })

    s.resetLookup()
    expect(s.lookupResults).toBeNull()
  })

  it('lookupLinks rejects a blank name without calling the API', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useStampRalliesStore()
    await s.lookupLinks('  ', 'Balmung')
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('cards', () => {
  it('createCard issues a card under the trimmed name and copies its link', async () => {
    Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => undefined) } })
    api.POST.mockResolvedValueOnce(ok({ id: 5, token: 'tok', participantName: 'Tataru' }, 201))
    api.GET.mockResolvedValueOnce(ok({ stampRally: rally({ id: 4 }), cards: [] }))
    const s = useStampRalliesStore()
    s.selectedRally = rally({ id: 4 })
    s.cardAdd = { participantName: '  Tataru  ' }
    await s.createCard()
    const call = callTo('POST', '/stamp-rallies/{id}/cards')
    expect(call?.[1].params.path).toEqual({ id: 4 })
    expect(call?.[1].body).toEqual({ participantName: 'Tataru' })
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(
      expect.stringMatching(/\/stamp-card\/tok$/),
    )
    expect(s.cardAdd.participantName).toBe('')
  })

  it('deleteCard asks first, then removes the card from the list', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    api.DELETE.mockResolvedValueOnce(ok(undefined, 204))
    const s = useStampRalliesStore()
    s.selectedRally = rally({ id: 4 })
    s.rallyCards = [{ id: 12, participantName: 'Tataru' } as never, { id: 13 } as never]
    await s.deleteCard(s.rallyCards[0]!)
    expect(callTo('DELETE', '/stamp-rallies/{id}/cards/{cardId}')?.[1].params.path).toEqual({
      id: 4,
      cardId: 12,
    })
    expect(s.rallyCards.map((c) => c.id)).toEqual([13])
  })
})

describe('copyRallyForm', () => {
  /** A rally with the pieces a duplicate has to carry over - and the ones it must not. */
  function sourceRally(): StampRally {
    return rally({
      id: 7,
      title: 'Obon Rally',
      cardImage: 'images/rally/card.png',
      notStampedImage: 'images/rally/blank.png',
      availableFrom: '2026-08-01T00:00:00.000Z',
      availableTo: '2026-08-07T00:00:00.000Z',
      details: 'Collect them all',
      redeemInstructions: 'See staff',
      redeemImage: 'images/rally/where.png',
      publicSignup: true,
      completionMode: 'counts',
      requiredFood: 1,
      requiredGame: 1,
      status: 'closed',
      stampCount: 1,
      stamps: [
        {
          id: 41,
          occupantId: null,
          stallName: '',
          affiliateId: 3,
          affiliateName: 'Lunaria',
          image: 'images/stamp.png',
          password: 'moon',
          stampType: 'game',
          placement: { x: 1, y: 2, width: 10, height: 10, rotation: 0 },
          activeFrom: '2026-08-01T00:00:00.000Z',
          activeTo: '2026-08-02T00:00:00.000Z',
          paused: true,
          sortOrder: 0,
        },
      ],
      prizes: [
        {
          id: 88,
          name: 'Grand',
          image: 'images/prize.png',
          placement: { x: 3, y: 4, width: 20, height: 20, rotation: 0 },
          sortOrder: 0,
        },
      ],
    })
  }

  /** Reads a rally's detail through the store, as the manager does before it edits one. */
  async function detailed(store: ReturnType<typeof useStampRalliesStore>, r: StampRally) {
    api.GET.mockResolvedValueOnce(ok({ stampRally: r, cards: [] }))
    await store.loadRallyDetail(r.id)
    return store.selectedRally!
  }

  it('carries the reusable content and marks the title as a copy', async () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(await detailed(store, sourceRally()))
    const f = store.rallyForm!

    expect(f.title).toBe('Obon Rally (Copy)')
    expect(f.cardImage).toBe('images/rally/card.png')
    expect(f.redeemInstructions).toBe('See staff')
    expect(f.publicSignup).toBe(true)
    expect(f.stamps).toHaveLength(1)
    expect(f.stamps[0]!.password).toBe('moon')
    expect(f.stamps[0]!.affiliateId).toBe(3)
    expect(f.stamps[0]!.stampType).toBe('game')
    // The completion rule describes the event, not the run that already happened.
    expect(f.completionMode).toBe('counts')
    expect(f.requiredFood).toBe(1)
    expect(f.requiredGame).toBe(1)
    expect(f.prizes[0]!.name).toBe('Grand')
  })

  /**
   * A save is a FULL REPLACE, and the admin LIST leaves stamps/prizes empty. So
   * seeding the form from a list row and saving it would delete every stamp on the
   * rally, taking every participant's collected rows with it. Both entry points
   * must refuse a rally that has not had its detail loaded.
   */
  it('refuses a rally whose detail has not loaded, rather than seeding an empty card', () => {
    const store = useStampRalliesStore()
    const listRow = { ...sourceRally(), stamps: [], prizes: [] }

    expect(store.hasRallyDetail(listRow)).toBe(false)
    expect(store.editRallyForm(listRow)).toBe(false)
    expect(store.copyRallyForm(listRow)).toBe(false)
    // Nothing was seeded, so there is no way to save the destructive payload.
    expect(store.rallyForm).toBeNull()
  })

  it('accepts a rally read by its detail', async () => {
    const store = useStampRalliesStore()
    const r = await detailed(store, sourceRally())
    expect(store.hasRallyDetail(r)).toBe(true)
    expect(store.editRallyForm(r)).toBe(true)
    expect(store.rallyForm!.stamps).toHaveLength(1)
  })

  it('drops every id so saving creates instead of overwriting the original', async () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(await detailed(store, sourceRally()))
    const f = store.rallyForm!

    expect(f.id).toBe(0)
    expect(f.stamps[0]!.id).toBe(0)
    expect(f.prizes[0]!.id).toBe(0)
  })

  it('clears the dates and paused flags tied to the run that already happened', async () => {
    const store = useStampRalliesStore()
    store.copyRallyForm(await detailed(store, sourceRally()))
    const f = store.rallyForm!

    expect(f.availableFrom).toBe('')
    expect(f.availableTo).toBe('')
    // A stale per-stamp window would silently gate a stall on last event's dates.
    expect(f.stamps[0]!.activeFrom).toBe('')
    expect(f.stamps[0]!.activeTo).toBe('')
    expect(f.stamps[0]!.paused).toBe(false)
  })

  it('drops the festival link, so a copy is not filed under the finished festival', async () => {
    const store = useStampRalliesStore()
    const source = sourceRally()
    source.festivalMapId = 4
    source.stamps[0]!.occupantId = 21
    store.copyRallyForm(await detailed(store, source))
    const f = store.rallyForm!

    // The original's stalls still exist, so nothing on the server would clear
    // these - next year's rally would silently name last year's stalls.
    expect(f.festivalMapId).toBeNull()
    expect(f.stamps[0]!.occupantId).toBeNull()
  })

  it('leaves the original untouched', async () => {
    const store = useStampRalliesStore()
    const source = sourceRally()
    store.copyRallyForm(await detailed(store, source))
    store.rallyForm!.title = 'Something else'
    store.rallyForm!.stamps[0]!.password = 'changed'

    expect(source.title).toBe('Obon Rally')
    expect(source.stamps[0]!.password).toBe('moon')
    expect(source.stamps[0]!.id).toBe(41)
  })
})

// -- Festival Map linkage -----------------------------------------------------
//
// A rally linked to a map names that map's STALLS instead of bare affiliates, so
// the map can badge the stalls that are part of the rally.

describe('festival map linkage', () => {
  /** One occupant of a map pitch - what the rally's stall select actually lists. */
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

  it('flattens the linked map into an occupant list and clears stamps on change', async () => {
    maps.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    store.rallyForm!.stamps[0]!.occupantId = 999 // belonged to some other map

    await store.setFestivalMap(4)

    expect(store.rallyForm!.festivalMapId).toBe(4)
    expect(store.mapStalls).toHaveLength(1)
    // A stall id from the map the rally was switched away from can't stand.
    expect(store.rallyForm!.stamps[0]!.occupantId).toBeNull()
  })

  it('unlinking clears the stall list and every stamp stall', async () => {
    maps.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    await store.setFestivalMap(4)
    store.setStampStall(0, 21)

    await store.setFestivalMap(null)

    expect(store.mapStalls).toEqual([])
    expect(store.rallyForm!.stamps[0]!.occupantId).toBeNull()
  })

  it("takes the stall's affiliate and seeds the stamp type from what it offers", async () => {
    maps.mapDetail.mockResolvedValue(
      mapWith([mapOccupant(), mapOccupant({ id: 22, stallType: 'food', affiliateId: undefined })]),
    )
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.addStamp('food')
    await store.setFestivalMap(4)

    store.setStampStall(0, 21)
    expect(store.rallyForm!.stamps[0]!.affiliateId).toBe(9)
    expect(store.rallyForm!.stamps[0]!.stampType).toBe('game')

    // A stall with no affiliate of its own falls back to the venue, as a food stamp.
    store.setStampStall(0, 22)
    expect(store.rallyForm!.stamps[0]!.affiliateId).toBeNull()
    expect(store.rallyForm!.stamps[0]!.stampType).toBe('food')
  })

  it('sends the map link and each stamp stall on save', async () => {
    maps.mapDetail.mockResolvedValue(mapWith([mapOccupant()]))
    api.POST.mockResolvedValueOnce(ok(rally(), 201))
    api.GET.mockResolvedValueOnce(ok({ stampRallies: [] }))
    const store = useStampRalliesStore()
    store.newRallyForm()
    store.rallyForm!.title = 'Obon Rally'
    store.addStamp('food')
    await store.setFestivalMap(4)
    store.setStampStall(0, 21)

    expect(await store.saveRally()).toBe(true)
    const body = callTo('POST', '/stamp-rallies')![1].body
    expect(body.festivalMapId).toBe(4)
    expect((body.stamps as Record<string, unknown>[])[0]!.occupantId).toBe(21)
  })
})
