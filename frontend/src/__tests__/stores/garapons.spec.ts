import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type {
  Garapon,
  GaraponDraw,
  GaraponPlayer,
  GaraponPlayerSummary,
  PublicGarapon,
} from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  DELETE: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useGaraponsStore } from '@/stores/garapons'
import { useUiStore } from '@/stores/ui'

/** What the store passes a call: the path parameters and the body. */
interface CallInit {
  params: { path: Record<string, unknown> }
  body: Record<string, unknown>
}

/** The call to `path` among a method's calls, as `[path, init]`. */
function callTo(method: 'GET' | 'POST' | 'PUT' | 'DELETE', path: string) {
  return api[method].mock.calls.find(([p]) => p === path) as [string, CallInit] | undefined
}

/** Minimal garapon with the fields the store reads. */
function garapon(over: Partial<Garapon> = {}): Garapon {
  return {
    id: 1,
    title: 'G',
    details: '',
    grandPrizeImage: '',
    status: 'open',
    stampRallyId: null,
    stampRallyTitle: '',
    defaultDraws: 1,
    createdAt: null,
    prizes: [],
    playerCount: 0,
    drawCount: 0,
    ...over,
  }
}

function publicGarapon(over: Partial<PublicGarapon> = {}): PublicGarapon {
  return {
    id: 1,
    title: 'G',
    details: '',
    grandPrizeImage: '',
    status: 'open',
    prizes: [],
    ...over,
  }
}

function summary(over: Partial<GaraponPlayerSummary> = {}): GaraponPlayerSummary {
  return { playerName: 'Hero', world: '', maxDraws: 3, drawsUsed: 0, stampCardToken: '', ...over }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  // jsdom has no clipboard by default; provide a resolving stub.
  Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => undefined) } })
})

describe('loading', () => {
  it('loadGarapons populates the list', async () => {
    api.GET.mockResolvedValueOnce(ok({ garapons: [garapon(), garapon({ id: 2 })] }))
    const s = useGaraponsStore()
    await s.loadGarapons()
    expect(s.garapons).toHaveLength(2)
    expect(s.garaponsLoading).toBe(false)
  })

  it('loadGaraponDetail populates selected garapon, players, and draws', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        garapon: garapon({ id: 5 }),
        players: [{ id: 1 } as GaraponPlayer],
        draws: [{ id: 1 }, { id: 2 }] as GaraponDraw[],
      }),
    )
    const s = useGaraponsStore()
    await s.loadGaraponDetail(5)
    expect(s.selectedGarapon?.id).toBe(5)
    expect(s.garaponPlayers).toHaveLength(1)
    expect(s.garaponDraws).toHaveLength(2)
    expect(callTo('GET', '/garapons/{id}')?.[1].params.path).toEqual({ id: 5 })
  })
})

describe('computed', () => {
  it('splits open and closed garapons', () => {
    const s = useGaraponsStore()
    s.garapons = [garapon(), garapon({ id: 2, status: 'closed' })]
    expect(s.openGarapons).toHaveLength(1)
    expect(s.closedGarapons).toHaveLength(1)
  })

  it('derives grand/other prizes and draw availability from the public garapon', () => {
    const s = useGaraponsStore()
    s.publicGarapon = publicGarapon({
      prizes: [
        { id: 1, name: 'Grand', ballColor: '#fff', isGrand: true },
        { id: 2, name: 'Other', ballColor: '#000', isGrand: false },
      ],
    })
    s.publicPlayer = summary({ drawsUsed: 1 })
    expect(s.grandPrize?.name).toBe('Grand')
    expect(s.otherPrizes).toHaveLength(1)
    expect(s.drawsRemaining).toBe(2)
    expect(s.canDraw).toBe(true)
  })

  it('canDraw is false when the allowance is exhausted', () => {
    const s = useGaraponsStore()
    s.publicGarapon = publicGarapon()
    s.publicPlayer = summary({ maxDraws: 1, drawsUsed: 1 })
    expect(s.drawsRemaining).toBe(0)
    expect(s.canDraw).toBe(false)
  })
})

