import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Spy the game endpoints the store touches; the store imports the endpoint +
// sound layers at setup, so stub both (no other path here touches them).
const { halftime, setDelay, setAutoEnabled, setAutoInterval } = vi.hoisted(() => ({
  halftime: vi.fn(async () => ({ ok: true })),
  setDelay: vi.fn(async () => ({ ok: true })),
  setAutoEnabled: vi.fn(async () => ({ ok: true })),
  setAutoInterval: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { game: { halftime, setDelay, setAutoEnabled, setAutoInterval } },
}))
vi.mock('@/lib/sound', () => ({ playWinnerChime: vi.fn() }))

import { useGameStore } from './game'

beforeEach(() => {
  setActivePinia(createPinia())
  halftime.mockClear()
  setDelay.mockClear()
  setAutoEnabled.mockClear()
  setAutoInterval.mockClear()
})

describe('halftime prompt — server-driven mini-game choice', () => {
  it('confirmHalftime answers "mini-game" (true) and closes the prompt', async () => {
    const game = useGameStore()
    game.showHalftimePrompt = true
    game.halftimeAutoPaused = true
    await game.confirmHalftime()
    expect(halftime).toHaveBeenCalledWith(true)
    expect(game.showHalftimePrompt).toBe(false)
    expect(game.halftimeAutoPaused).toBe(false)
  })

  it('dismissHalftime answers "no mini-game" (false) so the server can resume auto', async () => {
    const game = useGameStore()
    game.showHalftimePrompt = true
    await game.dismissHalftime()
    expect(halftime).toHaveBeenCalledWith(false)
    expect(game.showHalftimePrompt).toBe(false)
  })
})

describe('auto-draw live controls (optimistic)', () => {
  it('setAutoEnabled flips currentGame.auto_enabled and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = { auto_enabled: false, auto_interval: 30 } as never
    await game.setAutoEnabled(true)
    expect(setAutoEnabled).toHaveBeenCalledWith(true)
    expect((game.currentGame as unknown as { auto_enabled: boolean }).auto_enabled).toBe(true)
  })

  it('setAutoInterval updates currentGame.auto_interval and PATCHes the server', async () => {
    const game = useGameStore()
    game.currentGame = { auto_enabled: true, auto_interval: 30 } as never
    await game.setAutoInterval(45)
    expect(setAutoInterval).toHaveBeenCalledWith(45)
    expect((game.currentGame as unknown as { auto_interval: number }).auto_interval).toBe(45)
  })
})

describe('persistDrawDelay — shared delay control', () => {
  it('sends the current drawDelay to the server', async () => {
    const game = useGameStore()
    game.drawDelay = 15
    await game.persistDrawDelay()
    expect(setDelay).toHaveBeenCalledWith(15)
  })
})
