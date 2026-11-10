import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { FeedOptions } from '@/lib/feed'

// Capture each stream the composable opens: its options (URL, handlers, the
// re-read on open) and whether it is running. vi.hoisted makes these available
// inside the mock factories (which are hoisted above imports).
const h = vi.hoisted(() => ({
  feeds: [] as { opts: FeedOptions<{ type: string }>; running: boolean }[],
  push: vi.fn(),
}))

vi.mock('@/lib/feed', () => ({
  Feed: class {
    entry: { opts: FeedOptions<{ type: string }>; running: boolean }
    constructor(opts: FeedOptions<{ type: string }>) {
      this.entry = { opts, running: false }
      h.feeds.push(this.entry)
    }
    start() {
      this.entry.running = true
    }
    stop() {
      this.entry.running = false
    }
  },
}))
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  useRouter: () => ({ push: h.push }),
}))
const sound = vi.hoisted(() => ({
  playEvent: vi.fn(),
  vibrate: vi.fn(),
  playWinnerChime: vi.fn(),
  playUnlockChime: vi.fn(),
  playYoeverSound: vi.fn(),
}))
vi.mock('@/lib/sound', async (importOriginal) => ({
  ...(await importOriginal<Record<string, unknown>>()),
  ...sound,
}))
vi.mock('@/lib/theme', () => ({
  applyCustomCSS: vi.fn(),
  applyHeaderFont: vi.fn(),
  applyUploadedFonts: vi.fn(),
  applyNumberFlourish: vi.fn(),
}))

import { useLiveEvents } from './useLiveEvents'
import { useAdminStore } from '@/stores/admin'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { useCardsStore } from '@/stores/cards'
import { useGameStore } from '@/stores/game'
import { useLogsStore } from '@/stores/logs'
import { usePatternsStore } from '@/stores/patterns'
import { usePlayerStore } from '@/stores/player'
import { useUiStore } from '@/stores/ui'
import { useYoeverStore } from '@/stores/yoever'
import type { Account, Card, Game } from '@/api/client'

/** A game running, as a console or a board holds it. */
function liveGame(id: number): Game {
  return {
    id,
    createdAt: null,
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
  }
}

/** An event as the stream carries it: the envelope around a payload. */
function event(type: string, payload: Record<string, unknown> = {}, gameId: number | null = null) {
  return { id: 1, type, gameId, at: '2026-11-09T20:00:00Z', payload }
}

/** The stream open now. */
function open() {
  const f = [...h.feeds].reverse().find((f) => f.running)
  if (!f) {
    throw new Error('no stream is open')
  }
  return f
}

/** Sends an event through the open stream's handler for it. */
function send(e: { type: string; [field: string]: unknown }) {
  const handler = (open().opts.handlers as Record<string, (e: unknown) => void>)[e.type]
  if (!handler) {
    throw new Error(`the stream has no handler for ${e.type}`)
  }
  handler(e)
}

function signIn(account: Partial<Account>) {
  const auth = useAuthStore()
  auth.user = { id: 1, username: 'staff', isAdmin: false, permissions: [], ...account }
  auth.isAdmin = auth.user.isAdmin
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  h.feeds.length = 0
  h.push.mockClear()
  for (const fn of Object.values(sound)) {
    fn.mockClear()
  }
})

describe('the streams it keeps', () => {
  it("opens a board's stream for a board, the staff's for the admin area, one at a time", () => {
    const live = useLiveEvents()
    live.follow({ kind: 'board', cardId: 'AB CD' })
    expect(open().opts.url).toBe('/api/v1/board/AB%20CD/events')
    live.follow({ kind: 'staff' })
    expect(h.feeds.filter((f) => f.running)).toHaveLength(1)
    expect(open().opts.url).toBe('/api/v1/events')
    live.follow(null)
    expect(h.feeds.some((f) => f.running)).toBe(false)
  })

  it('leaves the open stream alone when told to follow it again', () => {
    const live = useLiveEvents()
    live.follow({ kind: 'staff' })
    live.follow({ kind: 'staff' })
    expect(h.feeds).toHaveLength(1)
    live.stop()
    expect(h.feeds[0]!.running).toBe(false)
  })

  it('has a handler for every event of each stream (the types enforce it)', () => {
    const live = useLiveEvents()
    live.follow({ kind: 'staff' })
    expect(Object.keys(open().opts.handlers)).toContain('tea_room_unlocked')
    live.follow({ kind: 'board', cardId: 'CARD01' })
    expect(Object.keys(open().opts.handlers)).toContain('card_deleted')
  })
})