describe('form editing', () => {
  it('newGaraponForm seeds a single grand prize row', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    expect(s.garaponForm?.prizes).toHaveLength(1)
    expect(s.garaponForm?.prizes[0]!.isGrand).toBe(true)
  })

  it('editGaraponForm guarantees exactly one grand prize', () => {
    const s = useGaraponsStore()
    s.editGaraponForm(
      garapon({
        prizes: [
          { id: 1, name: 'A', ballColor: '#111', rate: 1, isGrand: false, sortOrder: 0 },
          { id: 2, name: 'B', ballColor: '#222', rate: 1, isGrand: false, sortOrder: 1 },
        ],
      }),
    )
    expect(s.garaponForm?.prizes.filter((p) => p.isGrand)).toHaveLength(1)
  })

  it('setGrandPrize is radio-style (single selection)', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.addPrizeRow()
    s.setGrandPrize(1)
    expect(s.garaponForm?.prizes.map((p) => p.isGrand)).toEqual([false, true])
  })

  it('removePrizeRow promotes the first row when the grand row is removed', () => {
    const s = useGaraponsStore()
    s.newGaraponForm() // [grand]
    s.addPrizeRow() // [grand, normal]
    s.removePrizeRow(0) // remove the grand row
    expect(s.garaponForm?.prizes).toHaveLength(1)
    expect(s.garaponForm?.prizes[0]!.isGrand).toBe(true)
  })

  it('removePrizeRow refuses to drop the last row', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.removePrizeRow(0)
    expect(s.garaponForm?.prizes).toHaveLength(1)
  })
})

