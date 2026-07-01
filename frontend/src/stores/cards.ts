/**
 * Cards store: admin card management (list, generate, delete, preview, edit).
 * Mirrors app.js card methods. The card list is the lightweight shape
 * (id + player_name + details); board data is fetched on demand for previews.
 */
import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { router } from '@/router'
import type { Card, CardListEntry } from '@/types/api'
import { useUiStore } from './ui'

export const useCardsStore = defineStore('cards', () => {
  const ui = useUiStore()

  const cards = ref<CardListEntry[]>([])
  const generateCount = ref(10)
  const cardSearchQuery = ref('')
  /** Player name for the single-card generator (assigned on creation). */
  const singleCardName = ref('')
  /** True while the card list is loading (drives the list spinner). */
  const cardsLoading = ref(false)
  /** True while a generate request is in flight (drives the Generate button). */
  const generating = ref(false)
  /** True while a single-card generate is in flight (drives its button). */
  const generatingSingle = ref(false)

  const previewCard = ref<Card | null>(null)
  const previewLoading = ref(false)
  const previewCardEditing = ref<'player_name' | 'details' | null>(null)
  const previewCardEditValue = ref('')

  /** Sort + pagination state for the Manage Cards table (all client-side, since
   *  the full list is already in memory). */
  type CardSortKey = 'id' | 'player_name' | 'created_at'
  const cardsSortKey = ref<CardSortKey>('created_at')
  const cardsSortDir = ref<'asc' | 'desc'>('desc')
  const cardsPage = ref(1)
  const cardsPerPage = ref(25)

  /** Cards filtered by search query matching ID or player name. */
  const filteredCards = computed(() => {
    const q = cardSearchQuery.value.trim().toLowerCase()
    if (!q) return cards.value
    return cards.value.filter(
      (c) =>
        c.id.toLowerCase().includes(q) ||
        (c.player_name && c.player_name.toLowerCase().includes(q)),
    )
  })

  /** Filtered cards sorted by the active column/direction. */
  const sortedCards = computed(() => {
    const key = cardsSortKey.value
    const dir = cardsSortDir.value === 'asc' ? 1 : -1
    return [...filteredCards.value].sort((a, b) => {
      const av = (a[key] || '').toLowerCase()
      const bv = (b[key] || '').toLowerCase()
      if (av < bv) return -dir
      if (av > bv) return dir
      return 0
    })
  })

  const cardsTotalPages = computed(() =>
    Math.max(1, Math.ceil(sortedCards.value.length / cardsPerPage.value)),
  )

  /** The current page of sorted cards (page clamped to the available range). */
  const pagedCards = computed(() => {
    const page = Math.min(cardsPage.value, cardsTotalPages.value)
    const start = (page - 1) * cardsPerPage.value
    return sortedCards.value.slice(start, start + cardsPerPage.value)
  })

  /** Reset to the first page whenever the result set shape changes. */
  watch([cardSearchQuery, cardsPerPage, cardsSortKey, cardsSortDir], () => {
    cardsPage.value = 1
  })

  function cardsGoPage(page: number): void {
    cardsPage.value = Math.min(Math.max(1, page), cardsTotalPages.value)
  }

  /** Toggle direction when re-selecting the active column, else sort ascending. */
  function cardsSetSort(key: string): void {
    if (cardsSortKey.value === key) {
      cardsSortDir.value = cardsSortDir.value === 'asc' ? 'desc' : 'asc'
    } else {
      cardsSortKey.value = key as CardSortKey
      cardsSortDir.value = 'asc'
    }
  }

  async function loadCards(): Promise<void> {
    cardsLoading.value = true
    try {
      const data = await endpoints.cards.list()
      cards.value = data.cards
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      cardsLoading.value = false
    }
  }

  async function generateCards(): Promise<void> {
    generating.value = true
    try {
      const data = await endpoints.cards.generate(generateCount.value)
      ui.notify(`Generated ${data.count} card(s)`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      generating.value = false
    }
  }

  /**
   * Generates a single card already assigned to a player name, then copies the
   * card's playable URL to the clipboard so the admin can hand it straight to the
   * player. One combined toast confirms both the creation and the copy. The card
   * list refreshes via the server's `cards_update` broadcast.
   */
  async function generateSingleCard(): Promise<void> {
    const name = singleCardName.value.trim()
    if (!name) {
      ui.notify('Enter a player name for the card', 'error')
      return
    }
    generatingSingle.value = true
    try {
      const data = await endpoints.cards.create(name)
      singleCardName.value = ''
      const href = router.resolve({ name: 'player', params: { cardId: data.card.id } }).href
      ui.copyToClipboard(
        window.location.origin + href,
        `Created card ${data.card.id} for ${name} — link copied to clipboard`,
      )
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      generatingSingle.value = false
    }
  }

  async function deleteCard(id: string): Promise<void> {
    try {
      await endpoints.cards.delete(id)
      cards.value = cards.value.filter((c) => c.id !== id)
      ui.notify('Card deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteAllCards(): Promise<void> {
    if (
      !(await ui.confirm('Delete ALL cards? This cannot be undone.', {
        title: 'Delete all cards',
        confirmText: 'Delete all',
      }))
    )
      return
    try {
      await endpoints.cards.deleteAll()
      cards.value = []
      ui.notify('All cards deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Fetches a single card's board data on demand for the preview modal. */
  async function openCardPreview(id: string): Promise<void> {
    if (previewLoading.value) return
    previewLoading.value = true
    try {
      const data = await endpoints.board.get(id, { preview: true })
      previewCard.value = data.card
      previewCardEditing.value = null
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      previewLoading.value = false
    }
  }

  function startPreviewCardEdit(field: 'player_name' | 'details'): void {
    previewCardEditing.value = field
    previewCardEditValue.value = previewCard.value?.[field] || ''
  }

  /** Saves the inline-edited field on the preview card to the server. */
  async function savePreviewCardField(field: 'player_name' | 'details'): Promise<void> {
    if (!previewCard.value) return
    const newValue = previewCardEditValue.value.trim()
    const oldValue = previewCard.value[field] || ''
    previewCardEditing.value = null
    if (newValue === oldValue) return
    try {
      const playerName =
        field === 'player_name' ? newValue : previewCard.value.player_name || ''
      const details = field === 'details' ? newValue : previewCard.value.details || ''
      await endpoints.cards.updatePlayer(previewCard.value.id, playerName, details)
      previewCard.value[field] = newValue
      const card = cards.value.find((c) => c.id === previewCard.value!.id)
      if (card) {
        card.player_name = playerName
        card.details = details
      }
      ui.notify('Card updated', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    cards,
    generateCount,
    cardSearchQuery,
    singleCardName,
    cardsSortKey,
    cardsSortDir,
    cardsPage,
    cardsPerPage,
    cardsLoading,
    generating,
    generatingSingle,
    previewCard,
    previewLoading,
    previewCardEditing,
    previewCardEditValue,
    filteredCards,
    sortedCards,
    pagedCards,
    cardsTotalPages,
    cardsGoPage,
    cardsSetSort,
    loadCards,
    generateCards,
    generateSingleCard,
    deleteCard,
    deleteAllCards,
    openCardPreview,
    startPreviewCardEdit,
    savePreviewCardField,
  }
})
