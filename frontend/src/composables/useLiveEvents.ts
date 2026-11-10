/**
 * The page's live events: one stream per tab, the board's on a player's board and
 * the staff's in the admin area, with each event handed to the stores.
 *
 * An event names what changed, and the store reads it again over REST; a draw
 * carries its number, and an event with nothing to read again (a reaction, an
 * answered prompt, a log line) carries what it reports. Every time a stream opens
 * the page reads again what it shows, since anything may have changed while it
 * was closed. The event types come from the API's document (lib/feed.ts), and each
 * stream has a handler for every one of its events.
 */
import { useRouter } from 'vue-router'
import { Feed, type BoardEvent, type FeedStatus, type Handlers, type StaffEvent } from '@/lib/feed'
import { playEvent, playUnlockChime, playYoeverSound, vibrate } from '@/lib/sound'
import { useUiStore } from '@/stores/ui'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { useAdminStore } from '@/stores/admin'
import { useLogsStore } from '@/stores/logs'
import { useYoeverStore } from '@/stores/yoever'
import { useAuthStore } from '@/stores/auth'
import { teaRoomUnlockedMessage } from '@/stores/teaRooms'

/** What App.vue keeps open: a board's stream, the staff's, or none. */
export type LiveTarget = { kind: 'board'; cardId: string } | { kind: 'staff' } | null

