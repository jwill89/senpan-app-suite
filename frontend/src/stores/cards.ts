/**
 * Cards store: admin card management (list, generate, delete, approve, protect,
 * preview, edit). The list is cards without their numbers; a card's numbers are read
 * on demand for the preview. The list is read again on each `cards_changed` live
 * event (`setCards`), and an open preview follows it.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, unwrap, type Card, type CardChanges, type CardSummary } from '@/api/client'
import { router } from '@/router'
import { useUiStore } from './ui'

/** A field of a card staff edit in the preview. */
export type CardField = 'playerName' | 'details'

/** The most cards one bulk generate makes (the API refuses more). */
const MAX_GENERATE = 500

export const useCardsStore = defineStore('cards', () => {
  const ui = useUiStore()

  const cards = ref<CardSummary[]>([])
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
  const previewCardEditing = ref<CardField | null>(null)
  const previewCardEditValue = ref('')
  /** The edited field's value when the edit began. A save sends the field only when
   *  what was typed differs from it, so a field this person didn't change never
   *  overwrites another admin's edit. */
  let previewEditStart = ''

  /** Page + page size for the Manage Cards table. The filter -> sort -> paginate
   *  pipeline itself lives in DataTable; the store keeps only what outlives the
   *  table (the query the toolbar binds, and the page the pager drives). */
  const cardsPage = ref(1)
  const cardsPerPage = ref(25)

  /** A card matches the search when its ID or player name contains the query.
   *  Passed to DataTable as its `filter-fn`; kept here so the search semantics
   *  stay testable independently of the table. */
  function cardMatches(c: CardSummary, q: string): boolean {
    return c.id.toLowerCase().includes(q) || c.playerName.toLowerCase().includes(q)
  }

  function closePreview(): void {
    previewCard.value = null
    previewCardEditing.value = null
  }

  /**
   * Takes in the card list, from a load (one after `cards_changed` too), and keeps an
   * open preview in step: it shows the card's current player, note and status as
   * they change, while a field being edited keeps what is typed in it. A preview
   * whose card has gone closes.
   */
  function setCards(list: CardSummary[]): void {
    cards.value = list
    const open = previewCard.value
    if (!open) {
      return
    }
    const current = list.find((c) => c.id === open.id)
    if (!current) {
      closePreview()
      ui.notify(`Card ${open.id} was deleted`, 'info')
      return
    }
    previewCard.value = { ...open, ...current }
  }

  /** Takes in a card a write answered: its list entry, and the preview showing it. */
  function takeIn(card: Card): void {
    const i = cards.value.findIndex((c) => c.id === card.id)
    if (i >= 0) {
      cards.value[i] = card
    }
    if (previewCard.value?.id === card.id) {
      previewCard.value = card
    }
  }

  async function loadCards(): Promise<void> {
    cardsLoading.value = true
    try {
      setCards((await unwrap(api.GET('/cards'))).cards)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      cardsLoading.value = false
    }
  }

  async function generateCards(): Promise<void> {
    // The API makes 1 to 500 at a time; the field is held to that rather than refused.
    const count = Math.min(MAX_GENERATE, Math.max(1, Math.floor(generateCount.value) || 1))
    generateCount.value = count
    generating.value = true
    try {
      const data = await unwrap(api.POST('/cards/generate', { body: { count } }))
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
   * list is read again on the `cards_changed` live event that follows.
   */
  async function generateSingleCard(): Promise<void> {
    const name = singleCardName.value.trim()
    if (!name) {
      ui.notify('Enter a player name for the card', 'error')
      return
    }
    generatingSingle.value = true
    try {
      const card = await unwrap(api.POST('/cards', { body: { playerName: name } }))
      singleCardName.value = ''
      const href = router.resolve({ name: 'player', params: { cardId: card.id } }).href
      ui.copyToClipboard(
        window.location.origin + href,
        `Created card ${card.id} for ${name} - link copied to clipboard`,
      )
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      generatingSingle.value = false
    }
  }

  async function deleteCard(id: string): Promise<void> {
    try {
      await unwrap(api.DELETE('/cards/{id}', { params: { path: { id } } }))
      cards.value = cards.value.filter((c) => c.id !== id)
      ui.notify('Card deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Bulk delete from the table's selection: confirms ONCE for the whole set
   *  (a per-card prompt would be unusable), then removes them together. */
  async function deleteCards(ids: string[]): Promise<number> {
    if (ids.length === 0) {
      return 0
    }
    if (
      !(await ui.confirm(
        `Delete ${ids.length} selected card${ids.length === 1 ? '' : 's'}? This cannot be undone.`,
        { title: 'Delete selected cards', confirmText: `Delete ${ids.length}` },
      ))
    ) {
      return 0
    }
    let done = 0
    for (const id of ids) {
      try {
        await unwrap(api.DELETE('/cards/{id}', { params: { path: { id } } }))
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
    ) {
      return
    }
    try {
      const data = await unwrap(api.DELETE('/cards/all'))
      // Protected cards survive, so refetch the surviving set rather than clearing.
      await loadCards()
      ui.notify(`Deleted ${data.deleted} card(s); protected cards kept`, 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Approve a pending custom card (-> approved + protected). */
  async function approveCard(id: string): Promise<void> {
    try {
      takeIn(await unwrap(api.POST('/cards/{id}/approve', { params: { path: { id } } })))
      ui.notify(`Card ${id} approved`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Mark or unmark a card as Protected (spared by Delete All). */
  async function setProtected(id: string, protectedFlag: boolean): Promise<void> {
    try {
      takeIn(
        await unwrap(
          api.POST('/cards/{id}/protect', {
            params: { path: { id } },
            body: { protected: protectedFlag },
          }),
        ),
      )
      ui.notify(protectedFlag ? `Card ${id} protected` : `Card ${id} unprotected`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Reads a single card, numbers and all, for the preview modal. */
  async function openCardPreview(id: string): Promise<void> {
    if (previewLoading.value) {
      return
    }
    previewLoading.value = true
    try {
      previewCard.value = await unwrap(api.GET('/cards/{id}', { params: { path: { id } } }))
      previewCardEditing.value = null
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      previewLoading.value = false
    }
  }

  function startPreviewCardEdit(field: CardField): void {
    previewCardEditing.value = field
    previewEditStart = previewCard.value?.[field] ?? ''
    previewCardEditValue.value = previewEditStart
  }

  /**
   * Saves the field being edited in the preview, when this person changed it. The
   * PATCH carries that field alone, so the other one keeps whatever another admin
   * saved meanwhile.
   */
  async function savePreviewCardField(field: CardField): Promise<void> {
    const card = previewCard.value
    if (!card || previewCardEditing.value !== field) {
      return
    }
    const typed = previewCardEditValue.value.trim()
    previewCardEditing.value = null
    if (typed === previewEditStart.trim()) {
      return
    }
    const body: CardChanges = field === 'playerName' ? { playerName: typed } : { details: typed }
    try {
      takeIn(await unwrap(api.PATCH('/cards/{id}', { params: { path: { id: card.id } }, body })))
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
    setCards,
    closePreview,
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
