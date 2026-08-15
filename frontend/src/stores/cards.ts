/**
 * Cards store: admin card management (list, generate, delete, preview, edit).
 * Mirrors app.js card methods. The card list is the lightweight shape
 * (id + player_name + details); board data is fetched on demand for previews.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
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

  /** Page + page size for the Manage Cards table. The filter -> sort -> paginate
   *  pipeline itself lives in DataTable; the store keeps only what outlives the
   *  table (the query the toolbar binds, and the page the pager drives). */
  const cardsPage = ref(1)
  const cardsPerPage = ref(25)

  /** A card matches the search when its ID or player name contains the query.
   *  Passed to DataTable as its `filter-fn`; kept here so the search semantics
   *  stay testable independently of the table. */
  function cardMatches(c: CardListEntry, q: string): boolean {
    return c.id.toLowerCase().includes(q) || c.player_name.toLowerCase().includes(q)
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
        `Created card ${data.card.id} for ${name} - link copied to clipboard`,
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

  /** Bulk delete from the table's selection: confirms ONCE for the whole set
   *  (a per-card prompt would be unusable), then removes them together. */
  async function deleteCards(ids: string[]): Promise<number> {
    if (ids.length === 0) return 0
    if (
      !(await ui.confirm(
        `Delete ${ids.length} selected card${ids.length === 1 ? '' : 's'}? This cannot be undone.`,
        { title: 'Delete selected cards', confirmText: `Delete ${ids.length}` },
      ))
    )
      return 0
    let done = 0
    for (const id of ids) {
      try {
        await endpoints.cards.delete(id)
        done++
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    }
    if (done) {
      const gone = new Set(ids)
      cards.value = cards.value.filter((c) => !gone.has(c.id))
      ui.notify(`${done} card${done === 1 ? '' : 's'} deleted`, 'info')
    }
    return done
  }

  async function deleteAllCards(): Promise<void> {
    if (
      !(await ui.confirm('Delete ALL cards? Protected cards are kept. This cannot be undone.', {
        title: 'Delete all cards',
        confirmText: 'Delete all',
      }))
    )
      return
    try {
      const data = await endpoints.cards.deleteAll()
      // Protected cards survive, so refetch the surviving set rather than clearing.
      await loadCards()
      ui.notify(`Deleted ${data.deleted} card(s); protected cards kept`, 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Approve a pending custom card (-> approved + protected). List updates via the
   *  cards_update broadcast. */
  async function approveCard(id: string): Promise<void> {
    try {
      await endpoints.cards.approve(id)
      ui.notify(`Card ${id} approved`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Mark or unmark a card as Protected (spared by Delete All). */
  async function setProtected(id: string, protectedFlag: boolean): Promise<void> {
    try {
      await endpoints.cards.setProtected(id, protectedFlag)
      ui.notify(protectedFlag ? `Card ${id} protected` : `Card ${id} unprotected`, 'success')
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
      const playerName = field === 'player_name' ? newValue : previewCard.value.player_name || ''
      const details = field === 'details' ? newValue : previewCard.value.details || ''
      await endpoints.cards.updatePlayer(previewCard.value.id, playerName, details)
      previewCard.value[field] = newValue
      const previewCardId = previewCard.value.id
      const card = cards.value.find((c) => c.id === previewCardId)
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
    cardsPage,
    cardsPerPage,
    cardsLoading,
    generating,
    generatingSingle,
    previewCard,
    previewLoading,
    previewCardEditing,
    previewCardEditValue,
    cardMatches,
    loadCards,
    generateCards,
    generateSingleCard,
    deleteCard,
    deleteCards,
    deleteAllCards,
    approveCard,
    setProtected,
    openCardPreview,
    startPreviewCardEdit,
    savePreviewCardField,
  }
})
