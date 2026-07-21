/**
 * Player store: joining a game, the player's board + game state, manual stamps
 * (persisted to localStorage), and stamp customization (shape/color/opacity/
 * custom image). Mirrors all player-side logic from app.js.
 */
import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { ApiError } from '@/lib/api'
import { STAMP_COLORS, STAMP_SHAPES } from '@/lib/constants'
import type { BingoDrawnNumber, BingoGameState, Card } from '@/types/api'
import { setSoundVolume as applySoundVolume } from '@/lib/sound'
import { useUiStore } from './ui'
import { useGameStore } from './game'

/** Player sound preference: off, synthesized "basic" beeps, or "game" effects. */
export type SoundMode = 'off' | 'basic' | 'game'

/** Reads the persisted sound mode, migrating the legacy on/off boolean. */
function readSoundMode(): SoundMode {
  const stored = localStorage.getItem('bingo_sound_mode')
  if (stored === 'off' || stored === 'basic' || stored === 'game') return stored
  // Legacy `bingo_sound_enabled` ('1' = on) → the basic beeps that were in place.
  return localStorage.getItem('bingo_sound_enabled') === '1' ? 'basic' : 'off'
}

/** Reads the persisted sound volume (0..1), defaulting to 0.7. */
function readSoundVolume(): number {
  const v = parseFloat(localStorage.getItem('bingo_sound_volume') ?? '')
  if (!Number.isFinite(v)) return 0.7
  return Math.min(1, Math.max(0, v))
}

/** Fallback stamp tint (pink @ 55% alpha) when nothing is stored. */
const DEFAULT_STAMP_COLOR = 'rgba(229,49,112,0.55)'

/** Default secondary-stamp tint — blue @ 55% alpha, distinct from the primary. */
const DEFAULT_SECONDARY_STAMP_COLOR = 'rgba(56,128,255,0.55)'

/**
 * Resolves the persisted stamp colour. New installs store a full CSS color
 * string; legacy installs stored a preset id (e.g. `"pink"`) — map those to the
 * matching preset value so returning players keep their chosen colour.
 */
function resolveStoredColor(stored: string | null): string {
  if (!stored) return DEFAULT_STAMP_COLOR
  const preset = STAMP_COLORS.find((c) => c.id === stored)
  return preset ? preset.value : stored
}

/**
 * Resolves the persisted stamp shape into a { mode, emoji } pair. New installs
 * store a mode ('blank' | 'emoji' | 'custom') plus the chosen emoji character
 * separately; legacy installs stored a fixed shape id (e.g. 'heart') — map those
 * forward to the matching emoji so a returning player keeps their stamp.
 */
function resolveStoredShape(): { mode: string; emoji: string } {
  const mode = localStorage.getItem('bingo_stamp_shape') || 'blank'
  const emoji = localStorage.getItem('bingo_stamp_emoji') || ''
  if (mode === 'custom' || mode === 'emoji') return { mode, emoji }
  if (mode === 'blank') return { mode: 'blank', emoji: '' }
  // Legacy fixed shape id (heart/star/…): map to its emoji, else fall back blank.
  const legacy = STAMP_SHAPES.find((s) => s.id === mode)
  return legacy && legacy.emoji
    ? { mode: 'emoji', emoji: legacy.emoji }
    : { mode: 'blank', emoji: '' }
}

