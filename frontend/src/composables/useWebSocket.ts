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
import { playDrawChime, vibrate } from '@/lib/sound'
import { applyCustomCSS, applyHeaderFont } from '@/lib/theme'
import type { WsMessage } from '@/types/api'
import { useRouter } from 'vue-router'
import { useUiStore } from '@/stores/ui'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { usePlayerStore } from '@/stores/player'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { endpoints } from '@/lib/endpoints'

export function useWebSocket() {
  const router = useRouter()
  const ui = useUiStore()
  const app = useAppStore()
  const game = useGameStore()
  const player = usePlayerStore()
  const cards = useCardsStore()
  const patterns = usePatternsStore()

  // Route-based context predicates (replace the old `ui.view === …` checks).
  const isPlayerView = (): boolean => router.currentRoute.value.name === 'player'
  const isAdminView = (): boolean => {
    const n = router.currentRoute.value.name
    return typeof n === 'string' && n.startsWith('admin') && n !== 'admin-login'
  }

  const client = new WsClient({
    onMessage: dispatch,
    onReconnected: refreshStateAfterReconnect,
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
        if (isAdminView()) cards.cards = msg.cards || []
        break
      case 'patterns_update':
        if (isAdminView()) {
          patterns.patterns = msg.patterns || []
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
        break
      case 'settings_update':
        if (msg.app_title) {
          app.settings.app_title = msg.app_title
          document.title = msg.app_title
        }
        if (msg.header_font) {
          app.settings.header_font = msg.header_font
          applyHeaderFont(msg.header_font)
        }
        break
      case 'halftime_minigame':
        if (isPlayerView()) player.showMinigameModal = true
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
        player.endedCalledCount = player.playerGame.called_numbers?.length ?? 0
        player.gameEnded = true
        player.lastDrawn = null
      }
      player.playerGame = g
      if (g) {
        if (g.id !== oldGameId) {
          player.loadStamps()
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
    if (!drawn) return

    // Player view — append the drawn number to local called_numbers and surface
    // it as the "last called" announcement (plus the opt-in chime/vibration).
    if (isPlayerView() && player.playerGame) {
      if (!player.playerGame.called_numbers) player.playerGame.called_numbers = []
      player.playerGame.called_numbers.push(drawn.number)
      player.playerGame.total_called = player.playerGame.called_numbers.length
      player.lastDrawn = drawn
      if (player.soundEnabled) {
        playDrawChime()
        vibrate(60)
      }
    }

    // Admin view — only if we don't already have this number
    if (isAdminView() && game.currentGame) {
      if (!game.currentGame.called_numbers) game.currentGame.called_numbers = []
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
   * Handle a card_deleted message — the player's card was deleted server-side.
   * Disconnect, reset, and return home.
   */
  function handleCardDeleted(): void {
    if (isPlayerView()) {
      client.disconnect()
      player.resetPlayer()
      router.push({ name: 'home' })
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
