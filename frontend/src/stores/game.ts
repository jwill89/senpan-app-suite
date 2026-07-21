/**
 * Game store (admin): game lifecycle (start/draw/end), winners + verification,
 * frequent winners, halftime prompt, the draw-delay countdown, game details,
 * and the winners log. Mirrors all admin game logic from app.js.
 *
 * `gameDetails` lives here but is also read by the player view; the App shell
 * keeps them in sync via WebSocket `details_update` messages.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { BingoGameState, Card, FrequentWinner, WinnersLogEntry } from '@/types/api'
import { playWinnerChime } from '@/lib/sound'
import { halftimeCallThreshold } from '@/lib/halftime'
import { DEFAULT_AUTO_INTERVAL } from '@/lib/constants'
import { useUiStore } from './ui'

export const useGameStore = defineStore('game', () => {
  const ui = useUiStore()

  const currentGame = ref<BingoGameState | null>(null)
  const winners = ref<string[]>([])
  const selectedPatternIds = ref<number[]>([])

  /**
   * Admin opt-in: play a chime when a draw produces a new winner so the caller
   * can hear a bingo without watching the screen. Persisted across sessions;
   * off by default (audio needs a user gesture to start — the toggle provides it).
   */
  const winnerSoundEnabled = ref(localStorage.getItem('bingo_admin_winner_sound') === '1')
  function setWinnerSoundEnabled(on: boolean): void {
    winnerSoundEnabled.value = on
    localStorage.setItem('bingo_admin_winner_sound', on ? '1' : '0')
  }
  const lastDrawn = ref<{ number: number; letter: string; call_order: number } | null>(null)
  const gameDetails = ref('')

  const drawDelay = ref(0)
  const drawCountdown = ref<number | null>(null)
  const drawSent = ref(false)
  let drawCountdownTimer: ReturnType<typeof setInterval> | null = null

  // ── Auto-draw (server-driven) ──────────────────────────────────────────────
  // New-game form controls: whether to start with auto on, and the "Time Between
  // Calls" interval. The live game's auto state travels on `currentGame`
  // (auto_enabled / auto_interval) and is toggled via setAuto*.
  const newGameAuto = ref(false)
  const newGameAutoInterval = ref(DEFAULT_AUTO_INTERVAL)
  // True when the half-time prompt fired after auto was paused for it — the modal
  // uses it to explain that declining the mini-game resumes the auto draws.
  const halftimeAutoPaused = ref(false)

  // In-flight flags for the game-control buttons.
  const starting = ref(false)
  const drawing = ref(false)
  const ending = ref(false)
  const winnersLogLoading = ref(false)

  // Winner verification modal: { card, matchedCells }
  const winnerPreview = ref<{ card: Card; matchedCells: Set<string> } | null>(null)
  const winnerLoading = ref(false)

  // Halftime modals
  const showHalftimePrompt = ref(false)
  /** Call number at which the halftime prompt fires — the 35-of-75 mark scaled
   *  to the current game's actual callable pool (driven by its win patterns). */
  const halftimeThreshold = computed(() => halftimeCallThreshold(currentGame.value?.patterns))

  // End-game winner confirmation
  const showEndGameModal = ref(false)
  const endGameSelectedWinners = ref<string[]>([])

  // Frequent winners
  const frequentWinners = ref<FrequentWinner[]>([])

  // Winners log
  const winnersLog = ref<WinnersLogEntry[]>([])
  const winnersLogTotal = ref(0)
  const winnersLogPage = ref(1)
  const winnersLogPerPage = ref(25)
  const winnersLogSort = ref('logged_at')
  const winnersLogDir = ref<'asc' | 'desc'>('desc')

  // ── Computed ───────────────────────────────────────────────────────────────

  const adminCalledSet = computed(() => {
    if (!currentGame.value) return new Set<number>()
    return new Set(currentGame.value.called_numbers)
  })

  const adminGameLabel = computed(() => (currentGame.value ? 'Current Game' : 'New Game'))

  function isCalledAdmin(n: number): boolean {
    return adminCalledSet.value.has(n)
  }

  // ── Game state ─────────────────────────────────────────────────────────────

  async function loadGameState(): Promise<void> {
    try {
      const data = await endpoints.game.getState()
      currentGame.value = data.game ?? null
      winners.value = data.winners
      gameDetails.value = data.game_details || ''
      void loadFrequentWinners()
    } catch {
      /* silent */
    }
  }

  async function startGame(): Promise<void> {
    if (selectedPatternIds.value.length === 0) {
      ui.notify('Select at least one win pattern', 'error')
      return
    }
    starting.value = true
    try {
      const data = await endpoints.game.start(
        selectedPatternIds.value,
        newGameAuto.value,
        newGameAutoInterval.value,
      )
      currentGame.value = data.game ?? null
      winners.value = []
      lastDrawn.value = null
      selectedPatternIds.value = []
      gameDetails.value = data.game_details
      ui.notify(newGameAuto.value ? 'Game started — auto-drawing!' : 'Game started!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      starting.value = false
    }
  }

  async function drawNumber(): Promise<void> {
    if (drawing.value) return
    drawing.value = true
    try {
      const delay = drawDelay.value || 0
      const prevCount = winners.value.length
      const data = await endpoints.game.draw(delay)
      lastDrawn.value = data.drawn
      currentGame.value = data.game
      winners.value = data.winners
      if (winners.value.length > prevCount) {
        ui.notify('We have winner(s)!', 'success', 6000)
        if (winnerSoundEnabled.value) playWinnerChime()
      }

      clearDrawCountdown()
      if (delay > 0) {
        drawSent.value = false
        drawCountdown.value = delay
        drawCountdownTimer = setInterval(() => {
          if (drawCountdown.value === null) return
          drawCountdown.value--
          if (drawCountdown.value <= 0) {
            clearDrawCountdown()
            drawSent.value = true
            setTimeout(() => {
              drawSent.value = false
            }, 3000)
          }
        }, 1000)
      } else {
        drawCountdown.value = null
        drawSent.value = false
      }

      // The half-time prompt is now driven by the server (it must fire for auto
      // draws too, and consistently across every admin surface): the backend
      // detects the crossing on any draw and broadcasts `halftime_prompt`, handled
      // in the WebSocket layer. No client-side detection here.
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      drawing.value = false
    }
  }

  /**
   * Persist the chosen draw delay so it's shared: the server stores it (admins
   * read it as `default_draw_delay` on the next page load) and broadcasts a
   * `draw_delay_update` so other admins' selectors update live. The local
   * `drawDelay` is already set by the selector's v-model; this just propagates it.
   */
  async function persistDrawDelay(): Promise<void> {
    try {
      await endpoints.game.setDelay(drawDelay.value || 0)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function clearDrawCountdown(): void {
    if (drawCountdownTimer) {
      clearInterval(drawCountdownTimer)
      drawCountdownTimer = null
    }
    drawCountdown.value = null
  }

  /** Opens the end-game modal if there are winners, else ends immediately. */
  async function endGame(): Promise<void> {
    if (winners.value.length > 0) {
      endGameSelectedWinners.value = [...winners.value]
      showEndGameModal.value = true
      return
    }
    await confirmEndGame([])
  }

  async function confirmEndGame(validWinnerIds: string[]): Promise<void> {
    showEndGameModal.value = false
    ending.value = true
    try {
      await endpoints.game.end(validWinnerIds)
      currentGame.value = null
      winners.value = []
      lastDrawn.value = null
      winnerPreview.value = null
      clearDrawCountdown()
      drawSent.value = false
      ui.notify('Game ended', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      ending.value = false
    }
  }

  async function saveGameDetails(): Promise<void> {
    try {
      await endpoints.game.updateDetails(gameDetails.value)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Switches the "It's Yoever" reaction on/off for the current game. Optimistic:
   * flips the local flag immediately (the server also broadcasts yoever_config to
   * every client), reverting if the request fails.
   */
  async function setYoeverEnabled(on: boolean): Promise<void> {
    if (currentGame.value) currentGame.value.yoever_enabled = on
    try {
      await endpoints.game.setYoeverEnabled(on)
    } catch (e) {
      if (currentGame.value) currentGame.value.yoever_enabled = !on
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Switches the automatic-draw loop on/off for the current game. Optimistic:
   * flips `auto_enabled` immediately (the server also broadcasts `auto_config` to
   * every admin), reverting on failure. This is a live, game-level control — it
   * never writes back to a preset the game was started from.
   */
  async function setAutoEnabled(on: boolean): Promise<void> {
    if (currentGame.value) currentGame.value.auto_enabled = on
    try {
      await endpoints.game.setAutoEnabled(on)
    } catch (e) {
      if (currentGame.value) currentGame.value.auto_enabled = !on
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Adjusts the seconds between automatic draws for the current game (optimistic). */
  async function setAutoInterval(seconds: number): Promise<void> {
    const prev = currentGame.value?.auto_interval
    if (currentGame.value) currentGame.value.auto_interval = seconds
    try {
      await endpoints.game.setAutoInterval(seconds)
    } catch (e) {
      if (currentGame.value && prev !== undefined) currentGame.value.auto_interval = prev
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Winner verification ────────────────────────────────────────────────────

  /** Fetches a winning card and highlights cells completing the win patterns. */
  async function viewWinner(cardId: string): Promise<void> {
    if (winnerLoading.value) return
    winnerLoading.value = true
    try {
      const data = await endpoints.board.get(cardId)
      const card = data.card
      const calledSet = adminCalledSet.value
      const patterns = currentGame.value ? currentGame.value.patterns : []
      const matchedCells = new Set<string>()
      for (const pat of patterns) {
        const pd = pat.pattern_data
        let satisfied = true
        for (let r = 0; r < 5; r++) {
          for (let c = 0; c < 5; c++) {
            if (pd[r][c]) {
              const val = card.board_data[r][c]
              if (val !== 0 && !calledSet.has(val)) {
                satisfied = false
                break
              }
            }
          }
          if (!satisfied) break
        }
        if (satisfied) {
          for (let r = 0; r < 5; r++) {
            for (let c = 0; c < 5; c++) {
              if (pd[r][c]) matchedCells.add(`${r}-${c}`)
            }
          }
        }
      }
      winnerPreview.value = { card, matchedCells }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      winnerLoading.value = false
    }
  }

  function isWinnerCellMatch(ri: number, ci: number): boolean {
    if (!winnerPreview.value) return false
    return winnerPreview.value.matchedCells.has(`${ri}-${ci}`)
  }

  // ── Halftime ───────────────────────────────────────────────────────────────

  /** "Yes, run a mini-game": alert players (the server holds the alert until the
   *  triggering number has reached them, and keeps auto paused). */
  async function confirmHalftime(): Promise<void> {
    showHalftimePrompt.value = false
    halftimeAutoPaused.value = false
    try {
      await endpoints.game.halftime(true)
      ui.notify('Halftime alert sent to all players!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** "No mini-game": don't alert players; the server resumes the auto loop if it
   *  had paused it for the prompt. Also the modal's close handler. */
  async function dismissHalftime(): Promise<void> {
    showHalftimePrompt.value = false
    const resumed = halftimeAutoPaused.value
    halftimeAutoPaused.value = false
    try {
      await endpoints.game.halftime(false)
      if (resumed) ui.notify('Auto-draw resumed.', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Frequent winners ───────────────────────────────────────────────────────

  async function loadFrequentWinners(): Promise<void> {
    try {
      const data = await endpoints.winnersLog.frequent()
      frequentWinners.value = data.winners
    } catch {
      /* silent */
    }
  }

  // ── Winners log ────────────────────────────────────────────────────────────

  async function loadWinnersLog(): Promise<void> {
    winnersLogLoading.value = true
    try {
      const data = await endpoints.winnersLog.list({
        page: winnersLogPage.value,
        perPage: winnersLogPerPage.value,
        sort: winnersLogSort.value,
        dir: winnersLogDir.value,
      })
      winnersLog.value = data.entries
      winnersLogTotal.value = data.total || 0
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      winnersLogLoading.value = false
    }
  }

  function winnersLogTotalPages(): number {
    return Math.ceil(winnersLogTotal.value / winnersLogPerPage.value) || 1
  }

  function winnersLogSetSort(field: string): void {
    if (winnersLogSort.value === field) {
      winnersLogDir.value = winnersLogDir.value === 'asc' ? 'desc' : 'asc'
    } else {
      winnersLogSort.value = field
      winnersLogDir.value = 'desc'
    }
    winnersLogPage.value = 1
    void loadWinnersLog()
  }

  function winnersLogGoPage(p: number): void {
    winnersLogPage.value = p
    void loadWinnersLog()
  }

  /** Deletes one winners-log entry (with confirm), then reloads the page. */
  async function deleteWinnerLogEntry(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this winners-log entry? This cannot be undone.', {
        title: 'Delete entry',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.winnersLog.delete(id)
      ui.notify('Entry deleted', 'info')
      await loadWinnersLog()
      // If that emptied the last page, step back one so we don't strand the user.
      if (winnersLog.value.length === 0 && winnersLogPage.value > 1) {
        winnersLogPage.value--
        await loadWinnersLog()
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Clears the entire winners log (with confirm), then reloads from page 1. */
  async function deleteAllWinnersLog(): Promise<void> {
    if (
      !(await ui.confirm('Delete ALL winners-log entries? This cannot be undone.', {
        title: 'Delete all entries',
        confirmText: 'Delete all',
      }))
    )
      return
    try {
      await endpoints.winnersLog.deleteAll()
      ui.notify('All winners-log entries deleted', 'info')
      winnersLogPage.value = 1
      await loadWinnersLog()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    currentGame,
    winners,
    selectedPatternIds,
    winnerSoundEnabled,
    setWinnerSoundEnabled,
    lastDrawn,
    gameDetails,
    drawDelay,
    drawCountdown,
    drawSent,
    newGameAuto,
    newGameAutoInterval,
    halftimeAutoPaused,
    starting,
    drawing,
    ending,
    winnersLogLoading,
    winnerPreview,
    winnerLoading,
    showHalftimePrompt,
    halftimeThreshold,
    showEndGameModal,
    endGameSelectedWinners,
    frequentWinners,
    winnersLog,
    winnersLogTotal,
    winnersLogPage,
    winnersLogPerPage,
    winnersLogSort,
    winnersLogDir,
    adminCalledSet,
    adminGameLabel,
    isCalledAdmin,
    loadGameState,
    startGame,
    drawNumber,
    persistDrawDelay,
    clearDrawCountdown,
    endGame,
    confirmEndGame,
    saveGameDetails,
    setYoeverEnabled,
    setAutoEnabled,
    setAutoInterval,
    viewWinner,
    isWinnerCellMatch,
    confirmHalftime,
    dismissHalftime,
    loadFrequentWinners,
    loadWinnersLog,
    winnersLogTotalPages,
    winnersLogSetSort,
    winnersLogGoPage,
    deleteWinnerLogEntry,
    deleteAllWinnersLog,
  }
})
