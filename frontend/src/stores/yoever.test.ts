import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { ApiError } from '@/lib/api'
import type { BingoGameState, Card } from '@/types/api'

// Mock the endpoint layer so triggerYoever doesn't hit the network, and the sound
// lib so importing the player/game stores doesn't touch the Web Audio API. The
// mock fn is created via vi.hoisted so it exists when the hoisted vi.mock runs.
const { yoeverEndpoint } = vi.hoisted(() => ({ yoeverEndpoint: vi.fn() }))
vi.mock('@/lib/endpoints', () => ({ endpoints: { game: { yoever: yoeverEndpoint } } }))
vi.mock('@/lib/sound', () => ({ setSoundVolume: vi.fn(), playWinnerChime: vi.fn() }))

import { useYoeverStore, YOEVER_DURATION_MS } from './yoever'
import { usePlayerStore } from './player'

const fakeCard = { id: 'CARD1', board_data: [[1]] } as unknown as Card
function enabledGame(): BingoGameState {
  return {
    id: 1,
    called_numbers: [],
    patterns: [],
    yoever_enabled: true,
    yoever_count: 0,
  } as unknown as BingoGameState
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  yoeverEndpoint.mockReset()
})

describe('yoever store', () => {
  it('persists the mute preference', () => {
    const y = useYoeverStore()
    expect(y.muted).toBe(false)
    y.setMuted(true)
    expect(y.muted).toBe(true)
    expect(localStorage.getItem('bingo_yoever_muted')).toBe('1')
  })

  it('defaults the sound to enabled and persists the toggle', () => {
    const y = useYoeverStore()
    expect(y.soundEnabled).toBe(true)
    y.setSoundEnabled(false)
    expect(y.soundEnabled).toBe(false)
    expect(localStorage.getItem('bingo_yoever_sound')).toBe('0')
  })

  it('couples the sound to the show-effects master toggle', () => {
    const y = useYoeverStore()
    // Both on by default.
    expect(y.muted).toBe(false)
    expect(y.soundEnabled).toBe(true)

    // Hiding effects also disables the sound...
    y.toggleShowEffects()
    expect(y.muted).toBe(true)
    expect(y.soundEnabled).toBe(false)

    // ...and the sound cannot be toggled back on while effects are hidden.
    y.toggleSound()
    expect(y.soundEnabled).toBe(false)

    // Showing effects again re-enables the sound.
    y.toggleShowEffects()
    expect(y.muted).toBe(false)
    expect(y.soundEnabled).toBe(true)

    // While shown, the sound is independently toggleable.
    y.toggleSound()
    expect(y.muted).toBe(false)
    expect(y.soundEnabled).toBe(false)
  })

  it('show() adds a reaction and auto-removes it after the animation', () => {
    vi.useFakeTimers()
    const y = useYoeverStore()
    y.show('Tifa')
    expect(y.active).toHaveLength(1)
    expect(y.active[0].name).toBe('Tifa')
    vi.advanceTimersByTime(YOEVER_DURATION_MS + 10)
    expect(y.active).toHaveLength(0)
    vi.useRealTimers()
  })

  it('show() is a no-op while muted', () => {
    const y = useYoeverStore()
    y.setMuted(true)
    y.show('Tifa')
    expect(y.active).toHaveLength(0)
  })

  it('caps the number of concurrent reactions', () => {
    vi.useFakeTimers()
    const y = useYoeverStore()
    for (let i = 0; i < 20; i++) y.show(`P${i}`)
    expect(y.active.length).toBeLessThanOrEqual(8)
    vi.useRealTimers()
  })
})

describe('player.triggerYoever', () => {
  it('does nothing when the reaction is not enabled', async () => {
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = { ...enabledGame(), yoever_enabled: false }
    expect(await player.triggerYoever()).toBe(false)
    expect(yoeverEndpoint).not.toHaveBeenCalled()
  })

  it('triggers with the card id and arms the cooldown on success', async () => {
    yoeverEndpoint.mockResolvedValue({ ok: true, count: 1, cooldown_seconds: 180 })
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(true)
    expect(yoeverEndpoint).toHaveBeenCalledWith('CARD1')
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before)
  })

  it('is blocked while still on cooldown', async () => {
    yoeverEndpoint.mockResolvedValue({ ok: true, count: 1, cooldown_seconds: 180 })
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    await player.triggerYoever()
    yoeverEndpoint.mockClear()
    expect(await player.triggerYoever()).toBe(false)
    expect(yoeverEndpoint).not.toHaveBeenCalled()
  })

  it('re-arms the cooldown from the server retry_after on a 429', async () => {
    yoeverEndpoint.mockRejectedValue(new ApiError('too soon', 429, { retry_after: 30 }))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(false)
    // Armed from retry_after (~30s), NOT the 180s hint default.
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before)
    expect(player.yoeverCooldownUntil).toBeLessThan(before + 60_000)
  })

  it('falls back to the cooldown hint on a 429 with no retry_after', async () => {
    yoeverEndpoint.mockRejectedValue(new ApiError('too soon', 429))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(false)
    // No retry_after → the 180s hint default keeps the button disabled.
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before + 120_000)
  })
})
