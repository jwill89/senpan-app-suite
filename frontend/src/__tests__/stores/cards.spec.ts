import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Card, CardSummary } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useCardsStore } from '@/stores/cards'
import { useUiStore } from '@/stores/ui'

function entry(id: string, playerName = '', details = ''): CardSummary {
  return {
    id,
    playerName,
    details,
    createdAt: null,
    protected: false,
    customStatus: '',
    world: '',
  }
}
function full(id: string, playerName = '', details = ''): Card {
  return { ...entry(id, playerName, details), boardData: [[1, 16, 31, 46, 61]] }
}

// jsdom has no navigator.clipboard; stub it so copyToClipboard (used by the
// single-card generator) can be asserted on.
const writeText = vi.fn<(text: string) => Promise<void>>(() => Promise.resolve())

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockResolvedValue(ok({ cards: [] }))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
  writeText.mockClear()
  Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
})

describe('cards cardMatches', () => {
  // The filter -> sort -> paginate pipeline itself now lives in DataTable (see
  // DataTable.spec.ts). What remains the store's business is what "matches"
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

  it('tolerates a card with no player name', () => {
    const cards = useCardsStore()
    expect(cards.cardMatches(entry('AAA111', ''), 'aaa')).toBe(true)
    expect(cards.cardMatches(entry('AAA111', ''), 'cloud')).toBe(false)
  })
})

