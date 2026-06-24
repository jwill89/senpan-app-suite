import { describe, it, expect, beforeEach } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { usePlayerStore } from './player'
import type { BingoGameState, Card } from '@/types/api'

/** Minimal Card/GameState stand-ins (only the fields the store reads). */
const fakeCard = { id: 'CARD1', board_data: [[1]] } as unknown as Card
const fakeGame = { id: 99, called_numbers: [5, 12] } as unknown as BingoGameState

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
})

describe('stamp toggling', () => {
  it('isStamped reflects toggleStamp', () => {
    const player = usePlayerStore()
    expect(player.isStamped(1, 2)).toBeFalsy()
    player.toggleStamp(1, 2)
    expect(player.isStamped(1, 2)).toBe(true)
    player.toggleStamp(1, 2)
    expect(player.isStamped(1, 2)).toBeFalsy()
  })

  it('clearAllStamps removes every stamp', () => {
    const player = usePlayerStore()
    player.toggleStamp(0, 0)
    player.toggleStamp(3, 4)
    player.clearAllStamps()
    expect(player.isStamped(0, 0)).toBeFalsy()
    expect(player.isStamped(3, 4)).toBeFalsy()
  })
})

describe('boardCellClass', () => {
  it('always includes board-cell', () => {
    const player = usePlayerStore()
    expect(player.boardCellClass(0, 1, 7)).toContain('board-cell')
  })

  it('adds free for the center (0) cell', () => {
    const player = usePlayerStore()
    expect(player.boardCellClass(2, 2, 0)).toContain('free')
  })

  it('adds stamped once the cell is stamped', () => {
    const player = usePlayerStore()
    player.toggleStamp(1, 1)
    expect(player.boardCellClass(1, 1, 9)).toContain('stamped')
  })
})

describe('stamp customization', () => {
  it('setStampEmoji selects any emoji, switches to emoji mode, and persists', () => {
    const player = usePlayerStore()
    player.setStampEmoji('🎯')
    expect(player.stampShape).toBe('emoji')
    expect(player.currentStampEmoji).toBe('🎯')
    expect(localStorage.getItem('bingo_stamp_emoji')).toBe('🎯')
    expect(localStorage.getItem('bingo_stamp_shape')).toBe('emoji')
  })

  it('blank mode shows no emoji', () => {
    const player = usePlayerStore()
    player.setStampEmoji('🎯')
    player.setStampShape('blank')
    expect(player.currentStampEmoji).toBe('')
  })

  it('custom mode shows no emoji (the image is used instead)', () => {
    const player = usePlayerStore()
    player.setStampEmoji('🎯')
    player.setStampShape('custom')
    expect(player.currentStampEmoji).toBe('')
  })

  it('migrates a legacy fixed shape id to its emoji on load', () => {
    localStorage.setItem('bingo_stamp_shape', 'heart')
    setActivePinia(createPinia())
    const player = usePlayerStore()
    expect(player.stampShape).toBe('emoji')
    expect(player.currentStampEmoji).toBe('♥️')
  })

  it('currentStampBg follows the selected color and persists', () => {
    const player = usePlayerStore()
    player.setStampColor('rgba(44, 182, 125, 0.55)')
    expect(player.currentStampBg).toBe('rgba(44, 182, 125, 0.55)')
    expect(localStorage.getItem('bingo_stamp_color')).toBe('rgba(44, 182, 125, 0.55)')
  })

  it('migrates a legacy preset-id color to its rgba value on load', () => {
    localStorage.setItem('bingo_stamp_color', 'green')
    setActivePinia(createPinia())
    expect(usePlayerStore().currentStampBg).toBe('rgba(44,182,125,.55)')
  })

  it('stampMarkStyle combines color and opacity', () => {
    const player = usePlayerStore()
    player.setStampColor('rgba(56, 128, 255, 1)')
    player.setStampOpacity(0.5)
    expect(player.stampMarkStyle).toEqual({ background: 'rgba(56, 128, 255, 1)', opacity: 0.5 })
  })
})

describe('stamp persistence', () => {
  it('saves stamps keyed by card + game id', () => {
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = fakeGame
    player.toggleStamp(2, 3)
    const stored = localStorage.getItem('stamps_CARD1_99')
    expect(stored).toBeTruthy()
    expect(JSON.parse(stored!)).toEqual({ '2-3': true })
  })

  it('loadStamps rehydrates persisted stamps for the active card/game', () => {
    localStorage.setItem('stamps_CARD1_99', JSON.stringify({ '4-4': true }))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = fakeGame
    player.loadStamps()
    expect(player.isStamped(4, 4)).toBe(true)
  })
})