describe('saveGarapon', () => {
  it('rejects a blank title without calling the API', async () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = '   '
    s.garaponForm!.prizes[0]!.name = 'Grand'
    expect(await s.saveGarapon()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('rejects when no prize has a name', async () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = 'Festival'
    expect(await s.saveGarapon()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates a new garapon with only the named prizes, then reloads', async () => {
    api.POST.mockResolvedValueOnce(ok(garapon(), 201))
    api.GET.mockResolvedValueOnce(ok({ garapons: [] }))
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = 'Festival'
    s.garaponForm!.prizes[0]!.name = 'Grand'
    s.addPrizeRow() // unnamed row should be filtered out

    expect(await s.saveGarapon()).toBe(true)
    const call = callTo('POST', '/garapons')
    expect(call).toBeDefined()
    const body = call![1].body as { prizes: Record<string, unknown>[]; id?: number }
    expect(body.prizes).toHaveLength(1) // unnamed row dropped
    expect(body.prizes[0]).not.toHaveProperty('_uid')
    expect(body).not.toHaveProperty('id')
    expect(callTo('GET', '/garapons')).toBeDefined() // reloaded
    expect(s.garaponForm).toBeNull()
  })

  it('updates when the form has an id', async () => {
    api.PUT.mockResolvedValueOnce(ok(garapon({ id: 7 })))
    api.GET.mockResolvedValueOnce(ok({ garapons: [] }))
    const s = useGaraponsStore()
    s.editGaraponForm(
      garapon({
        id: 7,
        prizes: [{ id: 1, name: 'Grand', ballColor: '#fff', rate: 1, isGrand: true, sortOrder: 0 }],
      }),
    )
    s.garaponForm!.title = 'Updated'
    expect(await s.saveGarapon()).toBe(true)
    expect(callTo('PUT', '/garapons/{id}')?.[1].params.path).toEqual({ id: 7 })
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('deleteGarapon', () => {
  it('does nothing when the user cancels the confirm', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useGaraponsStore()
    s.garapons = [garapon()]
    await s.deleteGarapon(1)
    expect(api.DELETE).not.toHaveBeenCalled()
    expect(s.garapons).toHaveLength(1)
  })

  it('deletes and drops the garapon from the list when confirmed', async () => {
    api.DELETE.mockResolvedValueOnce(ok(undefined, 204))
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useGaraponsStore()
    s.garapons = [garapon(), garapon({ id: 2 })]
    s.selectedGarapon = garapon()
    await s.deleteGarapon(1)
    expect(callTo('DELETE', '/garapons/{id}')?.[1].params.path).toEqual({ id: 1 })
    expect(s.garapons.map((g) => g.id)).toEqual([2])
    expect(s.selectedGarapon).toBeNull()
  })
})

describe('setGaraponStatus', () => {
  it('updates the selected garapon and the list row', async () => {
    api.POST.mockResolvedValueOnce(ok(garapon({ status: 'closed' })))
    const s = useGaraponsStore()
    s.garapons = [garapon()]
    s.selectedGarapon = garapon()
    await s.setGaraponStatus(1, 'closed')
    expect(callTo('POST', '/garapons/{id}/close')?.[1].params.path).toEqual({ id: 1 })
    expect(s.selectedGarapon.status).toBe('closed')
    expect(s.garapons[0]!.status).toBe('closed')
  })

  it('reopens through its own verb', async () => {
    api.POST.mockResolvedValueOnce(ok(garapon({ status: 'open' })))
    const s = useGaraponsStore()
    s.garapons = [garapon({ status: 'closed' })]
    await s.setGaraponStatus(1, 'open')
    expect(callTo('POST', '/garapons/{id}/reopen')).toBeDefined()
    expect(s.garapons[0]!.status).toBe('open')
  })
})

describe('createPlayer', () => {
  it('trims the name, clamps maxDraws to >=1, copies the link, and reloads', async () => {
    api.POST.mockResolvedValueOnce(ok({ id: 9, token: 'tok', playerName: 'Hero' }, 201))
    api.GET.mockResolvedValueOnce(ok({ garapon: garapon({ id: 3 }), players: [], draws: [] }))
    const s = useGaraponsStore()
    s.selectedGarapon = garapon({ id: 3 })
    s.playerAdd = { playerName: '  Hero  ', maxDraws: 0 }
    await s.createPlayer()
    const call = callTo('POST', '/garapons/{id}/players')
    expect(call?.[1].params.path).toEqual({ id: 3 })
    expect(call?.[1].body).toEqual({ playerName: 'Hero', maxDraws: 1 })
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    expect(callTo('GET', '/garapons/{id}')?.[1].params.path).toEqual({ id: 3 }) // reloaded
    expect(s.playerAdd.playerName).toBe('') // form reset
  })

  it('does not submit a blank name', async () => {
    const s = useGaraponsStore()
    s.selectedGarapon = garapon({ id: 3 })
    s.playerAdd = { playerName: '   ', maxDraws: 2 }
    await s.createPlayer()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('starts the draw count at the garapon default', () => {
    const s = useGaraponsStore()
    s.viewGarapon(garapon({ id: 3, defaultDraws: 4 }))
    expect(s.playerAdd.maxDraws).toBe(4)
  })
})

describe('public draw flow', () => {
  it('loadByToken populates the public view', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        garapon: publicGarapon({ title: 'Festival' }),
        player: summary(),
        draws: [],
        festivalLink: 'obon',
      }),
    )
    const s = useGaraponsStore()
    expect(await s.loadByToken('tok')).toBe(true)
    expect(s.publicGarapon?.title).toBe('Festival')
    expect(s.publicPlayer?.maxDraws).toBe(3)
    expect(s.publicFestivalLink).toBe('obon')
    expect(callTo('GET', '/garapon-players/{token}')?.[1].params.path).toEqual({ token: 'tok' })
  })

  it('loadByToken returns false on error', async () => {
    api.GET.mockResolvedValueOnce({
      error: { title: 'Not Found' },
      response: new Response(null, { status: 404 }),
    })
    const s = useGaraponsStore()
    expect(await s.loadByToken('bad')).toBe(false)
  })

  it('draw returns the server response and commitDraw applies it to visible state', async () => {
    api.POST.mockResolvedValueOnce(
      ok({
        draw: { prizeName: 'Grand', ballColor: '#fff', isGrand: true, drawnAt: null },
        drawsUsed: 1,
        maxDraws: 3,
      }),
    )
    const s = useGaraponsStore()
    s.publicPlayer = summary()
    s.publicDraws = []
    const resp = await s.draw('tok')
    expect(resp?.drawsUsed).toBe(1)
    s.commitDraw(resp!)
    expect(s.publicDraws).toHaveLength(1)
    expect(s.publicPlayer.drawsUsed).toBe(1)
    expect(s.lastWin?.prizeName).toBe('Grand')
  })

  it('a refused draw notifies and answers null', async () => {
    api.POST.mockResolvedValueOnce({
      error: { title: 'Conflict', detail: 'No draws remaining.' },
      response: new Response(null, { status: 409 }),
    })
    const s = useGaraponsStore()
    expect(await s.draw('tok')).toBeNull()
    expect(s.drawing).toBe(false)
  })
})

describe('linked stamp rally', () => {
  it('loadStampRallyOptions keeps only open rallies', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        stampRallies: [
          { id: 1, title: 'Open', status: 'open' },
          { id: 2, title: 'Closed', status: 'closed' },
        ],
      }),
    )
    const s = useGaraponsStore()
    await s.loadStampRallyOptions()
    expect(s.stampRallyOptions.map((r) => r.id)).toEqual([1])
  })

  it('stampCardLinkUrl uses the card token, empty when unlinked', () => {
    const s = useGaraponsStore()
    expect(s.stampCardLinkUrl({ stampCardToken: 'abc' } as GaraponPlayer)).toMatch(
      /\/stamp-card\/abc$/,
    )
    expect(s.stampCardLinkUrl({ stampCardToken: '' } as GaraponPlayer)).toBe('')
  })

  it('loadByToken surfaces the linked stamp-card token to the public view', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        garapon: publicGarapon({ title: 'Festival' }),
        player: summary({ stampCardToken: 'tok' }),
        draws: [],
        festivalLink: '',
      }),
    )
    const s = useGaraponsStore()
    await s.loadByToken('tok')
    expect(s.publicStampCardToken).toBe('tok')
  })

  it('publicStampCardToken is empty when the drawing link has no paired rally card', () => {
    const s = useGaraponsStore()
    s.publicPlayer = summary()
    expect(s.publicStampCardToken).toBe('')
  })
})

