import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { CardListEntry } from '@/types/api'

// filteredCards is a pure computed; the store still imports the endpoint layer
// at setup, so stub it (no path here calls it).
vi.mock('@/lib/endpoints', () => ({ endpoints: { cards: {}, board: {} } }))

import { useCardsStore } from './cards'

function entry(id: string, player_name = ''): CardListEntry {
  return { id, player_name } as CardListEntry
}

beforeEach(() => {
  setActivePinia(createPinia())
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
