import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Garapon, GaraponDraw, GaraponPlayer, PublicGarapon } from '@/types/api'

// Mock the typed endpoint layer so store actions can be exercised without the
// network. vi.hoisted lets the spies be referenced inside the mock factory.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ garapons: [] as Garapon[] })),
  detail: vi.fn(async () => ({
    garapon: {} as Garapon,
    players: [] as GaraponPlayer[],
    draws: [] as GaraponDraw[],
  })),
  create: vi.fn(async () => ({ ok: true })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => ({ ok: true })),
  setStatus: vi.fn(async () => ({ ok: true })),
  createPlayer: vi.fn(async () => ({ player: { id: 9, token: 'tok', player_name: 'Hero' } })),
  publicGet: vi.fn(async () => ({ garapon: {} as Garapon, player: {}, draws: [] })),
  draw: vi.fn(async () => ({ draw: { prize_name: 'Grand' }, draws_used: 1, max_draws: 3 })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    garapons: {
      list: ep.list,
      detail: ep.detail,
      create: ep.create,
      update: ep.update,
      delete: ep.del,
      setStatus: ep.setStatus,
      createPlayer: ep.createPlayer,
    },
    garapon: { get: ep.publicGet, draw: ep.draw },
    stampRallies: {
      list: vi.fn(async () => ({
        stamp_rallies: [
          { id: 1, title: 'Open', status: 'open' },
          { id: 2, title: 'Closed', status: 'closed' },
        ],
      })),
    },
  },
}))

import { useGaraponsStore } from './garapons'
import { useUiStore } from './ui'

/** Minimal garapon with the fields the store reads. */
function garapon(over: Partial<Garapon> = {}): Garapon {
  return { id: 1, title: 'G', status: 'open', prizes: [] as never, ...over } as Garapon
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
  // jsdom has no clipboard by default; provide a resolving stub.
  Object.assign(navigator, { clipboard: { writeText: vi.fn(async () => {}) } })
})

describe('loading', () => {
  it('loadGarapons populates the list', async () => {
    ep.list.mockResolvedValueOnce({ garapons: [garapon(), garapon({ id: 2 })] })
    const s = useGaraponsStore()
    await s.loadGarapons()
    expect(s.garapons).toHaveLength(2)
    expect(s.garaponsLoading).toBe(false)
  })

  it('loadGaraponDetail populates selected garapon, players, and draws', async () => {
    ep.detail.mockResolvedValueOnce({
      garapon: garapon({ id: 5 }),
      players: [{ id: 1 } as GaraponPlayer],
      draws: [{ id: 1 }, { id: 2 }] as GaraponDraw[],
    })
    const s = useGaraponsStore()
    await s.loadGaraponDetail(5)
    expect(s.selectedGarapon?.id).toBe(5)
    expect(s.garaponPlayers).toHaveLength(1)
    expect(s.garaponDraws).toHaveLength(2)
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
    s.publicGarapon = garapon({
      prizes: [
        { name: 'Grand', is_grand: true },
        { name: 'Other', is_grand: false },
      ] as never,
    }) as unknown as PublicGarapon
    s.publicPlayer = { player_name: 'Hero', max_draws: 3, draws_used: 1 }
    expect(s.grandPrize?.name).toBe('Grand')
    expect(s.otherPrizes).toHaveLength(1)
    expect(s.drawsRemaining).toBe(2)
    expect(s.canDraw).toBe(true)
  })

  it('canDraw is false when the allowance is exhausted', () => {
    const s = useGaraponsStore()
    s.publicGarapon = garapon() as unknown as PublicGarapon
    s.publicPlayer = { player_name: 'Hero', max_draws: 1, draws_used: 1 }
    expect(s.drawsRemaining).toBe(0)
    expect(s.canDraw).toBe(false)
  })
})

describe('form editing', () => {
  it('newGaraponForm seeds a single grand prize row', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    expect(s.garaponForm?.prizes).toHaveLength(1)
    expect(s.garaponForm?.prizes[0].is_grand).toBe(true)
  })

  it('editGaraponForm guarantees exactly one grand prize', () => {
    const s = useGaraponsStore()
    s.editGaraponForm(
      garapon({
        prizes: [
          { name: 'A', is_grand: false },
          { name: 'B', is_grand: false },
        ] as never,
      }),
    )
    expect(s.garaponForm?.prizes.filter((p) => p.is_grand)).toHaveLength(1)
  })

  it('setGrandPrize is radio-style (single selection)', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.addPrizeRow()
    s.setGrandPrize(1)
    expect(s.garaponForm?.prizes.map((p) => p.is_grand)).toEqual([false, true])
  })

  it('removePrizeRow promotes the first row when the grand row is removed', () => {
    const s = useGaraponsStore()
    s.newGaraponForm() // [grand]
    s.addPrizeRow() // [grand, normal]
    s.removePrizeRow(0) // remove the grand row
    expect(s.garaponForm?.prizes).toHaveLength(1)
    expect(s.garaponForm?.prizes[0].is_grand).toBe(true)
  })

  it('removePrizeRow refuses to drop the last row', () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.removePrizeRow(0)
    expect(s.garaponForm?.prizes).toHaveLength(1)
  })
})

