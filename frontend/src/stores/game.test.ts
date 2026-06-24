import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Spy the halftime endpoint; the store imports the endpoint + sound layers at
// setup, so stub both (no other path here touches them).
const { triggerHalftime, setDelay } = vi.hoisted(() => ({
  triggerHalftime: vi.fn(async () => ({ ok: true })),
  setDelay: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { game: { triggerHalftime, setDelay } } }))
vi.mock('@/lib/sound', () => ({ playWinnerChime: vi.fn() }))

import { useGameStore } from './game'

beforeEach(() => {
  setActivePinia(createPinia())
  triggerHalftime.mockClear()
  setDelay.mockClear()
  vi.useFakeTimers()
})
afterEach(() => {
  vi.useRealTimers()
})

describe('confirmHalftime — tied to the draw delay', () => {
  it('sends the alert immediately when no draw delay is pending', async () => {
    const game = useGameStore()
    game.drawCountdown = null
    await game.confirmHalftime()
    expect(triggerHalftime).toHaveBeenCalledTimes(1)
  })

  it('holds the alert until the delayed number has been sent', async () => {
    const game = useGameStore()
    game.drawCountdown = 15 // 15s delay still counting down
    const done = game.confirmHalftime()

    // Not sent yet — still within the delay window.
    await vi.advanceTimersByTimeAsync(14_000)
    expect(triggerHalftime).not.toHaveBeenCalled()

    // Once the countdown elapses, the alert fires.
    await vi.advanceTimersByTimeAsync(1_000)
    await done
    expect(triggerHalftime).toHaveBeenCalledTimes(1)
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
