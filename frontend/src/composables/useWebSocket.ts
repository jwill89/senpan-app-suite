/**
 * WebSocket integration composable.
 *
 * Wires the shared `WsClient` (lib/ws.ts) to the Pinia stores, reproducing the
 * exact message-dispatch behaviour from the original app.js `connectWS` /
 * `_handleGameUpdate` / `_handleGameDraw` / etc.
 *
 * The connection is opened for both the player and admin views; which store(s)
 * a message updates depends on the current route (player vs. admin area).
 */
import { WsClient } from '@/lib/ws'
import { playEvent, playYoeverSound, vibrate } from '@/lib/sound'
import { applyCustomCSS, applyHeaderFont, applyUploadedFonts } from '@/lib/theme'
import type { WsMessage } from '@/types/api'
import { useRouter } from 'vue-router'
import { useUiStore } from '@/stores/ui'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { useAdminStore } from '@/stores/admin'
import { useLogsStore } from '@/stores/logs'
import { useYoeverStore } from '@/stores/yoever'
import { endpoints } from '@/lib/endpoints'

export function useWebSocket() {
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

  // Route-based context predicates (replace the old `ui.view === …` checks).
  const isPlayerView = (): boolean => router.currentRoute.value.name === 'player'
  const isAdminView = (): boolean => {
    const n = router.currentRoute.value.name
    // The auth pages (login/register) are under the `admin-` name prefix but have
    // no live data, so they must NOT keep a socket open — otherwise a post-logout
    // landing on them fires "Connection lost. Reconnecting…" toasts.
    return (
      typeof n === 'string' &&
      n.startsWith('admin') &&
      n !== 'admin-login' &&
      n !== 'admin-register'
    )
  }

  const client = new WsClient({
    onMessage: dispatch,
    onReconnected: () => {
      void refreshStateAfterReconnect()
    },
    notify: (m, t) => ui.notify(m, t),
    // Keep reconnecting while on the player or admin view.
    shouldReconnect: () => isPlayerView() || isAdminView(),
    // Surface connection state for the player's "Live"/"Reconnecting" badge.
    onStatus: (s) => ui.setWsStatus(s),
  })

  /** Dispatches a parsed WebSocket message to the relevant store(s). */
  function dispatch(msg: WsMessage): void {
    switch (msg.type) {
      case 'game_update':
        handleGameUpdate(msg)
        break
      case 'game_draw':
        handleGameDraw(msg)
        break
      case 'cards_update':
        if (isAdminView()) cards.cards = msg.cards
        break
      case 'patterns_update':
        if (isAdminView()) {
          patterns.patterns = msg.patterns
          if (msg.categories) patterns.categories = msg.categories
        }
        break
      case 'card_deleted':
        handleCardDeleted()
        break
      case 'details_update':
        game.gameDetails = msg.game_details || ''
        break
      case 'style_update':
        applyCustomCSS(msg.css || '')
        app.applyFlourishes(msg.board_flourish || '', msg.number_flourish || '')
        break
      case 'settings_update':
        if (msg.app_title) {
          app.settings.app_title = msg.app_title
          document.title = msg.app_title
        }
        if (msg.header_font) {
          // Register any uploaded fonts the broadcast carried so a newly
          // selected uploaded font renders without a reload, then apply.
          if (msg.uploaded_fonts) {
            app.uploadedFonts = msg.uploaded_fonts
            applyUploadedFonts(msg.uploaded_fonts)
          }
          app.settings.header_font = msg.header_font
          applyHeaderFont(msg.header_font)
        }
        break
      case 'halftime_minigame':
        if (isPlayerView()) {
          player.showMinigameModal = true
          const mode = player.soundMode
          if (mode !== 'off') playEvent('minigame', mode)
        }
        break
      case 'yoever':
        handleYoever(msg)
        break
      case 'yoever_config':
        // An admin switched the reaction on/off: reflect it on whichever game
        // state is loaded so the player's trigger button shows/hides and every
        // admin's toggle stays in step.
        if (isPlayerView() && player.playerGame) player.playerGame.yoever_enabled = msg.enabled
        if (isAdminView() && game.currentGame) game.currentGame.yoever_enabled = msg.enabled
        break
      case 'draw_delay_update':
        // Shared draw-delay control: sync every admin's selector live, and keep
        // the settings copy in step so the Settings page reflects it too.
        if (isAdminView()) {
          game.drawDelay = msg.delay
          app.settings.default_draw_delay = String(msg.delay)
        }
        break
      case 'resource_changed':
        // An admin elsewhere mutated a CRUD resource: if we're viewing it, refetch
        // it (the REST load re-applies our own permission guard); otherwise mark it
        // stale so the next navigation refetches. See admin.refreshResource.
        if (isAdminView()) admin.refreshResource(msg.resource)
        break
      case 'log':
        // Live server-log tail — append to the viewer (it self-limits to the
        // admin Logs tab: entries only accumulate while that store is in use).
        if (isAdminView()) logs.appendLive(msg.entry)
        break
    }
  }

  /**
   * Handle a game_update message (game start or end). Players receive game
   * state + details on start; admins receive game state + winners. On end,
   * both receive game=null.
   */
  function handleGameUpdate(msg: Extract<WsMessage, { type: 'game_update' }>): void {
    const g = msg.game

    if (msg.game_details !== undefined) game.gameDetails = msg.game_details

    // Player view
    if (isPlayerView() && player.playerCard) {
      const oldGameId = player.playerGame?.id
      if (!g && player.playerGame) {
        // The game the player was watching has ended — show a thank-you summary
        // (a neutral fact, the call count; we never track their board for them).
        player.endedCalledCount = player.playerGame.called_numbers.length
        player.gameEnded = true
        player.lastDrawn = null
        const mode = player.soundMode
        if (mode !== 'off') playEvent('gameend', mode)
      }
      player.playerGame = g
      if (g) {
        if (g.id !== oldGameId) {
          player.loadStamps()
          // A new game clears every card's cooldown server-side — re-read ours
          // (a fresh game id has no stored expiry) so the button starts enabled.
          player.loadYoeverCooldown()
          // A new game started — clear any prior "game over"/last-called state.
          player.gameEnded = false
          player.lastDrawn = null
        }
      } else {
        game.gameDetails = ''
      }
    }

    // Admin view
    if (isAdminView()) {
      game.currentGame = g
      game.winners = msg.winners || []
      if (!g) {
        game.lastDrawn = null
        game.winnerPreview = null
      }
    }
  }

  /**
   * Handle a game_draw message. Players append the drawn number to local state;
   * admins also receive an updated winners list (skipping numbers they already
   * have from the HTTP draw response).
   */
  function handleGameDraw(msg: Extract<WsMessage, { type: 'game_draw' }>): void {
    const drawn = msg.drawn

    // Player view — append the drawn number to local called_numbers and surface
    // it as the "last called" announcement (plus the opt-in chime/vibration).
    if (isPlayerView() && player.playerGame) {
      player.playerGame.called_numbers.push(drawn.number)
      player.playerGame.total_called = player.playerGame.called_numbers.length
      player.lastDrawn = drawn
      const mode = player.soundMode
      if (mode !== 'off') {
        playEvent('draw', mode)
        vibrate(60)
      }
    }

    // Admin view — only if we don't already have this number
    if (isAdminView() && game.currentGame) {
      const alreadyHas = game.currentGame.called_numbers.includes(drawn.number)
      if (!alreadyHas) {
        game.currentGame.called_numbers.push(drawn.number)
        game.currentGame.total_called = game.currentGame.called_numbers.length
        game.lastDrawn = drawn
      }
      const prevWinnerCount = game.winners.length
      if (msg.winners) game.winners = msg.winners
      if (game.winners.length > prevWinnerCount && game.winners.length > 0) {
        ui.notify('We have winner(s)!', 'success', 6000)
      }
    }
  }

  /**
   * Handle a `yoever` reaction fired by some player. The admin "Yoevers: N"
   * counter always updates (even when this client has opted out, so an admin
   * monitoring the game still sees it climb). "Show effects" (not muted) is the
   * master: `show()` self-gates the animation on it, and the sound only plays when
   * effects are shown AND the separate "play sound" toggle is on — the latter is
   * independent of the main sound mode, but still at the master volume.
   */
  function handleYoever(msg: Extract<WsMessage, { type: 'yoever' }>): void {
    if (isAdminView() && game.currentGame) game.currentGame.yoever_count = msg.count
    yoever.show(msg.player_name)
    if (!yoever.muted && yoever.soundEnabled) playYoeverSound()
  }

  /**
   * Handle a card_deleted message — the player's card was deleted server-side.
   * Disconnect, reset, and return home.
   */
  function handleCardDeleted(): void {
    if (isPlayerView()) {
      client.disconnect()
      player.resetPlayer()
      void router.push({ name: 'home' })
      ui.notify('Your card has been deleted. You have been logged out.', 'error')
    }
  }

  /**
   * After a reconnect, fetch the latest state via REST to catch up on any
   * updates missed while disconnected.
   */
  async function refreshStateAfterReconnect(): Promise<void> {
    try {
      if (isPlayerView() && player.playerCard) {
        const data = await endpoints.board.get(player.playerCard.id)
        player.playerGame = data.game ?? null
        game.gameDetails = data.game_details || ''
      } else if (isAdminView()) {
        await game.loadGameState()
      }
    } catch {
      /* silent — WebSocket will deliver future updates */
    }
  }

  return { client }
}