describe('saveGarapon', () => {
  it('rejects a blank title without calling the endpoint', async () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = '   '
    s.garaponForm!.prizes[0].name = 'Grand'
    expect(await s.saveGarapon()).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('rejects when no prize has a name', async () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = 'Festival'
    expect(await s.saveGarapon()).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates a new garapon with only the named prizes, then reloads', async () => {
    const s = useGaraponsStore()
    s.newGaraponForm()
    s.garaponForm!.title = 'Festival'
    s.garaponForm!.prizes[0].name = 'Grand'
    s.addPrizeRow() // unnamed row should be filtered out

    expect(await s.saveGarapon()).toBe(true)
    expect(ep.create).toHaveBeenCalledTimes(1)
    const payload = (ep.create.mock.calls[0] as unknown[])[0] as { prizes: unknown[] }
    expect(payload.prizes).toHaveLength(1) // unnamed row dropped
    expect(ep.list).toHaveBeenCalled() // reloaded
    expect(s.garaponForm).toBeNull()
  })

  it('updates when the form has an id', async () => {
    const s = useGaraponsStore()
    s.editGaraponForm(garapon({ id: 7, prizes: [{ name: 'Grand', is_grand: true }] as never }))
    s.garaponForm!.title = 'Updated'
    expect(await s.saveGarapon()).toBe(true)
    expect(ep.update).toHaveBeenCalledTimes(1)
    expect(ep.create).not.toHaveBeenCalled()
  })
})

describe('deleteGarapon', () => {
  it('does nothing when the user cancels the confirm', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useGaraponsStore()
    s.garapons = [garapon()]
    await s.deleteGarapon(1)
    expect(ep.del).not.toHaveBeenCalled()
    expect(s.garapons).toHaveLength(1)
  })

  it('deletes and drops the garapon from the list when confirmed', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useGaraponsStore()
    s.garapons = [garapon(), garapon({ id: 2 })]
    s.selectedGarapon = garapon()
    await s.deleteGarapon(1)
    expect(ep.del).toHaveBeenCalledWith(1)
    expect(s.garapons.map((g) => g.id)).toEqual([2])
    expect(s.selectedGarapon).toBeNull()
  })
})

describe('setGaraponStatus', () => {
  it('updates the selected garapon and the list row', async () => {
    const s = useGaraponsStore()
    s.garapons = [garapon()]
    s.selectedGarapon = garapon()
    await s.setGaraponStatus(1, 'closed')
    expect(ep.setStatus).toHaveBeenCalledWith(1, 'closed')
    expect(s.selectedGarapon.status).toBe('closed')
    expect(s.garapons[0].status).toBe('closed')
  })
})

describe('createPlayer', () => {
  it('trims the name, clamps maxDraws to >=1, copies the link, and reloads', async () => {
    const s = useGaraponsStore()
    s.selectedGarapon = garapon({ id: 3 })
    s.playerAdd = { playerName: '  Hero  ', maxDraws: 0 }
    await s.createPlayer()
    expect(ep.createPlayer).toHaveBeenCalledWith(3, { player_name: 'Hero', max_draws: 1 })
    expect(navigator.clipboard.writeText).toHaveBeenCalled()
    expect(ep.detail).toHaveBeenCalledWith(3) // reloaded
    expect(s.playerAdd.playerName).toBe('') // form reset
  })

  it('does not submit a blank name', async () => {
    const s = useGaraponsStore()
    s.selectedGarapon = garapon({ id: 3 })
    s.playerAdd = { playerName: '   ', maxDraws: 2 }
    await s.createPlayer()
    expect(ep.createPlayer).not.toHaveBeenCalled()
  })
})

describe('public draw flow', () => {
  it('loadByToken populates the public view', async () => {
    ep.publicGet.mockResolvedValueOnce({
      garapon: garapon({ title: 'Festival' }),
      player: { player_name: 'Hero', max_draws: 3, draws_used: 0 },
      draws: [],
    })
    const s = useGaraponsStore()
    expect(await s.loadByToken('tok')).toBe(true)
    expect(s.publicGarapon?.title).toBe('Festival')
    expect(s.publicPlayer?.max_draws).toBe(3)
  })

  it('loadByToken returns false on error', async () => {
    ep.publicGet.mockRejectedValueOnce(new Error('404'))
    const s = useGaraponsStore()
    expect(await s.loadByToken('bad')).toBe(false)
  })

  it('draw returns the server response and commitDraw applies it to visible state', async () => {
    const s = useGaraponsStore()
    s.publicPlayer = { player_name: 'Hero', max_draws: 3, draws_used: 0 }
    s.publicDraws = []
    const resp = await s.draw('tok')
    expect(resp?.draws_used).toBe(1)
    s.commitDraw(resp!)
    expect(s.publicDraws).toHaveLength(1)
    expect(s.publicPlayer.draws_used).toBe(1)
    expect(s.lastWin?.prize_name).toBe('Grand')
  })
})

describe('linked stamp rally', () => {
  it('loadStampRallyOptions keeps only open rallies', async () => {
    const s = useGaraponsStore()
    await s.loadStampRallyOptions()
    expect(s.stampRallyOptions.map((r) => r.id)).toEqual([1])
  })

  it('stampCardLinkUrl uses the shared token, empty when unlinked', () => {
    const s = useGaraponsStore()
    expect(s.stampCardLinkUrl({ stamp_card_token: 'abc' } as GaraponPlayer)).toMatch(
      /\/stamp-card\/abc$/,
    )
    expect(s.stampCardLinkUrl({ stamp_card_token: '' } as GaraponPlayer)).toBe('')
  })

  it('loadByToken surfaces the linked stamp-card token to the public view', async () => {
    ep.publicGet.mockResolvedValueOnce({
      garapon: garapon({ title: 'Festival' }),
      player: { player_name: 'Hero', max_draws: 3, draws_used: 0, stamp_card_token: 'tok' },
      draws: [],
    })
    const s = useGaraponsStore()
    await s.loadByToken('tok')
    expect(s.publicStampCardToken).toBe('tok')
  })

  it('publicStampCardToken is empty when the drawing link has no paired rally card', () => {
    const s = useGaraponsStore()
    s.publicPlayer = { player_name: 'Hero', max_draws: 3, draws_used: 0 }
    expect(s.publicStampCardToken).toBe('')
  })
})