describe('called-number lookup', () => {
  it('isCalledPlayer reads the active game called set', () => {
    const player = usePlayerStore()
    player.playerGame = fakeGame
    expect(player.isCalledPlayer(5)).toBe(true)
    expect(player.isCalledPlayer(7)).toBe(false)
  })
})

describe('sound preference', () => {
  it('defaults to off (opt-in) with a sensible default volume', () => {
    const player = usePlayerStore()
    expect(player.soundMode).toBe('off')
    expect(player.soundOn).toBe(false)
    expect(player.soundVolume).toBe(0.7)
  })

  it('persists the mode + volume and reads them back on a fresh store', () => {
    const player = usePlayerStore()
    player.setSoundMode('game')
    player.setSoundVolume(0.4)
    expect(localStorage.getItem('bingo_sound_mode')).toBe('game')
    expect(localStorage.getItem('bingo_sound_volume')).toBe('0.4')
    // A fresh Pinia instance should hydrate the saved preferences.
    setActivePinia(createPinia())
    const fresh = usePlayerStore()
    expect(fresh.soundMode).toBe('game')
    expect(fresh.soundOn).toBe(true)
    expect(fresh.soundVolume).toBe(0.4)
  })

  it('clamps volume to 0..1', () => {
    const player = usePlayerStore()
    player.setSoundVolume(5)
    expect(player.soundVolume).toBe(1)
    player.setSoundVolume(-1)
    expect(player.soundVolume).toBe(0)
  })

  it('migrates the legacy on/off flag to basic mode', () => {
    localStorage.setItem('bingo_sound_enabled', '1')
    setActivePinia(createPinia())
    expect(usePlayerStore().soundMode).toBe('basic')
  })
})

describe('secondary stamp', () => {
  it('is off by default with its own default colour', () => {
    const player = usePlayerStore()
    expect(player.secondaryStampEnabled).toBe(false)
    // Distinct from the primary stamp default.
    expect(player.currentSecondaryStampBg).not.toBe(player.currentStampBg)
  })

  it('persists the enabled flag and colour across a fresh store', () => {
    const player = usePlayerStore()
    player.setSecondaryStampEnabled(true)
    player.setSecondaryStampColor('rgba(1,2,3,0.5)')
    expect(localStorage.getItem('bingo_secondary_stamp_enabled')).toBe('1')
    expect(localStorage.getItem('bingo_secondary_stamp_color')).toBe('rgba(1,2,3,0.5)')
    setActivePinia(createPinia())
    const fresh = usePlayerStore()
    expect(fresh.secondaryStampEnabled).toBe(true)
    expect(fresh.secondaryStampColor).toBe('rgba(1,2,3,0.5)')
  })

  it('shares the single opacity slider value', () => {
    const player = usePlayerStore()
    player.setStampOpacity(0.5)
    expect(player.secondaryStampMarkStyle.opacity).toBe(0.5)
    expect(player.stampMarkStyle.opacity).toBe(0.5)
  })

  it('isWinningPatternCell reflects the active patterns (FREE-cell agnostic union)', () => {
    const player = usePlayerStore()
    // Pattern marks the top row (B..O at row 0) → those cells are pattern cells.
    const topRow = [true, true, true, true, true]
    const blank = [false, false, false, false, false]
    player.playerGame = {
      id: 1,
      called_numbers: [],
      patterns: [{ id: 1, name: 'Top Row', pattern_data: [topRow, blank, blank, blank, blank] }],
    } as unknown as BingoGameState
    expect(player.isWinningPatternCell(0, 0)).toBe(true)
    expect(player.isWinningPatternCell(0, 4)).toBe(true)
    expect(player.isWinningPatternCell(1, 0)).toBe(false)
    expect(player.isWinningPatternCell(2, 2)).toBe(false)
  })

  it('treats no patterns as no pattern cells', () => {
    const player = usePlayerStore()
    player.playerGame = { id: 1, called_numbers: [], patterns: [] } as unknown as BingoGameState
    expect(player.isWinningPatternCell(0, 0)).toBe(false)
  })
})

describe('live-feedback reset', () => {
  it('resetPlayer clears last-called and game-over state', () => {
    const player = usePlayerStore()
    player.lastDrawn = { number: 12, letter: 'B', call_order: 1 }
    player.gameEnded = true
    player.resetPlayer()
    expect(player.lastDrawn).toBeNull()
    expect(player.gameEnded).toBe(false)
  })
})