describe('loading and making cards', () => {
  it('loadCards takes in the list', async () => {
    api.GET.mockResolvedValueOnce(ok({ cards: [entry('AAA111'), entry('BBB222')] }))
    const cards = useCardsStore()
    await cards.loadCards()
    expect(api.GET).toHaveBeenCalledWith('/cards')
    expect(cards.cards.map((c) => c.id)).toEqual(['AAA111', 'BBB222'])
  })

  it('generateCards holds the count to 1-500 rather than sending one the API refuses', async () => {
    api.POST.mockResolvedValue(ok({ cards: [], count: 500 }, 201))
    const cards = useCardsStore()
    cards.generateCount = 9999
    await cards.generateCards()
    expect(api.POST).toHaveBeenCalledWith('/cards/generate', { body: { count: 500 } })
    expect(cards.generateCount).toBe(500)
    cards.generateCount = 0
    await cards.generateCards()
    expect(api.POST).toHaveBeenLastCalledWith('/cards/generate', { body: { count: 1 } })
  })

  it('generateSingleCard does not call the API when the name is blank', async () => {
    const cards = useCardsStore()
    cards.singleCardName = '   '
    await cards.generateSingleCard()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('generateSingleCard sends the trimmed name, clears the input, and copies the card URL', async () => {
    api.POST.mockResolvedValueOnce(ok(full('NEW123', 'Aerith'), 201))
    const cards = useCardsStore()
    cards.singleCardName = '  Aerith  '
    await cards.generateSingleCard()
    expect(api.POST).toHaveBeenCalledWith('/cards', { body: { playerName: 'Aerith' } })
    expect(cards.singleCardName).toBe('')
    // The new card's playable URL is auto-copied to the clipboard.
    expect(writeText).toHaveBeenCalledTimes(1)
    expect(writeText.mock.calls[0]![0]).toContain('/play/NEW123')
  })
})

describe('cards status actions', () => {
  it('approveCard approves the card and takes in the answer', async () => {
    const cards = useCardsStore()
    cards.cards = [entry('CUST01')]
    api.POST.mockResolvedValueOnce(
      ok({ ...full('CUST01'), customStatus: 'approved', protected: true }),
    )
    await cards.approveCard('CUST01')
    expect(api.POST).toHaveBeenCalledWith('/cards/{id}/approve', {
      params: { path: { id: 'CUST01' } },
    })
    expect(cards.cards[0]).toMatchObject({ customStatus: 'approved', protected: true })
  })

  it('setProtected sends the flag and takes in the answer', async () => {
    const cards = useCardsStore()
    cards.cards = [entry('ABC123')]
    api.POST.mockResolvedValueOnce(ok({ ...full('ABC123'), protected: true }))
    await cards.setProtected('ABC123', true)
    expect(api.POST).toHaveBeenCalledWith('/cards/{id}/protect', {
      params: { path: { id: 'ABC123' } },
      body: { protected: true },
    })
    expect(cards.cards[0]!.protected).toBe(true)
  })

  it('deleteCard deletes it and drops it from the list', async () => {
    const cards = useCardsStore()
    cards.cards = [entry('AAA111'), entry('BBB222')]
    await cards.deleteCard('AAA111')
    expect(api.DELETE).toHaveBeenCalledWith('/cards/{id}', { params: { path: { id: 'AAA111' } } })
    expect(cards.cards.map((c) => c.id)).toEqual(['BBB222'])
  })

  it('deleteAllCards refetches the surviving cards instead of clearing (protected survive)', async () => {
    const cards = useCardsStore()
    useUiStore().confirm = vi.fn(async () => true)
    cards.cards = [entry('AAA111'), entry('BBB222'), entry('KEEP01')]
    api.DELETE.mockResolvedValueOnce(ok({ deleted: 2 }))
    // The server keeps the protected card; loadCards() returns only the survivor.
    api.GET.mockResolvedValueOnce(ok({ cards: [entry('KEEP01')] }))

    await cards.deleteAllCards()

    expect(api.DELETE).toHaveBeenCalledWith('/cards/all')
    expect(api.GET).toHaveBeenCalledWith('/cards') // refetched rather than blindly cleared
    expect(cards.cards.map((c) => c.id)).toEqual(['KEEP01'])
  })

  it('deleteAllCards does nothing when the confirm is cancelled', async () => {
    const cards = useCardsStore()
    useUiStore().confirm = vi.fn(async () => false)
    await cards.deleteAllCards()
    expect(api.DELETE).not.toHaveBeenCalled()
  })
})

describe('the card preview', () => {
  /** Opens the preview on a card issued to Cloud, with the note VIP. */
  async function openPreview() {
    const cards = useCardsStore()
    cards.cards = [entry('ABC123', 'Cloud', 'VIP')]
    api.GET.mockResolvedValueOnce(ok(full('ABC123', 'Cloud', 'VIP')))
    await cards.openCardPreview('ABC123')
    expect(api.GET).toHaveBeenCalledWith('/cards/{id}', { params: { path: { id: 'ABC123' } } })
    return cards
  }

  it('follows the list read again while open, keeping what is typed in the field being edited', async () => {
    const cards = await openPreview()
    cards.startPreviewCardEdit('details')
    cards.previewCardEditValue = 'Front row'

    // Another admin renames the player and rewrites the note meanwhile.
    cards.setCards([entry('ABC123', 'Tifa', 'Somebody else'), entry('ZZZ999')])

    expect(cards.previewCard).toMatchObject({ playerName: 'Tifa', details: 'Somebody else' })
    expect(cards.previewCard!.boardData).toEqual([[1, 16, 31, 46, 61]])
    expect(cards.previewCardEditing).toBe('details')
    expect(cards.previewCardEditValue).toBe('Front row')
  })

  it('closes when the list read again no longer has its card', async () => {
    const cards = await openPreview()
    cards.setCards([entry('ZZZ999')])
    expect(cards.previewCard).toBeNull()
  })

  it('saving sends only the field this person changed', async () => {
    const cards = await openPreview()
    cards.startPreviewCardEdit('playerName')
    cards.previewCardEditValue = '  Aerith '
    // Another admin's note lands while the name is being typed.
    cards.setCards([entry('ABC123', 'Cloud', 'Newer note')])
    api.PATCH.mockResolvedValueOnce(ok(full('ABC123', 'Aerith', 'Newer note')))

    await cards.savePreviewCardField('playerName')

    expect(api.PATCH).toHaveBeenCalledTimes(1)
    expect(api.PATCH).toHaveBeenCalledWith('/cards/{id}', {
      params: { path: { id: 'ABC123' } },
      body: { playerName: 'Aerith' },
    })
    expect(cards.previewCard).toMatchObject({ playerName: 'Aerith', details: 'Newer note' })
    expect(cards.cards[0]).toMatchObject({ playerName: 'Aerith', details: 'Newer note' })
  })

  it('saving a field left as it was sends nothing, even when another admin changed it', async () => {
    const cards = await openPreview()
    cards.startPreviewCardEdit('details')
    cards.setCards([entry('ABC123', 'Cloud', 'Changed elsewhere')])
    await cards.savePreviewCardField('details')
    expect(api.PATCH).not.toHaveBeenCalled()
    expect(cards.previewCard!.details).toBe('Changed elsewhere')
  })

  it('an edit cancelled with Escape is not saved by the blur that follows', async () => {
    const cards = await openPreview()
    cards.startPreviewCardEdit('playerName')
    cards.previewCardEditValue = 'Discarded'
    cards.previewCardEditing = null
    await cards.savePreviewCardField('playerName')
    expect(api.PATCH).not.toHaveBeenCalled()
  })
})