describe('copyGaraponForm', () => {
  /** A garapon with a prize table and a rally link - the link must not carry over. */
  function sourceGarapon(): Garapon {
    return garapon({
      id: 4,
      title: 'Obon Garapon',
      details: 'Spin the drum',
      grandPrizeImage: 'images/grand.png',
      stampRallyId: 11,
      defaultDraws: 3,
      status: 'closed',
      prizes: [
        { id: 1, name: 'Grand', ballColor: '#ffd700', rate: 5, isGrand: true, sortOrder: 0 },
        { id: 2, name: 'Minor', ballColor: '#88ccff', rate: 95, isGrand: false, sortOrder: 1 },
      ],
    })
  }

  it('carries the prize table and marks the title as a copy', () => {
    const store = useGaraponsStore()
    store.copyGaraponForm(sourceGarapon())
    const f = store.garaponForm!

    expect(f.title).toBe('Obon Garapon (Copy)')
    expect(f.details).toBe('Spin the drum')
    expect(f.grandPrizeImage).toBe('images/grand.png')
    expect(f.defaultDraws).toBe(3)
    expect(f.prizes.map((p) => [p.name, p.ballColor, p.rate, p.isGrand])).toEqual([
      ['Grand', '#ffd700', 5, true],
      ['Minor', '#88ccff', 95, false],
    ])
  })

  it('creates rather than overwrites, and drops the old rally link', () => {
    const store = useGaraponsStore()
    store.copyGaraponForm(sourceGarapon())
    const f = store.garaponForm!

    expect(f.id).toBe(0)
    // Carrying the link over would leave two garapons issuing draws off one
    // rally's sign-ups.
    expect(f.stampRallyId).toBeNull()
  })

  it('leaves the original untouched', () => {
    const store = useGaraponsStore()
    const source = sourceGarapon()
    store.copyGaraponForm(source)
    store.garaponForm!.title = 'Something else'
    store.garaponForm!.prizes[0]!.name = 'Changed'

    expect(source.title).toBe('Obon Garapon')
    expect(source.prizes[0]!.name).toBe('Grand')
    expect(source.stampRallyId).toBe(11)
  })
})