describe('the board', () => {
  function onBoard(): ReturnType<typeof usePlayerStore> {
    const player = usePlayerStore()
    player.playerCard = { id: 'CARD01' } as Card
    player.playerGame = liveGame(4)
    player.refreshBoard = vi.fn(async () => 'details')
    useLiveEvents().follow({ kind: 'board', cardId: 'CARD01' })
    return player
  }

  it('appends a draw for its game once, with its sound, and drops one for another game', () => {
    const player = onBoard()
    player.soundMode = 'basic'
    send(event('draw', { drawn: { number: 9, letter: 'B', callOrder: 1 } }, 3))
    send(event('draw', { drawn: { number: 7, letter: 'B', callOrder: 1 } }, 4))
    send(event('draw', { drawn: { number: 7, letter: 'B', callOrder: 1 } }, 4))
    expect(player.playerGame?.calledNumbers).toEqual([7])
    expect(player.playerGame?.totalCalled).toBe(1)
    expect(player.lastDrawn?.number).toBe(7)
    expect(sound.playEvent).toHaveBeenCalledTimes(1)
  })

  it('shows the mini-game alert for its game once', () => {
    const player = onBoard()
    player.soundMode = 'basic'
    send(event('minigame', {}, 3))
    expect(player.showMinigameModal).toBe(false)
    send(event('minigame', {}, 4))
    expect(player.showMinigameModal).toBe(true)
    expect(sound.playEvent).toHaveBeenCalledWith('minigame', 'basic')
    player.showMinigameModal = false
    send(event('minigame', {}, 4))
    expect(player.showMinigameModal).toBe(false)
  })

  it('says thank you when its game ends, closes the mini-game, and reads the board again', () => {
    const player = onBoard()
    player.playerGame!.calledNumbers.push(1, 2, 3)
    player.showMinigameModal = true
    send(event('game_ended', {}, 4))
    expect(player.gameEnded).toBe(true)
    expect(player.endedCalledCount).toBe(3)
    expect(player.showMinigameModal).toBe(false)
    expect(player.playerGame).toBeNull()
    expect(player.refreshBoard).toHaveBeenCalled()
  })

  it('reads the board again when a game starts, clearing the last one', () => {
    const player = onBoard()
    player.gameEnded = true
    send(event('game_started', {}, 5))
    expect(player.gameEnded).toBe(false)
    expect(player.refreshBoard).toHaveBeenCalled()
  })

  it('follows the reaction switch and the details', () => {
    const player = onBoard()
    send(event('yoever_switched', { enabled: false }))
    expect(player.playerGame?.yoeverEnabled).toBe(false)
    send(event('game_details', { details: 'Round 2' }))
    expect(useGameStore().gameDetails).toBe('Round 2')
  })

  it('shows a reaction', () => {
    onBoard()
    const yoever = useYoeverStore()
    yoever.show = vi.fn()
    send(event('yoever', { playerName: 'Tifa', count: 2 }, 4))
    expect(yoever.show).toHaveBeenCalledWith('Tifa')
  })

  it('goes home when its card is deleted, closing its stream', () => {
    const player = onBoard()
    const ui = useUiStore()
    ui.notify = vi.fn()
    send(event('card_deleted'))
    expect(player.playerCard).toBeNull()
    expect(h.push).toHaveBeenCalledWith({ name: 'home' })
    expect(h.feeds.some((f) => f.running)).toBe(false)
  })

  it('reads the board on every open, and the look and settings too on a reopen', async () => {
    const player = onBoard()
    const app = useAppStore()
    app.loadSettings = vi.fn(async () => undefined)
    app.applyThemePreference = vi.fn(async () => undefined)
    open().opts.onOpen!(false)
    await Promise.resolve()
    expect(player.refreshBoard).toHaveBeenCalledTimes(1)
    expect(app.loadSettings).not.toHaveBeenCalled()
    open().opts.onOpen!(true)
    await Promise.resolve()
    expect(player.refreshBoard).toHaveBeenCalledTimes(2)
    expect(app.loadSettings).toHaveBeenCalled()
    expect(app.applyThemePreference).toHaveBeenCalled()
  })

  it('checks its card when the stream is refused, and goes home if it is gone', async () => {
    const player = onBoard()
    useUiStore().notify = vi.fn()
    player.cardExists = vi.fn(async () => false)
    open().opts.onRefused!()
    await vi.waitFor(() => expect(h.push).toHaveBeenCalledWith({ name: 'home' }))
  })

  it("keeps the board when the refusal wasn't about its card", async () => {
    const player = onBoard()
    player.cardExists = vi.fn(async () => true)
    open().opts.onRefused!()
    await Promise.resolve()
    await Promise.resolve()
    expect(h.push).not.toHaveBeenCalled()
    expect(player.playerCard).not.toBeNull()
  })

  it("follows a theme written when it wears the site's or that one", () => {
    onBoard()
    const app = useAppStore()
    app.applyThemePreference = vi.fn(async () => undefined)
    send(event('theme_changed', { styleId: 3 }))
    expect(app.applyThemePreference).toHaveBeenCalledTimes(1)
    app.themePreference = '5'
    send(event('theme_changed', { styleId: 3 }))
    expect(app.applyThemePreference).toHaveBeenCalledTimes(1)
    send(event('theme_changed', { styleId: 5 }))
    expect(app.applyThemePreference).toHaveBeenCalledTimes(2)
  })
})

