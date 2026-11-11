/**
 * Game store (admin): game lifecycle (start/draw/end), winners + verification,
 * frequent winners, halftime prompt, the draw-delay countdown, game details,
 * and the winners log.
 *
 * Several consoles (and the plugin) run one game, so what one does reaches the
 * others as live events (useLiveEvents hands each one here): a draw, the winners
 * growing (the chime sounds on every console), the half-time prompt being answered,
 * the game starting or ending (the game is read again). The game state the server
 * answers carries what a console opening mid-way needs: the draw delay, the prompt
 * waiting, and the last draw with when players get it.
 *
 * `gameDetails` lives here but is also read by the player view; the live
 * `game_details` event keeps both in step.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  api,
  unwrap,
  type Card,
  type DrawnNumber,
  type FrequentWinner,
  type Game,
  type GameState,
  type WinnersLogEntry,
} from '@/api/client'
import type { ApiError } from '@/lib/api'
import { playWinnerChime } from '@/lib/sound'
import { halftimeCallThreshold } from '@/lib/halftime'
import { DEFAULT_AUTO_INTERVAL } from '@/lib/constants'
import { useUiStore } from './ui'

// Safe cell readers for winner verification. The winner's board/pattern grids come
// from the server; a malformed (non-5x5) grid would otherwise throw on a missing
// row. Typed `unknown` on purpose so the shape checks are genuine (not flagged as
// unnecessary by the type checker).
function patternCell(grid: unknown, r: number, c: number): boolean {
  if (!Array.isArray(grid)) {
    return false
  }
  const row = grid[r]
  return Array.isArray(row) ? Boolean(row[c]) : false
}
function boardCell(grid: unknown, r: number, c: number): number {
  if (!Array.isArray(grid)) {
    return 0
  }
  const row = grid[r]
  if (!Array.isArray(row)) {
    return 0
  }
  const val = row[c]
  return typeof val === 'number' ? val : 0
}

export const useGameStore = defineStore('game', () => {
  const ui = useUiStore()

  const currentGame = ref<Game | null>(null)
  const winners = ref<string[]>([])
  const selectedPatternIds = ref<number[]>([])

  /**
   * Admin opt-in: play a chime when the winners grow so the caller can hear a
   * bingo without watching the screen - whoever drew the number. Persisted across
   * sessions; off by default (audio needs a user gesture to start - the toggle
   * provides it).
   */
  const winnerSoundEnabled = ref(localStorage.getItem('bingo_admin_winner_sound') === '1')
  function setWinnerSoundEnabled(on: boolean): void {
    winnerSoundEnabled.value = on
    localStorage.setItem('bingo_admin_winner_sound', on ? '1' : '0')
  }
  const lastDrawn = ref<DrawnNumber | null>(null)
  const gameDetails = ref('')

  const drawDelay = ref(0)
  const drawCountdown = ref<number | null>(null)
  const drawSent = ref(false)
  let drawCountdownTimer: ReturnType<typeof setInterval> | null = null
  // The one-shot timer that clears `drawSent` a few seconds after the countdown
  // elapses. Tracked so a fresh draw can cancel a stale one - otherwise its
  // callback could flip `drawSent` back off underneath a newer draw's state.
  let drawSentTimer: ReturnType<typeof setTimeout> | null = null

  // -- Auto-draw (server-driven) ----------------------------------------------
  // New-game form controls: whether to start with auto on, and the "Time Between
  // Calls" interval. The live game's auto state travels on `currentGame`
  // (autoEnabled / autoInterval) and is toggled via setAuto*.
  const newGameAuto = ref(false)
  const newGameAutoInterval = ref(DEFAULT_AUTO_INTERVAL)
  // True when the half-time prompt fired after auto was paused for it - the modal
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

  // Halftime modals. The prompt names the game it was raised for: the answer
  // sends it, so an answer left open across games is refused rather than taken.
  const showHalftimePrompt = ref(false)
  const halftimeGameId = ref<number | null>(null)
  /** Call number at which the halftime prompt fires - the 35-of-75 mark scaled
   *  to the current game's actual callable pool (driven by its win patterns). */
  const halftimeThreshold = computed(() => halftimeCallThreshold(currentGame.value?.patterns))

  // End-game winner confirmation, for the game it was opened on.
  const showEndGameModal = ref(false)
  const endGameSelectedWinners = ref<string[]>([])
  const endGameGameId = ref<number | null>(null)

  // Frequent winners
  const frequentWinners = ref<FrequentWinner[]>([])

  // Winners log
  const winnersLog = ref<WinnersLogEntry[]>([])
  const winnersLogTotal = ref(0)
  const winnersLogPage = ref(1)
  const winnersLogPerPage = ref(25)

  // -- Computed ---------------------------------------------------------------

  const adminCalledSet = computed(() => {
    if (!currentGame.value) {
      return new Set<number>()
    }
    return new Set(currentGame.value.calledNumbers)
  })

  const adminGameLabel = computed(() => (currentGame.value ? 'Current Game' : 'New Game'))

  function isCalledAdmin(n: number): boolean {
    return adminCalledSet.value.has(n)
  }

  // -- Keeping in step --------------------------------------------------------

  /**
   * Takes in the winners. When they grew - from this console's draw, another's,
   * the plugin's or an automatic one - every console says so, and chimes if its
   * caller asked to hear it. `announce` is off for a load, which isn't news.
   */
  function setWinners(list: string[], announce = true): void {
    const grew = list.length > winners.value.length
    winners.value = list
    if (announce && grew) {
      ui.notify('We have winner(s)!', 'success', 6000)
      if (winnerSoundEnabled.value) {
        playWinnerChime()
      }
    }
  }

  function closeHalftime(): void {
    showHalftimePrompt.value = false
    halftimeAutoPaused.value = false
    halftimeGameId.value = null
  }

  /**
   * The game changed under the open dialogs (it ended, or another started): the
   * end-game confirm, the half-time prompt and the winner preview belonged to the
   * game that was, so they close.
   */
  function closeGameDialogs(): void {
    showEndGameModal.value = false
    endGameGameId.value = null
    closeHalftime()
    winnerPreview.value = null
  }

  /** Forgets the game that ended, here or on another console. */
  function clearGame(): void {
    closeGameDialogs()
    currentGame.value = null
    winners.value = []
    lastDrawn.value = null
    clearDrawCountdown()
    drawSent.value = false
  }

  /**
   * Takes in the game state the server answered: the game, its winners and details,
   * the shared draw delay, the half-time prompt still waiting (so a console opening
   * mid-way shows it) and the last draw, counting down to when players get it.
   */
  function applyState(state: GameState): void {
    const g = state.game
    if (!g || g.id !== currentGame.value?.id) {
      closeGameDialogs()
    }
    currentGame.value = g
    setWinners(state.winners, false)
    gameDetails.value = state.gameDetails
    drawDelay.value = state.drawDelay
    lastDrawn.value = g?.lastDraw
      ? { number: g.lastDraw.number, letter: g.lastDraw.letter, callOrder: g.lastDraw.callOrder }
      : null
    if (g?.halftime) {
      showHalftimePrompt.value = true
      halftimeAutoPaused.value = g.halftime.autoPaused
      halftimeGameId.value = g.halftime.gameId
    } else {
      closeHalftime()
    }
    const playersAt = g?.lastDraw?.playersAt
    const left = playersAt ? Math.ceil((Date.parse(playersAt) - Date.now()) / 1000) : 0
    if (left > 0) {
      startCountdown(left)
    } else if (!g) {
      clearDrawCountdown()
    }
  }

  /**
   * A game ended on some console (`game_ended`): the one this console shows closes
   * with its dialogs. The caller reads the state again, in case another has started.
   */
  function takeGameEnded(gameId: number | null): void {
    if (currentGame.value?.id === gameId) {
      clearGame()
    }
  }

  /** A number drawn on some console, or automatically (`draw`, staff's copy). */
  function takeDraw(gameId: number, drawn: DrawnNumber, list: string[] | undefined): void {
    const g = currentGame.value
    if (g?.id !== gameId) {
      return
    }
    if (!g.calledNumbers.includes(drawn.number)) {
      g.calledNumbers.push(drawn.number)
      g.totalCalled = g.calledNumbers.length
      lastDrawn.value = drawn
    }
    if (list) {
      setWinners(list)
    }
  }

  /** The server reached the half-time mark (`halftime_prompt`). */
  function takeHalftimePrompt(gameId: number, autoPaused: boolean): void {
    showHalftimePrompt.value = true
    halftimeAutoPaused.value = autoPaused
    halftimeGameId.value = gameId
  }

  /**
   * Someone answered the half-time prompt (`halftime_resolved`): a console still
   * showing it closes it and says who answered, so nobody answers it twice.
   */
  function takeHalftimeResolved(gameId: number, minigame: boolean, by: string): void {
    if (!showHalftimePrompt.value || halftimeGameId.value !== gameId) {
      return
    }
    closeHalftime()
    ui.notify(
      `${by} answered the half-time prompt: ${minigame ? 'a mini-game is on' : 'no mini-game'}.`,
      'info',
    )
  }

  // -- Game state -------------------------------------------------------------

  async function loadGameState(): Promise<void> {
    try {
      applyState(await unwrap(api.GET('/game')))
      void loadFrequentWinners()
    } catch {
      /* silent */
    }
  }

  /**
   * Starts a game. A game already running (another console started it, and this
   * one hasn't heard) is refused: staff are asked whether to replace it, since its
   * winners won't be logged.
   */
  async function startGame(replace = false): Promise<void> {
    if (selectedPatternIds.value.length === 0) {
      ui.notify('Select at least one win pattern', 'error')
      return
    }
    starting.value = true
    try {
      const state = await unwrap(
        api.POST('/game/start', {
          body: {
            patternIds: selectedPatternIds.value,
            auto: newGameAuto.value,
            autoInterval: newGameAutoInterval.value,
            replace,
          },
        }),
      )
      applyState(state)
      lastDrawn.value = null
      selectedPatternIds.value = []
      ui.notify(newGameAuto.value ? 'Game started - auto-drawing!' : 'Game started!', 'success')
    } catch (e) {
      const err = e as ApiError
      if (err.status !== 409 || replace) {
        ui.notify(err.message, 'error')
        return
      }
      starting.value = false
      const replaceIt = await ui.confirm(
        "A game is already running - another console may have started it. Replace it with this one? Its winners won't be logged.",
        { title: 'Replace the running game?', confirmText: 'Replace' },
      )
      if (replaceIt) {
        await startGame(true)
      } else {
        await loadGameState()
      }
    } finally {
      starting.value = false
    }
  }

  async function drawNumber(): Promise<void> {
    if (drawing.value) {
      return
    }
    drawing.value = true
    try {
      const delay = drawDelay.value || 0
      const data = await unwrap(api.POST('/game/draw', { body: { delay } }))
      lastDrawn.value = data.drawn
      currentGame.value = data.game
      setWinners(data.winners)
      if (delay > 0) {
        startCountdown(delay)
      } else {
        clearDrawCountdown()
        drawSent.value = false
      }
      // The half-time prompt is driven by the server (it must fire for auto draws
      // too, and consistently across every admin surface): it sends the
      // `halftime_prompt` event on the crossing draw, handled in useLiveEvents.
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      drawing.value = false
    }
  }

  /** Counts down the seconds until players get the number just drawn. */
  function startCountdown(seconds: number): void {
    clearDrawCountdown()
    drawSent.value = false
    drawCountdown.value = seconds
    drawCountdownTimer = setInterval(() => {
      if (drawCountdown.value === null) {
        return
      }
      drawCountdown.value--
      if (drawCountdown.value <= 0) {
        clearDrawCountdown()
        drawSent.value = true
        drawSentTimer = setTimeout(() => {
          drawSent.value = false
          drawSentTimer = null
        }, 3000)
      }
    }, 1000)
  }

  /**
   * Persist the chosen draw delay so it's shared: the server stores it (every
   * console reads it with the game state) and sends a `draw_delay` event so
   * other admins' selectors update live. The local `drawDelay` is already set by
   * the selector's v-model; this just propagates it.
   */
  async function persistDrawDelay(): Promise<void> {
    try {
      await unwrap(api.PATCH('/game', { body: { delay: drawDelay.value || 0 } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function clearDrawCountdown(): void {
    if (drawCountdownTimer) {
      clearInterval(drawCountdownTimer)
      drawCountdownTimer = null
    }
    if (drawSentTimer) {
      clearTimeout(drawSentTimer)
      drawSentTimer = null
    }
    drawCountdown.value = null
  }

  /** Opens the end-game modal if there are winners, else ends immediately. */
  async function endGame(): Promise<void> {
    if (!currentGame.value) {
      return
    }
    endGameGameId.value = currentGame.value.id
    if (winners.value.length > 0) {
      endGameSelectedWinners.value = [...winners.value]
      showEndGameModal.value = true
      return
    }
    await confirmEndGame([])
  }

  /**
   * Ends the game the confirm was opened on, logging the winners checked. If another
   * console ended it first, that is said instead of "Game ended"; if a new game has
   * started since, the server refuses (so the new game isn't ended with the old
   * one's winners) and the console catches up.
   */
  async function confirmEndGame(validWinnerIds: string[]): Promise<void> {
    const gameId = endGameGameId.value ?? currentGame.value?.id
    showEndGameModal.value = false
    endGameGameId.value = null
    if (gameId === undefined) {
      return
    }
    ending.value = true
    try {
      const out = await unwrap(api.POST('/game/end', { body: { gameId, validWinnerIds } }))
      clearGame()
      ui.notify(out.ended ? 'Game ended' : 'The game had already ended.', 'info')
    } catch (e) {
      const err = e as ApiError
      ui.notify(err.message, 'error')
      if (err.status === 409) {
        void loadGameState()
      }
    } finally {
      ending.value = false
    }
  }

  async function saveGameDetails(): Promise<void> {
    try {
      await unwrap(api.PATCH('/game', { body: { details: gameDetails.value } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Switches the "It's Yoever" reaction on/off for the current game. Optimistic:
   * flips the local flag immediately (the server also sends yoever_switched to
   * every board and console), reverting if the request fails.
   */
  async function setYoeverEnabled(on: boolean): Promise<void> {
    if (currentGame.value) {
      currentGame.value.yoeverEnabled = on
    }
    try {
      await unwrap(api.PATCH('/game', { body: { yoeverEnabled: on } }))
    } catch (e) {
      if (currentGame.value) {
        currentGame.value.yoeverEnabled = !on
      }
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Switches the automatic-draw loop on/off for the current game. Optimistic:
   * flips `autoEnabled` immediately (the server also sends `auto_draw` to every
   * console), reverting on failure. This is a live, game-level control - it
   * never writes back to a preset the game was started from.
   */
  async function setAutoEnabled(on: boolean): Promise<void> {
    if (currentGame.value) {
      currentGame.value.autoEnabled = on
    }
    try {
      await unwrap(api.PATCH('/game', { body: { autoEnabled: on } }))
    } catch (e) {
      if (currentGame.value) {
        currentGame.value.autoEnabled = !on
      }
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Adjusts the seconds between automatic draws for the current game (optimistic). */
  async function setAutoInterval(seconds: number): Promise<void> {
    const prev = currentGame.value?.autoInterval
    if (currentGame.value) {
      currentGame.value.autoInterval = seconds
    }
    try {
      await unwrap(api.PATCH('/game', { body: { autoInterval: seconds } }))
    } catch (e) {
      if (currentGame.value && prev !== undefined) {
        currentGame.value.autoInterval = prev
      }
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Winner verification ----------------------------------------------------

  /** Fetches a winning card and highlights cells completing the win patterns. */
  async function viewWinner(cardId: string): Promise<void> {
    if (winnerLoading.value) {
      return
    }
    winnerLoading.value = true
    try {
      const { card } = await unwrap(
        api.GET('/board/{id}', { params: { path: { id: cardId }, query: { preview: true } } }),
      )
      const calledSet = adminCalledSet.value
      const patterns = currentGame.value ? currentGame.value.patterns : []
      const matchedCells = new Set<string>()
      for (const pat of patterns) {
        const pd = pat.patternData
        let satisfied = true
        for (let r = 0; r < 5; r++) {
          for (let c = 0; c < 5; c++) {
            if (patternCell(pd, r, c)) {
              const val = boardCell(card.boardData, r, c)
              if (val !== 0 && !calledSet.has(val)) {
                satisfied = false
                break
              }
            }
          }
          if (!satisfied) {
            break
          }
        }
        if (satisfied) {
          for (let r = 0; r < 5; r++) {
            for (let c = 0; c < 5; c++) {
              if (patternCell(pd, r, c)) {
                matchedCells.add(`${r}-${c}`)
              }
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
    if (!winnerPreview.value) {
      return false
    }
    return winnerPreview.value.matchedCells.has(`${ri}-${ci}`)
  }

  // -- Halftime ---------------------------------------------------------------

  /**
   * Answers the half-time prompt for the game it was raised on. Another console
   * may have answered first: the server says so (409), and that is shown rather
   * than an error.
   */
  async function answerHalftime(minigame: boolean): Promise<void> {
    const gameId = halftimeGameId.value ?? currentGame.value?.id
    const resumed = halftimeAutoPaused.value
    closeHalftime()
    if (gameId === undefined) {
      return
    }
    try {
      await unwrap(api.POST('/game/halftime', { body: { gameId, minigame } }))
      if (minigame) {
        ui.notify('Halftime alert sent to all players!', 'success')
      } else if (resumed) {
        ui.notify('Auto-draw resumed.', 'info')
      }
    } catch (e) {
      const err = e as ApiError
      ui.notify(err.message, err.status === 409 ? 'info' : 'error')
    }
  }

  /** "Yes, run a mini-game": alert players (the server holds the alert until the
   *  triggering number has reached them, and keeps auto paused). */
  function confirmHalftime(): Promise<void> {
    return answerHalftime(true)
  }

  /** "No mini-game": don't alert players; the server resumes the auto loop if it
   *  had paused it for the prompt. Also the modal's close handler. */
  function dismissHalftime(): Promise<void> {
    return answerHalftime(false)
  }

  // -- Frequent winners -------------------------------------------------------

  async function loadFrequentWinners(): Promise<void> {
    try {
      frequentWinners.value = (await unwrap(api.GET('/winners-log/frequent'))).winners
    } catch {
      /* silent */
    }
  }

  /** Trailing-debounced reload of the frequent winners, for the live-invalidation
   *  path: a game's end on any console logs winners (one `resource_changed`), and
   *  a bulk delete sends one per row. */
  let frequentWinnersDebounce: ReturnType<typeof setTimeout> | null = null

  function refreshFrequentWinnersSoon(): void {
    if (frequentWinnersDebounce) {
      clearTimeout(frequentWinnersDebounce)
    }
    frequentWinnersDebounce = setTimeout(() => {
      frequentWinnersDebounce = null
      void loadFrequentWinners()
    }, 250)
  }

  // -- Winners log ------------------------------------------------------------

  /**
   * Coalesces overlapping loads. Every winners-log DELETE echoes back on the live
   * stream as a resource_changed, and admin.ts calls this on each one - so
   * clearing a handful of entries would start that many full re-pages at once,
   * all racing to write the same ref. Callers still get a promise that resolves
   * when the data is there; they just share one walk of the log.
   */
  let winnersLogInFlight: Promise<void> | null = null

  async function loadWinnersLog(): Promise<void> {
    if (winnersLogInFlight) {
      return winnersLogInFlight
    }
    const run = loadWinnersLogOnce()
    winnersLogInFlight = run
    try {
      await run
    } finally {
      // Cleared unconditionally, and only for OUR run - never latch a settled
      // promise here, or every later refresh returns it and silently does nothing.
      if (winnersLogInFlight === run) {
        winnersLogInFlight = null
      }
    }
  }

  /**
   * Trailing-debounced reload, for the live-invalidation path.
   *
   * The server emits one `resource_changed` PER DELETED ROW and sends it back to
   * the deleting client too, so clearing forty rows arrives as forty events.
   * The in-flight guard above only merges calls that OVERLAP - events landing
   * between loads each start another full walk of the log. Waiting for the burst
   * to stop collapses them into one reload however they are spaced.
   *
   * Deliberately not applied to loadWinnersLog itself: opening the tab should not
   * sit behind a timer.
   */
  let winnersLogDebounce: ReturnType<typeof setTimeout> | null = null

  function refreshWinnersLogSoon(): void {
    if (winnersLogDebounce) {
      clearTimeout(winnersLogDebounce)
    }
    winnersLogDebounce = setTimeout(() => {
      winnersLogDebounce = null
      void loadWinnersLog()
    }, 250)
  }

  /**
   * Loads the whole winners log so the table can sort and paginate it client-side
   * like every other admin table.
   *
   * The API caps `perPage` at 200 (backend/internal/api/winners.go), so this
   * pages through until it has `total` rows rather than requesting one huge page
   * - asking for more than the cap is refused (422).
   */
  async function loadWinnersLogOnce(): Promise<void> {
    winnersLogLoading.value = true
    try {
      const PER_PAGE = 200
      const pageAt = (page: number) =>
        unwrap(
          api.GET('/winners-log', {
            params: { query: { page, perPage: PER_PAGE, sort: 'loggedAt', dir: 'desc' } },
          }),
        )

      // Page 1 first, because only it can tell us how many there are - then the
      // rest AT ONCE. Walking them serially would make a venue with a couple of years
      // of history wait on fifteen round trips in sequence before a single row
      // paints; this is two round trips regardless of size.
      const first = await pageAt(1)
      const total = first.total || first.entries.length
      const pages = Math.ceil(total / PER_PAGE)
      const rest =
        pages > 1
          ? await Promise.all(Array.from({ length: pages - 1 }, (_, i) => pageAt(i + 2)))
          : []

      winnersLog.value = [first.entries, ...rest.map((r) => r.entries)].flat()
      winnersLogTotal.value = total
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      winnersLogLoading.value = false
    }
  }

  /** Deletes one winners-log entry on the server. */
  async function deleteWinner(id: number): Promise<void> {
    await unwrap(api.DELETE('/winners-log/{id}', { params: { path: { id } } }))
  }

  /** Deletes one winners-log entry (with confirm), then reloads the page. */
  async function deleteWinnerLogEntry(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this winners-log entry? This cannot be undone.', {
        title: 'Delete entry',
        confirmText: 'Delete',
      }))
    ) {
      return
    }
    try {
      await deleteWinner(id)
      ui.notify('Entry deleted', 'info')
      await loadWinnersLog()
      // If that emptied the list, step back a page so we don't strand the user on
      // an empty one. No second reload: the store holds the WHOLE log and the
      // table paginates it client-side, so re-fetching would return exactly what
      // was just fetched.
      if (winnersLog.value.length === 0 && winnersLogPage.value > 1) {
        winnersLogPage.value--
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Clears the entire winners log (with confirm), then reloads from page 1. */
  /** Bulk delete from the table's selection: one confirm for the whole set and
   *  a single reload afterwards, rather than a prompt and a refetch per row. */
  async function deleteWinnerLogEntries(ids: number[]): Promise<number> {
    if (ids.length === 0) {
      return 0
    }
    if (
      !(await ui.confirm(
        `Delete ${ids.length} selected entr${ids.length === 1 ? 'y' : 'ies'}? This cannot be undone.`,
        { title: 'Delete selected entries', confirmText: `Delete ${ids.length}` },
      ))
    ) {
      return 0
    }
    // Concurrently, not one after another: deleting forty selected rows would be
    // forty sequential round trips before the single reload at the end. allSettled
    // so one failure doesn't abandon the rest, and the errors are reported once
    // rather than as a stack of toasts.
    const results = await Promise.allSettled(ids.map(deleteWinner))
    const done = results.filter((r) => r.status === 'fulfilled').length
    const failed = results.length - done
    if (failed) {
      ui.notify(`${failed} entr${failed === 1 ? 'y' : 'ies'} could not be deleted`, 'error')
    }
    if (done) {
      ui.notify(`${done} entr${done === 1 ? 'y' : 'ies'} deleted`, 'info')
      await loadWinnersLog()
    }
    return done
  }

  async function deleteAllWinnersLog(): Promise<void> {
    if (
      !(await ui.confirm('Delete ALL winners-log entries? This cannot be undone.', {
        title: 'Delete all entries',
        confirmText: 'Delete all',
      }))
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/winners-log/all'))
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
    halftimeGameId,
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
    endGameGameId,
    frequentWinners,
    winnersLog,
    winnersLogTotal,
    winnersLogPage,
    winnersLogPerPage,
    adminCalledSet,
    adminGameLabel,
    isCalledAdmin,
    setWinners,
    applyState,
    takeGameEnded,
    takeDraw,
    takeHalftimePrompt,
    takeHalftimeResolved,
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
    refreshFrequentWinnersSoon,
    loadWinnersLog,
    refreshWinnersLogSoon,
    deleteWinnerLogEntry,
    deleteWinnerLogEntries,
    deleteAllWinnersLog,
  }
})
