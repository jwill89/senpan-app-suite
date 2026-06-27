import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// Capture the dispatch callback the composable hands to WsClient, plus a mutable
// route so each test can set the player/admin view context. vi.hoisted makes
// these available inside the mock factories (which are hoisted above imports).
const h = vi.hoisted(() => ({
  captured: { onMessage: undefined as undefined | ((m: unknown) => void) },
  route: { value: { name: 'admin-game' as string } },
}))

vi.mock('@/lib/ws', () => ({
  WsClient: class {
    disconnect = vi.fn()
    connect = vi.fn()
    constructor(opts: { onMessage: (m: unknown) => void }) {
      h.captured.onMessage = opts.onMessage
    }
  },
}))
vi.mock('vue-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-router')>()),
  useRouter: () => ({ currentRoute: h.route, push: vi.fn() }),
}))
vi.mock('@/lib/sound', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/sound')>()),
  playEvent: vi.fn(),
  vibrate: vi.fn(),
}))
vi.mock('@/lib/theme', () => ({
  applyCustomCSS: vi.fn(),
  applyHeaderFont: vi.fn(),
  applyUploadedFonts: vi.fn(),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { board: { get: vi.fn() } } }))

import { useWebSocket } from './useWebSocket'
import { useAdminStore } from '@/stores/admin'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'

/** Fires a message through the captured dispatcher. */
function send(msg: Record<string, unknown>) {
  h.captured.onMessage!(msg)
}

beforeEach(() => {
  setActivePinia(createPinia())
  h.captured.onMessage = undefined
  h.route.value.name = 'admin-game'
})

describe('resource_changed dispatch (live admin invalidation)', () => {
  it('forwards to admin.refreshResource when in an admin view', () => {
    const admin = useAdminStore()
    admin.refreshResource = vi.fn()
    useWebSocket()
    send({ type: 'resource_changed', resource: 'garapons' })
    expect(admin.refreshResource).toHaveBeenCalledWith('garapons')
  })

  it('is ignored in the player view', () => {
    const admin = useAdminStore()
    admin.refreshResource = vi.fn()
    useWebSocket()
    h.route.value.name = 'player'
    send({ type: 'resource_changed', resource: 'raffles' })
    expect(admin.refreshResource).not.toHaveBeenCalled()
  })

  it('is ignored on the admin auth pages (no live data)', () => {
    const admin = useAdminStore()
    admin.refreshResource = vi.fn()
    useWebSocket()
    h.route.value.name = 'admin-login'
    send({ type: 'resource_changed', resource: 'users' })
    expect(admin.refreshResource).not.toHaveBeenCalled()
  })
})

describe('other dispatch routing', () => {
  it('details_update updates the game details regardless of view', () => {
    const game = useGameStore()
    useWebSocket()
    send({ type: 'details_update', game_details: 'GL HF' })
    expect(game.gameDetails).toBe('GL HF')
  })

  it('cards_update replaces the card list in the admin view', () => {
    const cards = useCardsStore()
    useWebSocket()
    send({ type: 'cards_update', cards: [{ id: 'A' }, { id: 'B' }] })
    expect(cards.cards).toHaveLength(2)
  })

  it('cards_update is ignored outside the admin view', () => {
    const cards = useCardsStore()
    cards.cards = []
    useWebSocket()
    h.route.value.name = 'player'
    send({ type: 'cards_update', cards: [{ id: 'A' }] })
    expect(cards.cards).toHaveLength(0)
  })

  it('draw_delay_update syncs the admin draw-delay selector and settings copy', () => {
    const game = useGameStore()
    useWebSocket()
    send({ type: 'draw_delay_update', delay: 7 })
    expect(game.drawDelay).toBe(7)
  })
})
