import { describe, it, expect, beforeEach, vi } from 'vitest'
import { nextTick } from 'vue'
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

describe('cards filteredCards', () => {
  it('returns all cards when the query is blank', () => {
    const cards = useCardsStore()
    cards.cards = [entry('AAA111'), entry('BBB222')]
    expect(cards.filteredCards).toHaveLength(2)
    cards.cardSearchQuery = '   '
    expect(cards.filteredCards).toHaveLength(2)
  })

  it('matches on card id, case-insensitively', () => {
    const cards = useCardsStore()
    cards.cards = [entry('AAA111'), entry('BBB222')]
    cards.cardSearchQuery = 'bbb'
    expect(cards.filteredCards.map((c) => c.id)).toEqual(['BBB222'])
  })

  it('matches on player name', () => {
    const cards = useCardsStore()
    cards.cards = [entry('AAA111', 'Cloud'), entry('BBB222', 'Tifa')]
    cards.cardSearchQuery = 'tif'
    expect(cards.filteredCards.map((c) => c.id)).toEqual(['BBB222'])
  })

  it('returns nothing when neither id nor player name matches', () => {
    const cards = useCardsStore()
    cards.cards = [entry('AAA111', 'Cloud')]
    cards.cardSearchQuery = 'zzz'
    expect(cards.filteredCards).toHaveLength(0)
  })
})

describe('cards sorting + pagination', () => {
  it('defaults to newest-first by created_at', () => {
    const cards = useCardsStore()
    cards.cards = [
      entry('A', '', '2026-06-01 10:00:00'),
      entry('B', '', '2026-06-03 10:00:00'),
      entry('C', '', '2026-06-02 10:00:00'),
    ]
    expect(cards.sortedCards.map((c) => c.id)).toEqual(['B', 'C', 'A'])
  })

  it('cardsSetSort selects a column ascending, then toggles direction', () => {
    const cards = useCardsStore()
    cards.cards = [entry('BBB'), entry('AAA'), entry('CCC')]
    cards.cardsSetSort('id')
    expect(cards.cardsSortDir).toBe('asc')
    expect(cards.sortedCards.map((c) => c.id)).toEqual(['AAA', 'BBB', 'CCC'])
    cards.cardsSetSort('id')
    expect(cards.cardsSortDir).toBe('desc')
    expect(cards.sortedCards.map((c) => c.id)).toEqual(['CCC', 'BBB', 'AAA'])
  })

  it('paginates by perPage and clamps the page via cardsGoPage', () => {
    const cards = useCardsStore()
    cards.cards = Array.from({ length: 12 }, (_, i) => entry(`C${i}`, '', `2026-06-01 00:00:${i}`))
    cards.cardsPerPage = 5
    expect(cards.cardsTotalPages).toBe(3)
    expect(cards.pagedCards).toHaveLength(5)
    cards.cardsGoPage(3)
    expect(cards.cardsPage).toBe(3)
    expect(cards.pagedCards).toHaveLength(2) // last page remainder
    cards.cardsGoPage(99) // clamps to the last page
    expect(cards.cardsPage).toBe(3)
  })

  it('resets to page 1 when the search query changes', async () => {
    const cards = useCardsStore()
    cards.cards = Array.from({ length: 12 }, (_, i) => entry(`C${i}`))
    cards.cardsPerPage = 5
    cards.cardsGoPage(3)
    expect(cards.cardsPage).toBe(3)
    cards.cardSearchQuery = 'C1'
    await nextTick()
    expect(cards.cardsPage).toBe(1)
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
