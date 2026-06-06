/**
 * Cards store: admin card management (list, generate, delete, preview, edit).
 * Mirrors app.js card methods. The card list is the lightweight shape
 * (id + player_name + details); board data is fetched on demand for previews.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api } from '@/lib/api'
import type { Card, CardListEntry } from '@/types/api'
import { useUiStore } from './ui'

export const useCardsStore = defineStore('cards', () => {
  const ui = useUiStore()

  const cards = ref<CardListEntry[]>([])
  const generateCount = ref(10)
  const cardSearchQuery = ref('')

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
    try {
      const data = await api<{ cards: CardListEntry[] }>('cards')
      cards.value = data.cards
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function generateCards(): Promise<void> {
    try {
      const data = await api<{ count: number }>('cards', {
        method: 'POST',
        body: { action: 'generate', count: generateCount.value },
      })
      ui.notify(`Generated ${data.count} card(s)`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteCard(id: string): Promise<void> {
    try {
      await api('cards', { method: 'POST', body: { action: 'delete', id } })
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
      await api('cards', { method: 'POST', body: { action: 'delete_all' } })
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
      const data = await api<{ card: Card }>(
        'board?id=' + encodeURIComponent(id) + '&preview=1',
      )
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
      const payload: Record<string, string> = {
        action: 'update_player',
        id: previewCard.value.id,
        player_name: field === 'player_name' ? newValue : previewCard.value.player_name || '',
        details: field === 'details' ? newValue : previewCard.value.details || '',
      }
      await api('cards', { method: 'POST', body: payload })
      previewCard.value[field] = newValue
      const card = cards.value.find((c) => c.id === previewCard.value!.id)
      if (card) {
        card.player_name = payload.player_name
        card.details = payload.details
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
