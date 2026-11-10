import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

const { playWinnerChime } = vi.hoisted(() => ({ playWinnerChime: vi.fn() }))
vi.mock('@/lib/sound', () => ({ playWinnerChime }))

// Everything goes through the API client; only its calls are faked (unwrap is the
// real one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const refused = (status: number, detail: string) => ({
  error: { title: 'Conflict', status, detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import type { Game, GameState } from '@/api/client'
import { useGameStore } from './game'
import { useUiStore } from './ui'

/** A page of the winners log, as GET /winners-log answers it. */
const winnersPage = (entries: { id: number }[], total: number) =>
  ok({ entries, total, page: 1, perPage: 200 })

/** A game running, as GET /game answers it to staff. */
function liveGame(id: number, changes: Partial<Game> = {}): Game {
  return {
    id,
    createdAt: '2026-11-09T20:00:00Z',
    calledNumbers: [],
    patterns: [],
    totalCalled: 0,
    yoeverEnabled: true,
    yoeverCount: 0,
    autoEnabled: false,
    autoInterval: 30,
    halftime: null,
    minigameAlerted: false,
    lastDraw: null,
    ...changes,
  }
}

function state(game: Game | null, changes: Partial<GameState> = {}): GameState {
  return { game, winners: [], gameDetails: '', drawDelay: 0, ...changes }
}

beforeEach(() => {
  setActivePinia(createPinia())
  playWinnerChime.mockClear()
  // mockReset, not mockClear: an implementation set inside one test would
  // otherwise persist and silently change what the next one is measuring.
  api.GET.mockReset()
  api.GET.mockResolvedValue(winnersPage([], 0))
  api.POST.mockReset()
  api.POST.mockResolvedValue(ok(state(null)))
  api.PATCH.mockReset()
  api.PATCH.mockResolvedValue(ok(state(null)))
  api.DELETE.mockReset()
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

describe('loadWinnersLog', () => {
  /**
   * Every winners-log DELETE echoes back on the live stream as a resource_changed
   * and admin.ts calls loadWinnersLog on each one - so clearing a handful of
   * entries started that many full re-pages of an unbounded log at once, all
   * racing to write the same ref. Overlapping calls now share one walk.
   */
  it('coalesces overlapping loads into a single pass', async () => {
    const game = useGameStore()
    await Promise.all([game.loadWinnersLog(), game.loadWinnersLog(), game.loadWinnersLog()])
    expect(api.GET).toHaveBeenCalledTimes(1)
    expect(api.GET).toHaveBeenCalledWith('/winners-log', {
      params: { query: { page: 1, perPage: 200, sort: 'loggedAt', dir: 'desc' } },
    })
  })

  it('fetches page 1 first, then every remaining page at once', async () => {
    // 3 pages of 200. Walking them serially made a venue with a couple of years of
    // history wait on that many round trips IN SEQUENCE before a row painted, so
    // the assertion has to be about ORDERING, not just the call count - a serial
    // loop issues exactly the same three requests.
    const order: string[] = []
    api.GET.mockImplementation(
      async (_path: string, init: { params: { query: { page: number } } }) => {
        const page = init.params.query.page
        order.push(`start${page}`)
        await Promise.resolve()
        order.push(`end${page}`)
        return winnersPage(
          Array.from({ length: page === 3 ? 50 : 200 }, (_, i) => ({ id: page * 1000 + i })),
          450,
        )
      },
    )

    const game = useGameStore()
    await game.loadWinnersLog()

    expect(api.GET).toHaveBeenCalledTimes(3)
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
    api.DELETE.mockImplementation(
      async (_path: string, init: { params: { path: { id: number } } }) => {
        const id = init.params.path.id
        order.push(`start${id}`)
        await Promise.resolve()
        order.push(`end${id}`)
        return ok(undefined, 204)
      },
    )

    await game.deleteWinnerLogEntries([1, 2, 3])

    expect(api.DELETE).toHaveBeenCalledTimes(3)
    expect(api.DELETE).toHaveBeenCalledWith('/winners-log/{id}', { params: { path: { id: 2 } } })
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
    expect(api.GET).toHaveBeenCalledTimes(2)
  })

  it('deleteAllWinnersLog clears the log after the confirm, then reloads from page 1', async () => {
    const game = useGameStore()
    useUiStore().confirm = vi.fn(async () => true)
    game.winnersLogPage = 3
    api.DELETE.mockResolvedValueOnce(ok({ deleted: 4 }))
    await game.deleteAllWinnersLog()
    expect(api.DELETE).toHaveBeenCalledWith('/winners-log/all')
    expect(game.winnersLogPage).toBe(1)
    expect(api.GET).toHaveBeenCalledTimes(1)
  })
})

describe('loadFrequentWinners', () => {
  it('fills the frequent winners from the API', async () => {
    api.GET.mockResolvedValueOnce(ok({ winners: [{ playerName: 'Alice', winCount: 3 }] }))
    const game = useGameStore()
    await game.loadFrequentWinners()
    expect(api.GET).toHaveBeenCalledWith('/winners-log/frequent')
    expect(game.frequentWinners).toEqual([{ playerName: 'Alice', winCount: 3 }])
  })
})

describe('halftime prompt - server-driven mini-game choice', () => {
  it('confirmHalftime answers "mini-game" for the prompt\'s game and closes the prompt', async () => {
    const game = useGameStore()
    game.takeHalftimePrompt(4, true)
    await game.confirmHalftime()
    expect(api.POST).toHaveBeenCalledWith('/game/halftime', { body: { gameId: 4, minigame: true } })
    expect(game.showHalftimePrompt).toBe(false)
    expect(game.halftimeAutoPaused).toBe(false)
  })

  it('dismissHalftime answers "no mini-game" so the server can resume auto', async () => {
    const game = useGameStore()
    game.takeHalftimePrompt(4, false)
    await game.dismissHalftime()
    expect(api.POST).toHaveBeenCalledWith('/game/halftime', {
      body: { gameId: 4, minigame: false },
    })
    expect(game.showHalftimePrompt).toBe(false)
  })

  it('says so, not an error, when another console answered first', async () => {
    const game = useGameStore()
    const ui = useUiStore()
    ui.notify = vi.fn()
    api.POST.mockResolvedValueOnce(refused(409, 'The half-time prompt was already answered.'))
    game.takeHalftimePrompt(4, false)
    await game.dismissHalftime()
    expect(ui.notify).toHaveBeenCalledWith('The half-time prompt was already answered.', 'info')
  })

  it('a console opening mid-way shows the prompt still waiting', () => {
    const game = useGameStore()
    game.applyState(state(liveGame(4, { halftime: { gameId: 4, autoPaused: true } })))
    expect(game.showHalftimePrompt).toBe(true)
    expect(game.halftimeAutoPaused).toBe(true)
    expect(game.halftimeGameId).toBe(4)
    game.applyState(state(liveGame(4)))
    expect(game.showHalftimePrompt).toBe(false)
  })
})

describe('the game state for a console opening mid-way', () => {
  it('takes the draw delay, the last draw, and counts down to when players get it', () => {
    vi.useFakeTimers()
    try {
      const game = useGameStore()
      const playersAt = new Date(Date.now() + 10_000).toISOString()
      game.applyState(
        state(
          liveGame(4, {
            calledNumbers: [7],
            totalCalled: 1,
            lastDraw: { number: 7, letter: 'B', callOrder: 1, playersAt },
          }),
          { drawDelay: 10, winners: ['WIN001'], gameDetails: 'Round 2' },
        ),
      )
      expect(game.drawDelay).toBe(10)
      expect(game.lastDrawn).toEqual({ number: 7, letter: 'B', callOrder: 1 })
      expect(game.drawCountdown).toBe(10)
      expect(game.winners).toEqual(['WIN001'])
      expect(game.gameDetails).toBe('Round 2')
      // A load isn't news: no chime for winners that were already there.
      expect(playWinnerChime).not.toHaveBeenCalled()
      vi.advanceTimersByTime(10_000)
      expect(game.drawCountdown).toBeNull()
      expect(game.drawSent).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('starting a game', () => {
  it('asks to replace a game already running, and replaces it on a yes', async () => {
    const game = useGameStore()
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    game.selectedPatternIds = [1]
    api.POST.mockResolvedValueOnce(refused(409, 'A game is already running.'))
    api.POST.mockResolvedValueOnce(ok(state(liveGame(5))))
    await game.startGame()
    expect(ui.confirm).toHaveBeenCalled()
    expect(api.POST).toHaveBeenLastCalledWith('/game/start', {
      body: { patternIds: [1], auto: false, autoInterval: 30, replace: true },
    })
    expect(game.currentGame?.id).toBe(5)
  })

  it('catches up on the running game on a no', async () => {
    const game = useGameStore()
    useUiStore().confirm = vi.fn(async () => false)
    game.selectedPatternIds = [1]
    api.POST.mockResolvedValueOnce(refused(409, 'A game is already running.'))
    api.GET.mockImplementation(async (path: string) =>
      path === '/game' ? ok(state(liveGame(3))) : winnersPage([], 0),
    )
    await game.startGame()
    expect(api.POST).toHaveBeenCalledTimes(1)
    expect(game.currentGame?.id).toBe(3)
  })
})

describe('ending a game', () => {
  it('names the game the confirm was opened on', async () => {
    const game = useGameStore()
    game.currentGame = liveGame(4)
    game.winners = ['WIN001']
    await game.endGame()
    expect(game.showEndGameModal).toBe(true)
    api.POST.mockResolvedValueOnce(ok({ ended: true, logged: 1 }))
    await game.confirmEndGame(['WIN001'])
    expect(api.POST).toHaveBeenCalledWith('/game/end', {
      body: { gameId: 4, validWinnerIds: ['WIN001'] },
    })
    expect(game.currentGame).toBeNull()
  })

  it('says the game had already ended rather than "Game ended"', async () => {
    const game = useGameStore()
    const ui = useUiStore()
    ui.notify = vi.fn()
    game.currentGame = liveGame(4)
    api.POST.mockResolvedValueOnce(ok({ ended: false, logged: 0 }))
    await game.endGame()
    expect(ui.notify).toHaveBeenCalledWith('The game had already ended.', 'info')
    expect(ui.notify).not.toHaveBeenCalledWith('Game ended', 'info')
    expect(game.currentGame).toBeNull()
  })

  it('catches up when a new game has started since the confirm opened', async () => {
    const game = useGameStore()
    const ui = useUiStore()
    ui.notify = vi.fn()
    game.currentGame = liveGame(4)
    api.POST.mockResolvedValueOnce(refused(409, 'That game has already ended.'))
    api.GET.mockImplementation(async (path: string) =>
      path === '/game' ? ok(state(liveGame(5))) : winnersPage([], 0),
    )
    await game.endGame()
    expect(ui.notify).toHaveBeenCalledWith('That game has already ended.', 'error')
    await vi.waitFor(() => expect(game.currentGame?.id).toBe(5))
  })
})

describe('the winner chime', () => {
  it('sounds when the winners grow, on any console, if asked for', () => {
    const game = useGameStore()
    game.currentGame = liveGame(4)
    game.setWinnerSoundEnabled(true)
    game.takeDraw(4, { number: 7, letter: 'B', callOrder: 1 }, ['WIN001'])
    expect(playWinnerChime).toHaveBeenCalledTimes(1)
    game.takeDraw(4, { number: 8, letter: 'B', callOrder: 2 }, ['WIN001'])
    expect(playWinnerChime).toHaveBeenCalledTimes(1)
    game.setWinnerSoundEnabled(false)
    game.takeDraw(4, { number: 9, letter: 'B', callOrder: 3 }, ['WIN001', 'WIN002'])
    expect(playWinnerChime).toHaveBeenCalledTimes(1)
  })
})

describe('auto-draw live controls (optimistic)', () => {
  it('setAutoEnabled flips currentGame.autoEnabled and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = liveGame(4)
    await game.setAutoEnabled(true)
    expect(api.PATCH).toHaveBeenCalledWith('/game', { body: { autoEnabled: true } })
    expect(game.currentGame.autoEnabled).toBe(true)
  })

  it('setAutoInterval updates currentGame.autoInterval and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = liveGame(4, { autoEnabled: true })
    await game.setAutoInterval(45)
    expect(api.PATCH).toHaveBeenCalledWith('/game', { body: { autoInterval: 45 } })
    expect(game.currentGame.autoInterval).toBe(45)
  })

  it('reverts when the server refuses', async () => {
    const game = useGameStore()
    useUiStore().notify = vi.fn()
    game.currentGame = liveGame(4)
    api.PATCH.mockResolvedValueOnce(refused(403, 'No.'))
    await game.setAutoEnabled(true)
    expect(game.currentGame.autoEnabled).toBe(false)
  })
})

describe('persistDrawDelay - shared delay control', () => {
  it('sends the current drawDelay to the server', async () => {
    const game = useGameStore()
    game.drawDelay = 15
    await game.persistDrawDelay()
    expect(api.PATCH).toHaveBeenCalledWith('/game', { body: { delay: 15 } })
  })
})
