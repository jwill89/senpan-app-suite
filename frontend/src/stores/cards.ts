/**
 * Cards store: admin card management (list, generate, delete, preview, edit).
 * Mirrors app.js card methods. The card list is the lightweight shape
 * (id + player_name + details); board data is fetched on demand for previews.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { Card, CardListEntry } from '@/types/api'
import { useUiStore } from './ui'

export const useCardsStore = defineStore('cards', () => {
  const ui = useUiStore()

  const cards = ref<CardListEntry[]>([])
  const generateCount = ref(10)
  const cardSearchQuery = ref('')
  /** True while the card list is loading (drives the list spinner). */
  const cardsLoading = ref(false)
  /** True while a generate request is in flight (drives the Generate button). */
  const generating = ref(false)

  const previewCard = ref<Card | null>(null)
  const previewLoading = ref(false)
  const previewCardEditing = ref<'player_name' | 'details' | null>(null)
  const previewCardEditValue = ref('')

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
    cardsLoading,
    generating,
    previewCard,
    previewLoading,
    previewCardEditing,
    previewCardEditValue,
    filteredCards,
    loadCards,
    generateCards,
    deleteCard,
    deleteAllCards,
    openCardPreview,
    startPreviewCardEdit,
    savePreviewCardField,
  }
})
