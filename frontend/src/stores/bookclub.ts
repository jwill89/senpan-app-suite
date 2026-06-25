/**
 * Book club store: admin management of reading lists and their items for the
 * book clubs (Yaoi, Yuri, …). Covers list CRUD, item CRUD (manual or pulled
 * from AniList), cover-image upload, and publishing a list to Discord. Mirrors
 * the structure of the raffles store.
 *
 * One store serves every club: the active club (set via `openClub` when a club
 * tab is entered) drives which club's lists are loaded/created and the
 * per-club labels (name, curator comments label) the tab renders.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { BOOK_CLUBS } from '@/lib/constants'
import { createFreshness } from '@/lib/freshness'
import type {
  ReadingList,
  ReadingListItem,
  ReadingListItemForm,
  ReadingListSource,
} from '@/types/api'
import { useUiStore } from './ui'

/** A blank item form (used for "Add item" and reset after save). */
function emptyItemForm(): ReadingListItemForm {
  return {
    id: 0,
    cover_image: '',
    title: '',
    summary: '',
    format: '',
    genres: '',
    tropes: '',
    chapters: '',
    comments: '',
    sources: [],
  }
}

export const useBookclubStore = defineStore('bookclub', () => {
  const ui = useUiStore()

  // The currently-open club (set by openClub when its tab is entered). Drives
  // which club's lists are loaded/created and the per-club labels in the UI.
  const activeClubSlug = ref<string>(BOOK_CLUBS[0].slug)
  const activeClub = computed(
    () => BOOK_CLUBS.find((c) => c.slug === activeClubSlug.value) ?? BOOK_CLUBS[0],
  )
  const clubName = computed(() => activeClub.value.name)
  const clubIcon = computed(() => activeClub.value.icon)
  /** Label for the per-item curator comments field (e.g. "Yao's Comments"). */
  const commentsLabel = computed(() => activeClub.value.commentsLabel)

  // Per-club freshness so re-entering a club tab keeps its open list and skips a
  // redundant refetch within the TTL. Keyed `<slug>:lists`. Mutations call the
  // loaders directly, so edits still refresh.
  const clubFresh = createFreshness(30_000)

  const lists = ref<ReadingList[]>([])
  const selectedList = ref<ReadingList | null>(null)
  const newListTitle = ref('')
  const itemForm = ref<ReadingListItemForm>(emptyItemForm())
  // AniList lookup state.
  const lookupQuery = ref('')
  const lookupResults = ref<ReadingListItem[]>([])

  // In-flight flags.
  const listsLoading = ref(false)
  const detailLoading = ref(false)
  const creatingList = ref(false)
  const savingItem = ref(false)
  const coverUploading = ref(false)
  const looking = ref(false)
  const publishing = ref(false)

  // ── Lists ────────────────────────────────────────────────────────────────

  /**
   * Switch to a club (called when its tab is entered) and load its lists.
   *
   * Re-entering the *same* club within the freshness TTL keeps the open list and
   * skips the refetch (snappy revisit). Switching to a *different* club resets
   * the per-club selection and refetches its lists.
   */
  function openClub(slug: string): void {
    const switching = slug !== activeClubSlug.value
    activeClubSlug.value = slug
    if (switching) {
      closeList()
    }
    // loadLists() overwrites lists.value (its spinner covers the swap), so we
    // don't pre-clear; a different club is always stale, a quick re-entry isn't.
    if (switching || clubFresh.isStale(`${slug}:lists`)) {
      clubFresh.touch(`${slug}:lists`)
      loadLists()
    }
  }

  /**
   * Apply a live "bookclub changed" signal (another admin edited a reading list
   * or its items). If a book-club tab is open, refetch the open club's lists now
   * (and the open list's detail); otherwise drop the freshness stamp so re-entry
   * refetches. The signal isn't club-specific, so it acts on the active club —
   * any other club's staleness self-heals via the freshness TTL.
   */
  function applyExternalChange(viewing: boolean): void {
    const key = `${activeClubSlug.value}:lists`
    if (viewing) {
      loadLists()
      clubFresh.touch(key)
      if (selectedList.value) loadListDetail(selectedList.value.id)
    } else {
      clubFresh.invalidate(key)
    }
  }

  async function loadLists(): Promise<void> {
    listsLoading.value = true
    try {
      const data = await endpoints.bookclub.lists(activeClubSlug.value)
      lists.value = data.reading_lists || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      listsLoading.value = false
    }
  }

  async function loadListDetail(id: number): Promise<void> {
    detailLoading.value = true
    try {
      const data = await endpoints.bookclub.listDetail(id)
      selectedList.value = data.reading_list
      resetItemForm()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      detailLoading.value = false
    }
  }

  /** Open a list's detail view (loads items). */
  function selectList(list: ReadingList): void {
    selectedList.value = list
    resetItemForm()
    lookupResults.value = []
    lookupQuery.value = ''
    loadListDetail(list.id)
  }

  function closeList(): void {
    selectedList.value = null
  }

  async function createList(): Promise<void> {
    const title = newListTitle.value.trim()
    if (!title) {
      ui.notify('Reading list title is required', 'error')
      return
    }
    creatingList.value = true
    try {
      const data = await endpoints.bookclub.createList(title, activeClubSlug.value)
      newListTitle.value = ''
      ui.notify('Reading list created', 'success')
      await loadLists()
      if (data.reading_list) selectList(data.reading_list)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      creatingList.value = false
    }
  }

  /** Rename a reading list to `title` (the component supplies it via inline edit). */
  async function renameList(list: ReadingList, title: string): Promise<boolean> {
    const trimmed = title.trim()
    if (!trimmed || trimmed === list.title) return false
    try {
      await endpoints.bookclub.renameList(list.id, trimmed)
      list.title = trimmed
      if (selectedList.value?.id === list.id) selectedList.value.title = trimmed
      ui.notify('Reading list renamed', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  async function deleteList(list: ReadingList): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${list.title}" and all its items?`, {
        title: 'Delete reading list',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.bookclub.deleteList(list.id)
      lists.value = lists.value.filter((l) => l.id !== list.id)
      if (selectedList.value?.id === list.id) selectedList.value = null
      ui.notify('Reading list deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function publishList(list: ReadingList): Promise<void> {
    if (
      !(await ui.confirm(`Publish "${list.title}" to Discord? Each item will be posted as an embed.`, {
        title: 'Publish reading list',
        confirmText: 'Publish',
      }))
    )
      return
    publishing.value = true
    try {
      const data = await endpoints.bookclub.publish(list.id)
      ui.notify(`Published ${data.published} item(s) to Discord`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      publishing.value = false
    }
  }

  // ── Items ──────────────────────────────────────────────────────────────────

  function resetItemForm(): void {
    itemForm.value = emptyItemForm()
  }

  /** Load an existing item into the form for editing. */
  function editItem(item: ReadingListItem): void {
    itemForm.value = {
      id: item.id,
      cover_image: item.cover_image,
      title: item.title,
      summary: item.summary,
      format: item.format,
      genres: item.genres,
      tropes: item.tropes,
      chapters: item.chapters,
      comments: item.comments,
      sources: (item.sources || []).map((s) => ({ ...s })),
    }
  }

  function addSourceRow(): void {
    itemForm.value.sources.push({ title: '', url: '' })
  }

  function removeSourceRow(index: number): void {
    itemForm.value.sources.splice(index, 1)
  }

  async function saveItem(): Promise<void> {
    if (!selectedList.value) return
    const f = itemForm.value
    if (!f.title.trim()) {
      ui.notify('Item title is required', 'error')
      return
    }
    savingItem.value = true
    try {
      await endpoints.bookclub.saveItem(selectedList.value.id, {
        id: f.id || undefined,
        cover_image: f.cover_image,
        title: f.title.trim(),
        summary: f.summary,
        format: f.format,
        genres: f.genres,
        tropes: f.tropes,
        chapters: f.chapters,
        comments: f.comments,
        sources: f.sources.filter((s) => s.url.trim()),
      })
      ui.notify(f.id ? 'Item updated' : 'Item added', 'success')
      await loadListDetail(selectedList.value.id)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingItem.value = false
    }
  }

  async function deleteItem(item: ReadingListItem): Promise<void> {
    if (!selectedList.value) return
    if (!(await ui.confirm(`Delete "${item.title}"?`, { title: 'Delete item', confirmText: 'Delete' })))
      return
    try {
      await endpoints.bookclub.deleteItem(selectedList.value.id, item.id)
      ui.notify('Item deleted', 'info')
      await loadListDetail(selectedList.value.id)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function uploadCover(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement
    const file = input.files && input.files[0]
    if (!file) return
    coverUploading.value = true
    try {
      const formData = new FormData()
      formData.append('image', file)
      const data = await endpoints.bookclub.uploadImage(formData)
      itemForm.value.cover_image = data.url
      ui.notify('Cover uploaded', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      coverUploading.value = false
      input.value = ''
    }
  }

  // ── AniList lookup ─────────────────────────────────────────────────────────

  async function runLookup(): Promise<void> {
    const q = lookupQuery.value.trim()
    if (!q) return
    looking.value = true
    lookupResults.value = []
    try {
      const data = await endpoints.bookclub.lookup(q)
      lookupResults.value = data.results || []
      if (!lookupResults.value.length) ui.notify('No matches found', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      looking.value = false
    }
  }

  /** Fill the item form from a chosen AniList result (keeps any existing id). */
  function applyLookupResult(result: ReadingListItem): void {
    const sources: ReadingListSource[] = (result.sources || []).map((s) => ({ ...s }))
    itemForm.value = {
      id: itemForm.value.id,
      cover_image: result.cover_image || '',
      title: result.title || '',
      summary: result.summary || '',
      format: result.format || '',
      genres: result.genres || '',
      // AniList has no "tropes" concept — keep whatever the admin typed.
      tropes: itemForm.value.tropes,
      chapters: result.chapters || '',
      comments: itemForm.value.comments,
      sources,
    }
    lookupResults.value = []
    lookupQuery.value = ''
    ui.notify('Form filled from AniList — review and submit', 'info')
  }

  return {
    activeClubSlug,
    clubName,
    clubIcon,
    commentsLabel,
    lists,
    selectedList,
    newListTitle,
    itemForm,
    lookupQuery,
    lookupResults,
    listsLoading,
    detailLoading,
    creatingList,
    savingItem,
    coverUploading,
    looking,
    publishing,
    openClub,
    applyExternalChange,
    loadLists,
    loadListDetail,
    selectList,
    closeList,
    createList,
    renameList,
    deleteList,
    publishList,
    resetItemForm,
    editItem,
    addSourceRow,
    removeSourceRow,
    saveItem,
    deleteItem,
    uploadCover,
    runLookup,
    applyLookupResult,
  }
})
