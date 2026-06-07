/**
 * Player store: joining a game, the player's board + game state, manual stamps
 * (persisted to localStorage), and stamp customization (shape/color/opacity/
 * custom image). Mirrors all player-side logic from app.js.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { STAMP_COLORS, STAMP_SHAPES } from '@/lib/constants'
import type { BingoDrawnNumber, BingoGameState, Card } from '@/types/api'
import { useUiStore } from './ui'

export const usePlayerStore = defineStore('player', () => {
  const ui = useUiStore()

  const joinId = ref('')
  const joinError = ref('')
  const playerCard = ref<Card | null>(null)
  const playerGame = ref<BingoGameState | null>(null)
  /** True while joining / loading a board (drives the Join button + board load). */
  const joining = ref(false)

  // Stamps: { "r-c": true }
  const stamps = ref<Record<string, boolean>>({})

  const stampShape = ref(localStorage.getItem('bingo_stamp_shape') || 'blank')
  const stampColor = ref(localStorage.getItem('bingo_stamp_color') || 'pink')
  const stampOpacity = ref(parseFloat(localStorage.getItem('bingo_stamp_opacity') || '') || 0.8)
  // Data URL for the user-uploaded custom stamp. Persisted to localStorage so it
  // survives a page refresh (the saved stampShape can be 'custom'); falls back to
  // null if storage is unavailable.
  const customStampImage = ref<string | null>(localStorage.getItem('bingo_custom_stamp'))

  const showMinigameModal = ref(false)

  // ── Live-game feedback (ambient; never tracks the player's own board) ───────
  /** The most recently called number, for the player's "last called" banner. */
  const lastDrawn = ref<BingoDrawnNumber | null>(null)
  /** True after a game the player was watching ends (drives the end summary). */
  const gameEnded = ref(false)
  /** How many numbers were called in the game that just ended (neutral stat). */
  const endedCalledCount = ref(0)
  /** Opt-in: play a chime + vibrate when a number is called. Off by default. */
  const soundEnabled = ref(localStorage.getItem('bingo_sound_enabled') === '1')

  const stampShapes = STAMP_SHAPES
  const stampColors = STAMP_COLORS

  // ── Computed ───────────────────────────────────────────────────────────────

  /** Set of called numbers (O(1) lookup in templates). */
  const playerCalledSet = computed(() => {
    if (!playerGame.value || !playerGame.value.called_numbers) return new Set<number>()
    return new Set(playerGame.value.called_numbers)
  })

  const currentStampEmoji = computed(() => {
    const found = stampShapes.find((s) => s.id === stampShape.value)
    return found ? found.emoji : ''
  })

  const currentStampBg = computed(() => {
    const found = stampColors.find((c) => c.id === stampColor.value)
    return found ? found.value : 'rgba(229,49,112,.55)'
  })

  const stampMarkStyle = computed(() => ({
    background: currentStampBg.value,
    opacity: stampOpacity.value,
  }))

  // ── Stamp helpers ────────────────────────────────────────────────────────

  function isCalledPlayer(n: number): boolean {
    return playerCalledSet.value.has(n)
  }

  function isStamped(ri: number, ci: number): boolean {
    return stamps.value[ri + '-' + ci]
  }

  function boardCellClass(ri: number, ci: number, cell: number): (string | false)[] {
    const classes: (string | false)[] = ['board-cell']
    if (cell === 0) classes.push('free')
    if (stamps.value[ri + '-' + ci]) classes.push('stamped')
    return classes
  }

  function toggleStamp(ri: number, ci: number): void {
    const key = ri + '-' + ci
    if (stamps.value[key]) {
      delete stamps.value[key]
    } else {
      stamps.value[key] = true
    }
    stamps.value = { ...stamps.value }
    saveStamps()
  }

  function clearAllStamps(): void {
    stamps.value = {}
    saveStamps()
  }

  function saveStamps(): void {
    if (!playerCard.value || !playerGame.value) return
    const k = 'stamps_' + playerCard.value.id + '_' + playerGame.value.id
    localStorage.setItem(k, JSON.stringify(stamps.value))
  }

  function loadStamps(): void {
    if (!playerCard.value || !playerGame.value) {
      stamps.value = {}
      return
    }
    const k = 'stamps_' + playerCard.value.id + '_' + playerGame.value.id
    const raw = localStorage.getItem(k)
    stamps.value = raw ? JSON.parse(raw) : {}
  }

  function setStampShape(id: string): void {
    stampShape.value = id
    localStorage.setItem('bingo_stamp_shape', id)
  }

  function setStampColor(id: string): void {
    stampColor.value = id
    localStorage.setItem('bingo_stamp_color', id)
  }

  function setStampOpacity(val: string | number): void {
    stampOpacity.value = parseFloat(String(val))
    localStorage.setItem('bingo_stamp_opacity', String(val))
  }

  /** Toggles the opt-in draw chime/vibration and persists the choice. */
  function setSoundEnabled(on: boolean): void {
    soundEnabled.value = on
    localStorage.setItem('bingo_sound_enabled', on ? '1' : '0')
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
    stampColor,
    stampOpacity,
    customStampImage,
    showMinigameModal,
    lastDrawn,
    gameEnded,
    endedCalledCount,
    soundEnabled,
    setSoundEnabled,
    stampShapes,
    stampColors,
    playerCalledSet,
    currentStampEmoji,
    currentStampBg,
    stampMarkStyle,
    isCalledPlayer,
    isStamped,
    boardCellClass,
    toggleStamp,
    clearAllStamps,
    saveStamps,
    loadStamps,
    setStampShape,
    setStampColor,
    setStampOpacity,
    uploadCustomStamp,
    joinGame,
    resetPlayer,
    loadBoardById,
  }
})
