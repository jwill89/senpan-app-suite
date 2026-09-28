import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Spy the game endpoints the store touches; the store imports the endpoint +
// sound layers at setup, so stub both (no other path here touches them).
const { halftime, setDelay, setAutoEnabled, setAutoInterval, winnersList, winnersDelete } =
  vi.hoisted(() => ({
    halftime: vi.fn(async () => ({ ok: true })),
    setDelay: vi.fn(async () => ({ ok: true })),
    setAutoEnabled: vi.fn(async () => ({ ok: true })),
    setAutoInterval: vi.fn(async () => ({ ok: true })),
    // Typed with their real parameters: a zero-arg vi.fn() makes every
    // mockImplementation that reads page/id a type error, even though it runs fine.
    winnersList: vi.fn(async (_params: { page: number }) => ({
      entries: [] as { id: number }[],
      total: 0,
    })),
    winnersDelete: vi.fn(async (_id: number) => ({ ok: true })),
  }))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    game: { halftime, setDelay, setAutoEnabled, setAutoInterval },
    winnersLog: { list: winnersList, delete: winnersDelete },
  },
}))
vi.mock('@/lib/sound', () => ({ playWinnerChime: vi.fn() }))

import { useGameStore } from './game'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
  halftime.mockClear()
  setDelay.mockClear()
  setAutoEnabled.mockClear()
  setAutoInterval.mockClear()
  // mockReset, not mockClear: an implementation set inside one test would
  // otherwise persist and silently change what the next one is measuring.
  winnersList.mockReset()
  winnersList.mockResolvedValue({ entries: [], total: 0 })
  winnersDelete.mockReset()
  winnersDelete.mockResolvedValue({ ok: true })
})

describe('loadWinnersLog', () => {
  /**
   * Every winners-log DELETE echoes back over the WebSocket as a resource_changed
   * and admin.ts calls loadWinnersLog on each one - so clearing a handful of
   * entries started that many full re-pages of an unbounded log at once, all
   * racing to write the same ref. Overlapping calls now share one walk.
   */
  it('coalesces overlapping loads into a single pass', async () => {
    const game = useGameStore()
    await Promise.all([game.loadWinnersLog(), game.loadWinnersLog(), game.loadWinnersLog()])
    expect(winnersList).toHaveBeenCalledTimes(1)
  })

  it('fetches page 1 first, then every remaining page at once', async () => {
    // 3 pages of 200. Walking them serially made a venue with a couple of years of
    // history wait on that many round trips IN SEQUENCE before a row painted, so
    // the assertion has to be about ORDERING, not just the call count - a serial
    // loop issues exactly the same three requests.
    const order: string[] = []
    winnersList.mockImplementation(async ({ page }: { page: number }) => {
      order.push(`start${page}`)
      await Promise.resolve()
      order.push(`end${page}`)
      return {
        entries: Array.from({ length: page === 3 ? 50 : 200 }, (_, i) => ({ id: page * 1000 + i })),
        total: 450,
      }
    })

    const game = useGameStore()
    await game.loadWinnersLog()

    expect(winnersList).toHaveBeenCalledTimes(3)
    expect(game.winnersLog).toHaveLength(450)
    expect(game.winnersLogTotal).toBe(450)

    // Page 1 goes alone - only it can report the total. Pages 2 and 3 then both
    // start before either finishes; a serial loop would read start2,end2,start3.
    expect(order[0]).toBe('start1')
    expect(order[1]).toBe('end1')
    expect(order.slice(2, 4).sort()).toEqual(['start2', 'start3'])
  })

  it('deletes a bulk selection concurrently rather than one at a time', async () => {
    const game = useGameStore()
    useUiStore().confirm = vi.fn(async () => true)
    const order: string[] = []
    winnersDelete.mockImplementation(async (id: number) => {
      order.push(`start${id}`)
      await Promise.resolve()
      order.push(`end${id}`)
      return { ok: true }
    })

    await game.deleteWinnerLogEntries([1, 2, 3])

    expect(winnersDelete).toHaveBeenCalledTimes(3)
    // All three start before any finishes - a serial loop would interleave
    // start/end pairs instead.
    expect(order.slice(0, 3)).toEqual(['start1', 'start2', 'start3'])
  })

  it('does not latch: a later load runs again', async () => {
    const game = useGameStore()
    await game.loadWinnersLog()
    await game.loadWinnersLog()
    // Two sequential calls are two real loads - the guard must not remember a
    // settled promise, which would make every later refresh a silent no-op.
    expect(winnersList).toHaveBeenCalledTimes(2)
  })
})

describe('halftime prompt - server-driven mini-game choice', () => {
  it('confirmHalftime answers "mini-game" (true) and closes the prompt', async () => {
    const game = useGameStore()
    game.showHalftimePrompt = true
    game.halftimeAutoPaused = true
    await game.confirmHalftime()
    expect(halftime).toHaveBeenCalledWith(true)
    expect(game.showHalftimePrompt).toBe(false)
    expect(game.halftimeAutoPaused).toBe(false)
  })

  it('dismissHalftime answers "no mini-game" (false) so the server can resume auto', async () => {
    const game = useGameStore()
    game.showHalftimePrompt = true
    await game.dismissHalftime()
    expect(halftime).toHaveBeenCalledWith(false)
    expect(game.showHalftimePrompt).toBe(false)
  })
})

describe('auto-draw live controls (optimistic)', () => {
  it('setAutoEnabled flips currentGame.auto_enabled and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = { auto_enabled: false, auto_interval: 30 } as never
    await game.setAutoEnabled(true)
    expect(setAutoEnabled).toHaveBeenCalledWith(true)
    expect((game.currentGame as unknown as { auto_enabled: boolean }).auto_enabled).toBe(true)
  })

  it('setAutoInterval updates currentGame.auto_interval and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = { auto_enabled: true, auto_interval: 30 } as never
    await game.setAutoInterval(45)
    expect(setAutoInterval).toHaveBeenCalledWith(45)
    expect((game.currentGame as unknown as { auto_interval: number }).auto_interval).toBe(45)
  })
})

describe('persistDrawDelay - shared delay control', () => {
  it('sends the current drawDelay to the server', async () => {
    const game = useGameStore()
    game.drawDelay = 15
    await game.persistDrawDelay()
    expect(setDelay).toHaveBeenCalledWith(15)
  })
})
