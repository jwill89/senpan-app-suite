import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { CardListEntry } from '@/types/api'

// Stub the endpoint layer; cards.create is spied so the create-single tests can
// assert on its calls.
const { create, approve, setProtected, deleteAll, list } = vi.hoisted(() => ({
  create: vi.fn(async (playerName: string) => ({
    count: 1,
    card: { id: 'NEW123', player_name: playerName, board_data: [] },
  })),
  approve: vi.fn(async () => ({ ok: true })),
  setProtected: vi.fn(async () => ({ ok: true })),
  deleteAll: vi.fn(async () => ({ deleted: 2 })),
  list: vi.fn(async () => ({ cards: [] as CardListEntry[] })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { cards: { create, approve, setProtected, deleteAll, list }, board: {} },
}))

import { useCardsStore } from './cards'
import { useUiStore } from './ui'

function entry(id: string, player_name = '', created_at = ''): CardListEntry {
  return { id, player_name, created_at } as CardListEntry
}

// jsdom has no navigator.clipboard; stub it so copyToClipboard (used by the
// single-card generator) can be asserted on.
const writeText = vi.fn<(text: string) => Promise<void>>(() => Promise.resolve())

beforeEach(() => {
  setActivePinia(createPinia())
  create.mockClear()
  approve.mockClear()
  setProtected.mockClear()
  deleteAll.mockClear()
  list.mockClear()
  writeText.mockClear()
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
})

describe('cards cardMatches', () => {
  // The filter -> sort -> paginate pipeline itself now lives in DataTable (see
  // DataTable.test.ts). What remains the store's business is what "matches"
  // means for a card, which is what these cover.
  it('matches on card id, case-insensitively', () => {
    const cards = useCardsStore()
    expect(cards.cardMatches(entry('BBB222'), 'bbb')).toBe(true)
    expect(cards.cardMatches(entry('AAA111'), 'bbb')).toBe(false)
  })

  it('matches on player name', () => {
    const cards = useCardsStore()
    expect(cards.cardMatches(entry('BBB222', 'Tifa'), 'tif')).toBe(true)
    expect(cards.cardMatches(entry('AAA111', 'Cloud'), 'tif')).toBe(false)
  })

  it('does not match when neither id nor player name contains the query', () => {
    const cards = useCardsStore()
    expect(cards.cardMatches(entry('AAA111', 'Cloud'), 'zzz')).toBe(false)
  })

  it('tolerates a card with no player name', () => {
    const cards = useCardsStore()
    expect(cards.cardMatches(entry('AAA111', ''), 'aaa')).toBe(true)
    expect(cards.cardMatches(entry('AAA111', ''), 'cloud')).toBe(false)
  })
})

describe('generateSingleCard', () => {
  it('does not call the endpoint when the name is blank', async () => {
    const cards = useCardsStore()
    cards.singleCardName = '   '
    await cards.generateSingleCard()
    expect(create).not.toHaveBeenCalled()
  })

  it('sends the trimmed name, clears the input, and copies the card URL', async () => {
    const cards = useCardsStore()
    cards.singleCardName = '  Aerith  '
    await cards.generateSingleCard()
    expect(create).toHaveBeenCalledWith('Aerith')
    expect(cards.singleCardName).toBe('')
    // The new card's playable URL is auto-copied to the clipboard.
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText.mock.calls[0][0]).toContain('/play/NEW123')
  })
})

describe('cards status actions', () => {
  it('approveCard calls the approve endpoint', async () => {
    const cards = useCardsStore()
    await cards.approveCard('CUST01')
    expect(approve).toHaveBeenCalledWith('CUST01')
  })

  it('setProtected calls the protect endpoint with the flag', async () => {
    const cards = useCardsStore()
    await cards.setProtected('ABC123', true)
    expect(setProtected).toHaveBeenCalledWith('ABC123', true)
    await cards.setProtected('ABC123', false)
    expect(setProtected).toHaveBeenLastCalledWith('ABC123', false)
  })

  it('deleteAllCards refetches the surviving cards instead of clearing (protected survive)', async () => {
    const cards = useCardsStore()
    useUiStore().confirm = vi.fn(async () => true)
    cards.cards = [entry('AAA111'), entry('BBB222'), entry('KEEP01')]
    // The server keeps the protected card; loadCards() returns only the survivor.
    list.mockResolvedValueOnce({ cards: [entry('KEEP01')] })

    await cards.deleteAllCards()

    expect(deleteAll).toHaveBeenCalled()
    expect(list).toHaveBeenCalled() // refetched rather than blindly cleared
    expect(cards.cards.map((c) => c.id)).toEqual(['KEEP01'])
  })

  it('deleteAllCards does nothing when the confirm is cancelled', async () => {
    const cards = useCardsStore()
    useUiStore().confirm = vi.fn(async () => false)
    await cards.deleteAllCards()
    expect(deleteAll).not.toHaveBeenCalled()
  })
})