describe('the admin area', () => {
  function onConsole(account: Partial<Account> = { isAdmin: true }) {
    signIn(account)
    useLiveEvents().follow({ kind: 'staff' })
  }

  it("chimes when the winners grow on another console's draw, once", () => {
    const game = useGameStore()
    game.currentGame = liveGame(4)
    game.setWinnerSoundEnabled(true)
    onConsole()
    const draw = event(
      'draw',
      { drawn: { number: 7, letter: 'B', callOrder: 1 }, winners: ['WIN001'] },
      4,
    )
    send(draw)
    expect(game.currentGame.calledNumbers).toEqual([7])
    expect(game.winners).toEqual(['WIN001'])
    expect(sound.playWinnerChime).toHaveBeenCalledTimes(1)
    send({ ...draw, payload: { ...draw.payload, drawn: { number: 8, letter: 'B', callOrder: 2 } } })
    expect(sound.playWinnerChime).toHaveBeenCalledTimes(1)
  })

  it("ignores a draw for a game this console isn't showing", () => {
    const game = useGameStore()
    game.currentGame = liveGame(4)
    onConsole()
    send(event('draw', { drawn: { number: 7, letter: 'B', callOrder: 1 }, winners: ['X'] }, 3))
    expect(game.currentGame.calledNumbers).toEqual([])
    expect(game.winners).toEqual([])
  })

  it('opens the half-time prompt for its game, and closes it when another console answers', () => {
    const game = useGameStore()
    const ui = useUiStore()
    ui.notify = vi.fn()
    game.currentGame = liveGame(4)
    onConsole()
    send(event('halftime_prompt', { autoPaused: true }, 4))
    expect(game.showHalftimePrompt).toBe(true)
    expect(game.halftimeAutoPaused).toBe(true)
    expect(game.halftimeGameId).toBe(4)
    send(event('halftime_resolved', { minigame: true, by: 'ann' }, 3))
    expect(game.showHalftimePrompt).toBe(true)
    send(event('halftime_resolved', { minigame: true, by: 'ann' }, 4))
    expect(game.showHalftimePrompt).toBe(false)
    expect(ui.notify).toHaveBeenCalledWith(expect.stringContaining('ann'), 'info')
  })

  it('closes the dialogs and reads the game again when its game ends', () => {
    const game = useGameStore()
    game.loadGameState = vi.fn(async () => undefined)
    game.currentGame = liveGame(4)
    game.winners = ['WIN001']
    game.showEndGameModal = true
    game.endGameGameId = 4
    game.showHalftimePrompt = true
    game.winnerPreview = { card: { id: 'WIN001' } as Card, matchedCells: new Set() }
    onConsole()
    send(event('game_ended', {}, 4))
    expect(game.currentGame).toBeNull()
    expect(game.showEndGameModal).toBe(false)
    expect(game.endGameGameId).toBeNull()
    expect(game.showHalftimePrompt).toBe(false)
    expect(game.winnerPreview).toBeNull()
    expect(game.loadGameState).toHaveBeenCalled()
  })

  it('reads the game again when one starts', () => {
    const game = useGameStore()
    game.loadGameState = vi.fn(async () => undefined)
    onConsole()
    send(event('game_started', {}, 5))
    expect(game.loadGameState).toHaveBeenCalled()
  })

  it('follows the controls other consoles change', () => {
    const game = useGameStore()
    const app = useAppStore()
    game.currentGame = liveGame(4)
    onConsole()
    send(event('auto_draw', { enabled: true, interval: 20 }))
    expect(game.currentGame.autoEnabled).toBe(true)
    expect(game.currentGame.autoInterval).toBe(20)
    send(event('yoever_switched', { enabled: false }))
    expect(game.currentGame.yoeverEnabled).toBe(false)
    send(event('draw_delay', { delay: 7 }))
    expect(game.drawDelay).toBe(7)
    expect(app.settings.defaultDrawDelay).toBe(7)
    useYoeverStore().show = vi.fn()
    send(event('yoever', { playerName: 'Tifa', count: 3 }, 4))
    expect(game.currentGame.yoeverCount).toBe(3)
  })

  it('reads the cards, the patterns and the settings again when they change', () => {
    const cards = useCardsStore()
    const patterns = usePatternsStore()
    const app = useAppStore()
    cards.loadCards = vi.fn(async () => undefined)
    patterns.loadPatterns = vi.fn(async () => undefined)
    app.loadSettings = vi.fn(async () => undefined)
    onConsole()
    send(event('cards_changed'))
    send(event('patterns_changed'))
    send(event('settings_changed'))
    expect(cards.loadCards).toHaveBeenCalledTimes(1)
    expect(patterns.loadPatterns).toHaveBeenCalledTimes(1)
    expect(app.loadSettings).toHaveBeenCalledTimes(1)
  })

  it('hands a resource change to the admin store, and a theme change as the themes', () => {
    const admin = useAdminStore()
    admin.refreshResource = vi.fn()
    useAppStore().applyThemePreference = vi.fn(async () => undefined)
    onConsole()
    send(event('resource_changed', { resource: 'garapons' }))
    expect(admin.refreshResource).toHaveBeenCalledWith('garapons')
    send(event('theme_changed', { styleId: null }))
    expect(admin.refreshResource).toHaveBeenCalledWith('themes')
  })

  it('appends a log line to the viewer', () => {
    const logs = useLogsStore()
    logs.appendLive = vi.fn()
    onConsole()
    const entry = { time: 't', level: 'INFO', message: 'm' }
    send(event('log', { entry }))
    expect(logs.appendLive).toHaveBeenCalledWith(entry)
  })

  it('alerts that a tea room unlocked, by its name and number', () => {
    const ui = useUiStore()
    ui.alert = vi.fn()
    onConsole()
    send(event('tea_room_unlocked', { id: 2, name: 'Jasmine Room', roomNumber: '12' }))
    expect(ui.alert).toHaveBeenCalledWith(
      expect.objectContaining({ message: expect.stringContaining('Jasmine Room (Room #12)') }),
    )
    expect(sound.playUnlockChime).toHaveBeenCalled()
  })

  it('reads again on open what the account may see, and the open page', () => {
    const game = useGameStore()
    const cards = useCardsStore()
    const patterns = usePatternsStore()
    const admin = useAdminStore()
    game.loadGameState = vi.fn(async () => undefined)
    cards.loadCards = vi.fn(async () => undefined)
    patterns.loadPatterns = vi.fn(async () => undefined)
    admin.refreshOpenTab = vi.fn()
    onConsole({ permissions: ['bingo-cards'] })
    open().opts.onOpen!(false)
    expect(cards.loadCards).toHaveBeenCalled()
    expect(game.loadGameState).not.toHaveBeenCalled()
    expect(patterns.loadPatterns).not.toHaveBeenCalled()
    expect(admin.refreshOpenTab).toHaveBeenCalled()
  })

  it('reads everything again when told to resync', () => {
    const game = useGameStore()
    const app = useAppStore()
    const admin = useAdminStore()
    game.loadGameState = vi.fn(async () => undefined)
    useCardsStore().loadCards = vi.fn(async () => undefined)
    usePatternsStore().loadPatterns = vi.fn(async () => undefined)
    app.loadSettings = vi.fn(async () => undefined)
    app.applyThemePreference = vi.fn(async () => undefined)
    admin.refreshOpenTab = vi.fn()
    onConsole()
    send(event('resync'))
    expect(game.loadGameState).toHaveBeenCalled()
    expect(app.loadSettings).toHaveBeenCalled()
    expect(admin.refreshOpenTab).toHaveBeenCalled()
  })
})
