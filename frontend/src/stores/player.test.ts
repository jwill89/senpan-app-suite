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
  it('currentStampEmoji follows the selected shape and persists', () => {
    const player = usePlayerStore()
    player.setStampShape('star')
    expect(player.currentStampEmoji).toBe('⭐')
    expect(localStorage.getItem('bingo_stamp_shape')).toBe('star')
  })

  it('currentStampBg follows the selected color and persists', () => {
    const player = usePlayerStore()
    player.setStampColor('green')
    expect(player.currentStampBg).toBe('rgba(44,182,125,.55)')
    expect(localStorage.getItem('bingo_stamp_color')).toBe('green')
  })

  it('stampMarkStyle combines color and opacity', () => {
    const player = usePlayerStore()
    player.setStampColor('blue')
    player.setStampOpacity(0.5)
    expect(player.stampMarkStyle).toEqual({ background: 'rgba(56,128,255,.55)', opacity: 0.5 })
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

describe('draw-sound preference', () => {
  it('defaults to off (opt-in)', () => {
    const player = usePlayerStore()
    expect(player.soundEnabled).toBe(false)
  })

  it('persists when enabled and reads back on a fresh store', () => {
    usePlayerStore().setSoundEnabled(true)
    expect(localStorage.getItem('bingo_sound_enabled')).toBe('1')
    // A fresh Pinia instance should hydrate the saved preference.
    setActivePinia(createPinia())
    expect(usePlayerStore().soundEnabled).toBe(true)
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