export function useLiveEvents() {
  const router = useRouter()
  const ui = useUiStore()
  const app = useAppStore()
  const game = useGameStore()
  const player = usePlayerStore()
  const cards = useCardsStore()
  const patterns = usePatternsStore()
  const admin = useAdminStore()
  const logs = useLogsStore()
  const yoever = useYoeverStore()
  const auth = useAuthStore()

  let feed: Feed<StaffEvent> | Feed<BoardEvent> | null = null
  let key: string | null = null

  const notify = (message: string, type: 'info' | 'success') => ui.notify(message, type)
  const onStatus = (s: FeedStatus) => ui.setFeedStatus(s)

  // -- Shared by both streams --------------------------------------------------

  /** A theme was written: a page wearing it, or wearing the site's, reads it again. */
  function themeChanged(styleId: number | null): void {
    const pref = app.themePreference
    if (pref === 'default' || pref === String(styleId)) {
      void app.applyThemePreference()
    }
  }

  /** A player's reaction: the overlay shows it, with its sound if wanted. */
  function showYoever(playerName: string): void {
    yoever.show(playerName)
    if (!yoever.muted && yoever.soundEnabled) {
      playYoeverSound()
    }
  }

  // -- A player's board ---------------------------------------------------------

  /** Reads the board again; on a reopen, the site's look and settings too. */
  async function refreshBoard(reopened: boolean): Promise<void> {
    if (reopened) {
      void app.loadSettings()
      void app.applyThemePreference()
    }
    const details = await player.refreshBoard()
    if (details !== null) {
      game.gameDetails = details
    }
  }

  /** The game this board watches has ended: a thank-you summary, then a fresh read. */
  function boardGameEnded(gameId: number | null): void {
    const watching = player.playerGame
    if (watching?.id === gameId) {
      player.endedCalledCount = watching.calledNumbers.length
      player.gameEnded = true
      player.lastDrawn = null
      player.showMinigameModal = false
      player.playerGame = null
      game.gameDetails = ''
      if (player.soundMode !== 'off') {
        playEvent('gameend', player.soundMode)
      }
    }
    void refreshBoard(false)
  }

  /** The card was deleted: the board goes home. Its stream has ended. */
  function cardDeleted(): void {
    stop()
    player.resetPlayer()
    void router.push({ name: 'home' })
    ui.notify('Your card has been deleted. You have been logged out.', 'error')
  }

  const boardHandlers: Handlers<BoardEvent> = {
    game_started: () => {
      player.gameEnded = false
      player.lastDrawn = null
      player.showMinigameModal = false
      void refreshBoard(false)
    },
    game_ended: (e) => boardGameEnded(e.gameId),
    draw: (e) => {
      const g = player.playerGame
      const drawn = e.payload.drawn
      // A draw for another game (around a game change) isn't this board's; one it
      // already has (a resumed stream) isn't news.
      if (g?.id !== e.gameId || g.calledNumbers.includes(drawn.number)) {
        return
      }
      g.calledNumbers.push(drawn.number)
      g.totalCalled = g.calledNumbers.length
      player.lastDrawn = drawn
      if (player.soundMode !== 'off') {
        playEvent('draw', player.soundMode)
        vibrate(60)
      }
    },
    minigame: (e) => {
      if (
        e.gameId !== null &&
        e.gameId === player.playerGame?.id &&
        player.alertMinigame(e.gameId)
      ) {
        if (player.soundMode !== 'off') {
          playEvent('minigame', player.soundMode)
        }
      }
    },
    yoever: (e) => showYoever(e.payload.playerName),
    yoever_switched: (e) => {
      if (player.playerGame) {
        player.playerGame.yoeverEnabled = e.payload.enabled
      }
    },
    game_details: (e) => {
      game.gameDetails = e.payload.details
    },
    settings_changed: () => void app.loadSettings(),
    theme_changed: (e) => themeChanged(e.payload.styleId),
    card_deleted: () => cardDeleted(),
    resync: () => void refreshBoard(true),
    ping: () => {
      // Nothing to do: any event, this one too, tells the stream it is alive.
    },
  }

  // -- The admin area -----------------------------------------------------------

  /** Reads again what the admin area shows: the game, the cards, the patterns and the open page. */
  function refreshStaff(reopened: boolean): void {
    if (reopened) {
      void app.loadSettings()
      void app.applyThemePreference()
    }
    if (auth.hasPermission('bingo-game')) {
      void game.loadGameState()
    }
    if (auth.hasPermission('bingo-cards')) {
      void cards.loadCards()
    }
    if (auth.hasPermission('bingo-patterns')) {
      void patterns.loadPatterns()
    }
    admin.refreshOpenTab()
  }

  const staffHandlers: Handlers<StaffEvent> = {
    game_started: () => void game.loadGameState(),
    game_ended: (e) => {
      game.takeGameEnded(e.gameId)
      void game.loadGameState()
    },
    draw: (e) => {
      if (e.gameId !== null) {
        game.takeDraw(e.gameId, e.payload.drawn, e.payload.winners)
      }
    },
    halftime_prompt: (e) => {
      if (e.gameId !== null) {
        game.takeHalftimePrompt(e.gameId, e.payload.autoPaused)
      }
    },
    halftime_resolved: (e) => {
      if (e.gameId !== null) {
        game.takeHalftimeResolved(e.gameId, e.payload.minigame, e.payload.by)
      }
    },
    auto_draw: (e) => {
      if (game.currentGame) {
        game.currentGame.autoEnabled = e.payload.enabled
        game.currentGame.autoInterval = e.payload.interval
      }
    },
    yoever: (e) => {
      // The console's count follows even when this console hides the effect.
      const g = game.currentGame
      if (g?.id === e.gameId) {
        g.yoeverCount = e.payload.count
      }
      showYoever(e.payload.playerName)
    },
    yoever_switched: (e) => {
      if (game.currentGame) {
        game.currentGame.yoeverEnabled = e.payload.enabled
      }
    },
    draw_delay: (e) => {
      game.drawDelay = e.payload.delay
      app.settings.defaultDrawDelay = e.payload.delay
    },
    game_details: (e) => {
      game.gameDetails = e.payload.details
    },
    cards_changed: () => void cards.loadCards(),
    patterns_changed: () => void patterns.loadPatterns(),
    resource_changed: (e) => admin.refreshResource(e.payload.resource),
    settings_changed: () => void app.loadSettings(),
    theme_changed: (e) => {
      themeChanged(e.payload.styleId)
      admin.refreshResource('themes')
    },
    tea_room_unlocked: (e) => {
      // Nobody did anything, so nobody is looking: alert whoever could go and act
      // on it. The rooms list reads itself again off the resource_changed that
      // follows.
      ui.alert({
        title: 'Tea room unlocked',
        message: teaRoomUnlockedMessage(e.payload.name, e.payload.roomNumber),
        action: { label: 'Open Tea Rooms', to: '/admin/teahouse/tea-rooms' },
      })
      playUnlockChime()
      vibrate([200, 100, 200])
    },
    log: (e) => logs.appendLive(e.payload.entry),
    resync: () => refreshStaff(true),
    ping: () => {
      // Nothing to do: any event, this one too, tells the stream it is alive.
    },
  }

  // -- Opening and closing --------------------------------------------------------

  /**
   * Keeps open the stream for target, closing any other: one stream per tab. The
   * same target again leaves the open stream alone.
   */
  function follow(target: LiveTarget): void {
    const next =
      target === null ? null : target.kind === 'board' ? `board:${target.cardId}` : 'staff'
    if (next === key) {
      return
    }
    stop()
    key = next
    if (target === null) {
      return
    }
    if (target.kind === 'board') {
      const board = new Feed<BoardEvent>({
        url: `/api/v1/board/${encodeURIComponent(target.cardId)}/events`,
        handlers: boardHandlers,
        onOpen: (reopened) => void refreshBoard(reopened),
        // A refusal may mean the card is gone: reading the board says so.
        onRefused: () => void checkCard(),
        onStatus,
        notify,
      })
      feed = board
      board.start()
    } else {
      const staff = new Feed<StaffEvent>({
        url: '/api/v1/events',
        handlers: staffHandlers,
        onOpen: (reopened) => refreshStaff(reopened),
        onStatus,
        notify,
      })
      feed = staff
      staff.start()
    }
  }

  /** A board whose stream was refused reads its card: one deleted sends it home. */
  async function checkCard(): Promise<void> {
    if (player.playerCard && !(await player.cardExists())) {
      cardDeleted()
    }
  }

  /** Closes the open stream, if any. */
  function stop(): void {
    feed?.stop()
    feed = null
    key = null
  }

  return { follow, stop }
}