export const usePlayerStore = defineStore('player', () => {
  const ui = useUiStore()
  const game = useGameStore()

  const joinId = ref('')
  const joinError = ref('')
  const playerCard = ref<Card | null>(null)
  const playerGame = ref<BingoGameState | null>(null)
  /** True while joining / loading a board (drives the Join button + board load). */
  const joining = ref(false)

  // Stamps: { "r-c": true }
  const stamps = ref<Record<string, boolean>>({})

  const stampShape = ref(resolveStoredShape().mode)
  /**
   * The chosen stamp emoji character when `stampShape === 'emoji'`. Picked from
   * the emoji selector and persisted separately from the mode so any emoji works
   * (not just a fixed preset list).
   */
  const stampEmoji = ref(resolveStoredShape().emoji)
  /**
   * The stamp tint as a full CSS color string *including its own alpha channel*
   * (e.g. `rgba(229,49,112,0.55)`). Chosen via the color-picker modal. Legacy
   * installs stored a preset id (e.g. `"pink"`) here — `resolveStoredColor()`
   * maps those forward so a returning player keeps their colour. This alpha is
   * deliberately separate from `stampOpacity` (below): the alpha tints only the
   * stamp's background fill, while opacity fades the whole mark (icon included).
   */
  const stampColor = ref(resolveStoredColor(localStorage.getItem('bingo_stamp_color')))
  // `?? 0.8` won't rescue a NaN and `|| 0.8` would discard a legitimately stored
  // 0, so guard on Number.isFinite: a stored 0 (fully transparent) persists, and
  // only an absent/corrupt value falls back to the default.
  const storedStampOpacity = parseFloat(localStorage.getItem('bingo_stamp_opacity') ?? '')
  const stampOpacity = ref(Number.isFinite(storedStampOpacity) ? storedStampOpacity : 0.8)
  // Data URL for the user-uploaded custom stamp. Persisted to localStorage so it
  // survives a page refresh (the saved stampShape can be 'custom'); falls back to
  // null if storage is unavailable.
  const customStampImage = ref<string | null>(localStorage.getItem('bingo_custom_stamp'))

  /**
   * Optional secondary stamp: a plain coloured circle (no emoji/custom image)
   * with its own colour. When enabled, it auto-marks cells that are NOT part of
   * any active win pattern, while the primary stamp marks the pattern cells —
   * giving players an at-a-glance view of which cells matter for the win. Shares
   * the single `stampOpacity` slider. Off by default (preserves prior behaviour).
   */
  const secondaryStampEnabled = ref(localStorage.getItem('bingo_secondary_stamp_enabled') === '1')
  const secondaryStampColor = ref(
    localStorage.getItem('bingo_secondary_stamp_color')
      ? resolveStoredColor(localStorage.getItem('bingo_secondary_stamp_color'))
      : DEFAULT_SECONDARY_STAMP_COLOR,
  )

  const showMinigameModal = ref(false)

  // ── Live-game feedback (ambient; never tracks the player's own board) ───────
  /** The most recently called number, for the player's "last called" banner. */
  const lastDrawn = ref<BingoDrawnNumber | null>(null)
  /** True after a game the player was watching ends (drives the end summary). */
  const gameEnded = ref(false)
  /** How many numbers were called in the game that just ended (neutral stat). */
  const endedCalledCount = ref(0)
  /** Opt-in sound feedback mode (off / basic beeps / game effects). Off by default. */
  const soundMode = ref<SoundMode>(readSoundMode())
  /** Master volume (0..1) for whichever sound mode is active. */
  const soundVolume = ref<number>(readSoundVolume())
  // Keep the sound lib's master volume in sync from the start.
  applySoundVolume(soundVolume.value)
  /** Whether any sound mode is enabled (drives the volume slider's enabled state). */
  const soundOn = computed(() => soundMode.value !== 'off')

  // ── "It's Yoever" reaction ──────────────────────────────────────────────────
  /** In-flight flag for the trigger button. */
  const yoeverTriggering = ref(false)
  /**
   * Epoch-ms until which this client's trigger button stays disabled. The server
   * owns the real cooldown; we mirror it locally (persisted per card+game) so the
   * button disables across a refresh, and fall back to the server 429 if it drifts.
   */
  const yoeverCooldownUntil = ref(0)
  /**
   * Last cooldown length (seconds) the server reported, used to re-arm the local
   * timer after a 429 when we have nothing fresher. Seeded to the 3-minute default.
   */
  const yoeverCooldownHint = ref(180)
  /** Whether the reaction is currently offered: admin-enabled AND a game running. */
  const yoeverEnabled = computed(() => !!playerGame.value?.yoever_enabled)

  const stampShapes = STAMP_SHAPES
  const stampColors = STAMP_COLORS

  // ── Computed ───────────────────────────────────────────────────────────────

  /** Set of called numbers (O(1) lookup in templates). */
  const playerCalledSet = computed(() => {
    if (!playerGame.value) return new Set<number>()
    return new Set(playerGame.value.called_numbers)
  })

  const currentStampEmoji = computed(() => (stampShape.value === 'emoji' ? stampEmoji.value : ''))

  const currentStampBg = computed(() => stampColor.value || DEFAULT_STAMP_COLOR)

  const stampMarkStyle = computed(() => ({
    background: currentStampBg.value,
    opacity: stampOpacity.value,
  }))

  /** Background tint of the secondary stamp (its own colour, shared opacity). */
  const currentSecondaryStampBg = computed(
    () => secondaryStampColor.value || DEFAULT_SECONDARY_STAMP_COLOR,
  )
  const secondaryStampMarkStyle = computed(() => ({
    background: currentSecondaryStampBg.value,
    opacity: stampOpacity.value,
  }))

  /**
   * Cell keys ("ri-ci") that are part of an active win pattern — the union of
   * every active pattern's required cells. Mirrors how the backend treats a cell
   * as pattern-relevant (`pattern_data[r][c] === true`). Used to route the
   * secondary stamp onto the non-pattern cells.
   */
  const winningPatternCells = computed(() => {
    const set = new Set<string>()
    for (const p of playerGame.value?.patterns ?? []) {
      const grid = p.pattern_data
      for (let r = 0; r < 5 && r < grid.length; r++) {
        const row = grid[r] ?? []
        for (let c = 0; c < 5 && c < row.length; c++) {
          if (row[c]) set.add(`${r}-${c}`)
        }
      }
    }
    return set
  })

  /**
   * Frozen snapshot of the win-pattern cell set, kept so the primary/secondary
   * stamp split survives a game ending. When a game ends the server sends
   * game=null, which empties `winningPatternCells` (it reads `playerGame`);
   * without this freeze every stamp would flip to the "non-pattern" secondary
   * stamp before the player can save their board. We mirror the live set here
   * while a game runs, and drop it when a new game starts (the game id changes)
   * or the board is cleared — exactly the moments the player expects a reset.
   */
  const frozenPatternCells = ref<Set<string> | null>(null)
  // A new game (different id) invalidates the old snapshot; the next watcher then
  // repopulates it from that game's patterns. Sync flush keeps it deterministic.
  watch(
    () => playerGame.value?.id,
    (id, prevId) => {
      if (id && id !== prevId) frozenPatternCells.value = null
    },
    { flush: 'sync' },
  )
  // Mirror the live pattern set while a game is active (non-empty). Once the game
  // ends the live set empties, so we stop updating and the last snapshot persists.
  watch(
    winningPatternCells,
    (cells) => {
      if (cells.size > 0) frozenPatternCells.value = new Set(cells)
    },
    { flush: 'sync' },
  )

  /**
   * True when cell [ri,ci] is a required cell of a win pattern. While a game is
   * live this reflects the active patterns; once the game has ended (playerGame
   * is null) it falls back to the frozen snapshot so stamps keep their type until
   * a new game starts or the board is cleared.
   */
  function isWinningPatternCell(ri: number, ci: number): boolean {
    const set =
      !playerGame.value && frozenPatternCells.value
        ? frozenPatternCells.value
        : winningPatternCells.value
    return set.has(`${ri}-${ci}`)
  }

  /**
   * Frozen copy of the game-details markdown, kept so the card-image export still
   * includes them after a game ends — game end clears the live `game.gameDetails`
   * (server sends game=null), which would otherwise leave the exported card
   * without its details if the player saves after the game is over. We mirror the
   * live details while a game is active and fall back to this once it has ended.
   */
  const frozenGameDetails = ref('')
  watch(
    () => game.gameDetails,
    (d) => {
      if (d) frozenGameDetails.value = d
    },
    { flush: 'sync' },
  )

  /**
   * The game details to bake into the exported card image: the live details
   * while a game is running, or the frozen copy once it has ended (so a player
   * saving their board after the game is over still gets the details).
   */
  const cardExportDetails = computed(() =>
    playerGame.value ? game.gameDetails : frozenGameDetails.value,
  )

  // ── Stamp helpers ────────────────────────────────────────────────────────

  function isCalledPlayer(n: number): boolean {
    return playerCalledSet.value.has(n)
  }

  function isStamped(ri: number, ci: number): boolean {
    return stamps.value[`${ri}-${ci}`]
  }

  function boardCellClass(ri: number, ci: number, cell: number): (string | false)[] {
    const classes: (string | false)[] = ['board-cell']
    if (cell === 0) classes.push('free')
    if (stamps.value[`${ri}-${ci}`]) classes.push('stamped')
    return classes
  }

  function toggleStamp(ri: number, ci: number): void {
    const key = `${ri}-${ci}`
    if (stamps.value[key]) {
      Reflect.deleteProperty(stamps.value, key)
    } else {
      stamps.value[key] = true
    }
    stamps.value = { ...stamps.value }
    saveStamps()
  }

  function clearAllStamps(): void {
    stamps.value = {}
    frozenPatternCells.value = null // clearing the board resets the pattern split
    // If no game is running (e.g. it ended), clearing the board also drops the
    // game details so they no longer linger in a subsequent card export.
    if (!playerGame.value) {
      frozenGameDetails.value = ''
      game.gameDetails = ''
    }
    saveStamps()
  }

  function saveStamps(): void {
    if (!playerCard.value || !playerGame.value) return
    const k = `stamps_${playerCard.value.id}_${playerGame.value.id}`
    // Best-effort persistence: the reactive stamps drive the board regardless, so
    // a full/unavailable quota mustn't throw out of the high-frequency toggle path.
    try {
      localStorage.setItem(k, JSON.stringify(stamps.value))
    } catch {
      /* storage unavailable — stamps still work this session, just won't persist */
    }
  }

  function loadStamps(): void {
    if (!playerCard.value || !playerGame.value) {
      stamps.value = {}
      return
    }
    const k = `stamps_${playerCard.value.id}_${playerGame.value.id}`
    const raw = localStorage.getItem(k)
    // Guard against corrupt/tampered storage so a bad value starts the board clean
    // instead of throwing during load. A literal 'null', an array, or a primitive
    // parses without throwing but isn't a usable stamp map — reject those too so
    // the board can't be handed a non-object.
    try {
      const parsed = raw ? JSON.parse(raw) : {}
      stamps.value =
        parsed && typeof parsed === 'object' && !Array.isArray(parsed)
          ? (parsed as Record<string, boolean>)
          : {}
    } catch {
      stamps.value = {}
    }
  }

  /** localStorage key for this card+game's yoever cooldown, or null if not loaded. */
  function yoeverCooldownKey(): string | null {
    if (!playerCard.value || !playerGame.value) return null
    return `yoever_cd_${playerCard.value.id}_${playerGame.value.id}`
  }

  /** Arms the local cooldown for `seconds` and persists its expiry (best-effort). */
  function setYoeverCooldown(seconds: number): void {
    if (seconds > 0) yoeverCooldownHint.value = seconds
    yoeverCooldownUntil.value = Date.now() + seconds * 1000
    const k = yoeverCooldownKey()
    if (k) {
      try {
        localStorage.setItem(k, String(yoeverCooldownUntil.value))
      } catch {
        /* storage unavailable — the timer still works this session */
      }
    }
  }

  /**
   * Restores the persisted cooldown for the current card+game (0 when none or
   * already elapsed). Called whenever the loaded game changes so a new game — for
   * which the server has cleared cooldowns — starts with the button enabled.
   */
  function loadYoeverCooldown(): void {
    const k = yoeverCooldownKey()
    if (!k) {
      yoeverCooldownUntil.value = 0
      return
    }
    const raw = localStorage.getItem(k)
    const v = raw ? parseInt(raw, 10) : 0
    yoeverCooldownUntil.value = Number.isFinite(v) && v > Date.now() ? v : 0
  }

  /**
   * Triggers the "It's Yoever" reaction for this board. The server broadcasts the
   * sound + animation to everyone (including us), so we don't play it here — we
   * just arm the local cooldown on success. Returns true if the trigger was sent.
   */
  async function triggerYoever(): Promise<boolean> {
    if (!playerCard.value || !playerGame.value || !yoeverEnabled.value) return false
    if (yoeverTriggering.value || Date.now() < yoeverCooldownUntil.value) return false
    yoeverTriggering.value = true
    try {
      const res = await endpoints.game.yoever(playerCard.value.id)
      setYoeverCooldown(res.cooldown_seconds)
      return true
    } catch (e) {
      const err = e as ApiError
      if (err.status === 429) {
        // Our local timer disagreed with the server — re-arm from the server's
        // reported retry_after (exact remaining time) when present, falling back
        // to the last-known cooldown length so we don't over-disable the button.
        const retryAfter = (err.body as { retry_after?: number } | null | undefined)?.retry_after
        setYoeverCooldown(
          typeof retryAfter === 'number' && retryAfter > 0 ? retryAfter : yoeverCooldownHint.value,
        )
        ui.notify('You just did that — give it a moment.', 'info')
      } else if (err.status === 403) {
        ui.notify("It's Yoever is switched off right now.", 'info')
      } else if (err.status === 409) {
        ui.notify('No game is currently active.', 'info')
      } else {
        ui.notify(err.message || 'Could not do that right now.', 'error')
      }
      return false
    } finally {
      yoeverTriggering.value = false
    }
  }

  /** Sets the stamp *mode* ('blank' | 'emoji' | 'custom') and persists it. */
  function setStampShape(mode: string): void {
    stampShape.value = mode
    localStorage.setItem('bingo_stamp_shape', mode)
  }

  /** Selects an arbitrary emoji as the stamp icon (switches into 'emoji' mode). */
  function setStampEmoji(emoji: string): void {
    stampEmoji.value = emoji
    localStorage.setItem('bingo_stamp_emoji', emoji)
    setStampShape('emoji')
  }

  /** Sets the stamp tint to a full CSS color string (incl. alpha) and persists it. */
  function setStampColor(value: string): void {
    stampColor.value = value
    localStorage.setItem('bingo_stamp_color', value)
  }

  /** Enables/disables the secondary (non-pattern) stamp and persists the choice. */
  function setSecondaryStampEnabled(on: boolean): void {
    secondaryStampEnabled.value = on
    localStorage.setItem('bingo_secondary_stamp_enabled', on ? '1' : '0')
  }

  /** Sets the secondary stamp colour (full CSS color string) and persists it. */
  function setSecondaryStampColor(value: string): void {
    secondaryStampColor.value = value
    localStorage.setItem('bingo_secondary_stamp_color', value)
  }

  function setStampOpacity(val: string | number): void {
    stampOpacity.value = parseFloat(String(val))
    localStorage.setItem('bingo_stamp_opacity', String(val))
  }

  /** Sets the sound mode (off / basic / game) and persists the choice. */
  function setSoundMode(mode: SoundMode): void {
    soundMode.value = mode
    localStorage.setItem('bingo_sound_mode', mode)
  }

  /** Sets the master sound volume (0..1), persists it, and applies it live. */
  function setSoundVolume(v: number): void {
    const clamped = Math.min(1, Math.max(0, v))
    soundVolume.value = clamped
    localStorage.setItem('bingo_sound_volume', String(clamped))
    applySoundVolume(clamped)
  }

  /**
   * Handles custom stamp image upload from a file input. Reads as a data URL
   * and persists it to localStorage (so it survives a refresh). Warns if the
   * image is not square, or if it's too large to store.
   */
  function uploadCustomStamp(event: Event): void {
    const input = event.target as HTMLInputElement
    const file = input.files && input.files[0]
    if (!file) return
    if (!file.type.startsWith('image/')) {
      ui.notify('Please select an image file.', 'error')
      input.value = ''
      return
    }
    const reader = new FileReader()
    reader.onload = (e) => {
      const dataUrl = e.target?.result as string
      const img = new Image()
      img.onload = () => {
        if (img.width !== img.height) {
          ui.notify(
            `Image is ${img.width}×${img.height}. Square images work best — non-square images will be stretched to fit.`,
            'error',
          )
        }
        customStampImage.value = dataUrl
        setStampShape('custom')
        saveCustomStamp(dataUrl)
      }
      img.onerror = () => ui.notify('Could not load image.', 'error')
      img.src = dataUrl
    }
    reader.readAsDataURL(file)
    input.value = ''
  }

  /**
   * Persists the custom stamp data URL to localStorage. If the image is too
   * large for the storage quota, the stamp still works for this session but
   * won't survive a refresh — warn the player rather than failing silently.
   */
  function saveCustomStamp(dataUrl: string): void {
    try {
      localStorage.setItem('bingo_custom_stamp', dataUrl)
    } catch {
      ui.notify(
        "Custom stamp is too large to save — it'll reset if you refresh. Try a smaller image.",
        'error',
      )
    }
  }

  // ── Join / leave ───────────────────────────────────────────────────────────

  /**
   * Joins a game by card ID. Returns the loaded game details string so the
   * caller (App/Player) can sync the shared game-details state.
   * On failure sets joinError and returns null.
   */
  async function joinGame(): Promise<string | null> {
    joinError.value = ''
    if (!joinId.value.trim()) return null
    joining.value = true
    try {
      const data = await endpoints.board.get(joinId.value.trim())
      playerCard.value = data.card
      playerGame.value = data.game ?? null
      loadStamps()
      loadYoeverCooldown()
      lastDrawn.value = null
      gameEnded.value = false
      return data.game_details || ''
    } catch (e) {
      joinError.value = (e as Error).message
      return null
    } finally {
      joining.value = false
    }
  }

  function resetPlayer(): void {
    playerCard.value = null
    playerGame.value = null
    stamps.value = {}
    lastDrawn.value = null
    gameEnded.value = false
    frozenPatternCells.value = null
    frozenGameDetails.value = ''
    yoeverCooldownUntil.value = 0
  }

  /**
   * Loads a board directly by card id (used when navigating to /play/:cardId,
   * e.g. on refresh or a shared link). Returns the game-details string on
   * success, or null on failure (also sets joinError). Skips the fetch if the
   * requested card is already loaded.
   */
  async function loadBoardById(id: string): Promise<string | null> {
    joinError.value = ''
    if (!id) return null
    if (playerCard.value && playerCard.value.id === id) {
      return null // already loaded for this id; caller keeps existing details
    }
    joining.value = true
    try {
      const data = await endpoints.board.get(id)
      playerCard.value = data.card
      playerGame.value = data.game ?? null
      loadStamps()
      loadYoeverCooldown()
      lastDrawn.value = null
      gameEnded.value = false
      return data.game_details || ''
    } catch (e) {
      joinError.value = (e as Error).message
      return null
    } finally {
      joining.value = false
    }
  }

  return {
    joinId,
    joinError,
    joining,
    playerCard,
    playerGame,
    stamps,
    stampShape,
    stampEmoji,
    stampColor,
    stampOpacity,
    customStampImage,
    secondaryStampEnabled,
    secondaryStampColor,
    showMinigameModal,
    lastDrawn,
    gameEnded,
    endedCalledCount,
    soundMode,
    soundVolume,
    soundOn,
    setSoundMode,
    setSoundVolume,
    yoeverTriggering,
    yoeverCooldownUntil,
    yoeverEnabled,
    triggerYoever,
    loadYoeverCooldown,
    stampShapes,
    stampColors,
    playerCalledSet,
    currentStampEmoji,
    currentStampBg,
    stampMarkStyle,
    currentSecondaryStampBg,
    secondaryStampMarkStyle,
    isWinningPatternCell,
    cardExportDetails,
    isCalledPlayer,
    isStamped,
    boardCellClass,
    toggleStamp,
    clearAllStamps,
    saveStamps,
    loadStamps,
    setStampShape,
    setStampEmoji,
    setStampColor,
    setStampOpacity,
    setSecondaryStampEnabled,
    setSecondaryStampColor,
    uploadCustomStamp,
    joinGame,
    resetPlayer,
    loadBoardById,
  }
})
