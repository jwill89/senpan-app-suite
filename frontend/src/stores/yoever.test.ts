import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Card, Game } from '@/api/client'

// Fake only the API client's calls, so triggerYoever doesn't hit the network, and
// the sound lib so importing the player/game stores doesn't touch the Web Audio
// API. Each fake answers a call's `{ data, error, response }`.
const api = vi.hoisted(() => ({ POST: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
vi.mock('@/lib/sound', () => ({ setSoundVolume: vi.fn(), playWinnerChime: vi.fn() }))

import { useYoeverStore, YOEVER_DURATION_MS } from './yoever'
import { usePlayerStore } from './player'

/** A reaction the server took, as POST /game/yoever answers it. */
const taken = (cooldownSeconds: number) => ({
  data: { count: 1, cooldownSeconds },
  response: new Response(null, { status: 200 }),
})
/** A reaction refused for its cooldown: a 429, with Retry-After when given. */
const tooSoon = (retryAfter?: number) => ({
  error: { title: 'Too Many Requests', status: 429, detail: 'You just did that.' },
  response: new Response(null, {
    status: 429,
    headers: retryAfter ? { 'Retry-After': String(retryAfter) } : {},
  }),
})

const fakeCard = { id: 'CARD1', boardData: [[1]] } as unknown as Card
function enabledGame(): Game {
  return {
    id: 1,
    calledNumbers: [],
    patterns: [],
    yoeverEnabled: true,
    yoeverCount: 0,
  } as unknown as Game
}

beforeEach(() => {
  localStorage.clear()
  setActivePinia(createPinia())
  api.POST.mockReset()
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
    expect(y.active[0]!.name).toBe('Tifa')
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
    for (let i = 0; i < 20; i++) {
      y.show(`P${i}`)
    }
    expect(y.active.length).toBeLessThanOrEqual(8)
    vi.useRealTimers()
  })
})

describe('player.triggerYoever', () => {
  it('does nothing when the reaction is not enabled', async () => {
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = { ...enabledGame(), yoeverEnabled: false }
    expect(await player.triggerYoever()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('triggers with the card id and arms the cooldown on success', async () => {
    api.POST.mockResolvedValue(taken(180))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(true)
    expect(api.POST).toHaveBeenCalledWith('/game/yoever', { body: { cardId: 'CARD1' } })
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before + 120_000)
  })

  it('is blocked while still on cooldown', async () => {
    api.POST.mockResolvedValue(taken(180))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    await player.triggerYoever()
    api.POST.mockClear()
    expect(await player.triggerYoever()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it("re-arms the cooldown from the server's Retry-After on a 429", async () => {
    api.POST.mockResolvedValue(tooSoon(30))
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(false)
    // Armed from Retry-After (~30s), NOT the 180s hint default.
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before)
    expect(player.yoeverCooldownUntil).toBeLessThan(before + 60_000)
  })

  it('falls back to the cooldown hint on a 429 with no Retry-After', async () => {
    api.POST.mockResolvedValue(tooSoon())
    const player = usePlayerStore()
    player.playerCard = fakeCard
    player.playerGame = enabledGame()
    const before = Date.now()
    expect(await player.triggerYoever()).toBe(false)
    // No Retry-After -> the 180s hint default keeps the button disabled.
    expect(player.yoeverCooldownUntil).toBeGreaterThan(before + 120_000)
  })
})
