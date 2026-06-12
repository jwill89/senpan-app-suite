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
import { BOOK_CLUBS, detectTimezone } from '@/lib/constants'
import type {
  BookClubEvent,
  BookClubEventForm,
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

/** A blank event form, defaulting the timezone to the admin's detected zone. */
function emptyEventForm(): BookClubEventForm {
  return {
    id: 0,
    title: '',
    start_local: '',
    timezone: detectTimezone(),
    length_hours: 1,
    location: '',
    details: '',
    image: '',
    post_at_local: '',
  }
}

/** Which sub-view of a club tab is active. */
export type BookClubView = 'lists' | 'events'

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

  // Which sub-view of the club tab is showing: reading lists or event posts.
  const view = ref<BookClubView>('lists')

  const lists = ref<ReadingList[]>([])
  const selectedList = ref<ReadingList | null>(null)
  const newListTitle = ref('')
  const itemForm = ref<ReadingListItemForm>(emptyItemForm())
  // AniList lookup state.
  const lookupQuery = ref('')
  const lookupResults = ref<ReadingListItem[]>([])

  // Event posts state.
  const events = ref<BookClubEvent[]>([])
  const eventForm = ref<BookClubEventForm>(emptyEventForm())
  const eventImages = ref<string[]>([])

  // In-flight flags.
  const listsLoading = ref(false)
  const detailLoading = ref(false)
  const creatingList = ref(false)
  const savingItem = ref(false)
  const coverUploading = ref(false)
  const looking = ref(false)
  const publishing = ref(false)
  const eventsLoading = ref(false)
  const savingEvent = ref(false)
  const eventImageUploading = ref(false)
  const postingEventId = ref<number | null>(null)

  // ── Lists ────────────────────────────────────────────────────────────────

  /** Switch to a club (called when its tab is entered) and load its lists. */
  function openClub(slug: string): void {
    activeClubSlug.value = slug
    view.value = 'lists'
    closeList()
    lists.value = []
    events.value = []
    resetEventForm()
    loadLists()
  }

  /** Switch sub-view; lazily loads events the first time the events view opens. */
  function setView(next: BookClubView): void {
    view.value = next
    if (next === 'events' && events.value.length === 0) {
      loadEvents()
      loadEventImages()
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

  // ── Event posts ──────────────────────────────────────────────────────────

  async function loadEvents(): Promise<void> {
    eventsLoading.value = true
    try {
      const data = await endpoints.bookclubEvents.list(activeClubSlug.value)
      events.value = data.events || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      eventsLoading.value = false
    }
  }

  async function loadEventImages(): Promise<void> {
    try {
      const data = await endpoints.bookclubEvents.images()
      eventImages.value = data.images || []
    } catch {
      /* non-fatal: the picker just shows nothing */
    }
  }

  function resetEventForm(): void {
    eventForm.value = emptyEventForm()
  }

  /** Load an existing event into the form for editing. */
  function editEvent(ev: BookClubEvent): void {
    eventForm.value = {
      id: ev.id,
      title: ev.title,
      start_local: ev.start_local,
      timezone: ev.timezone || detectTimezone(),
      length_hours: ev.length_hours || 1,
      location: ev.location,
      details: ev.details,
      image: ev.image,
      post_at_local: ev.post_at_local,
    }
  }

  async function saveEvent(): Promise<void> {
    const f = eventForm.value
    if (!f.title.trim()) {
      ui.notify('Event title is required', 'error')
      return
    }
    if (!f.start_local) {
      ui.notify('Start date/time is required', 'error')
      return
    }
    if (!f.post_at_local) {
      ui.notify('"When to post" date/time is required', 'error')
      return
    }
    savingEvent.value = true
    try {
      await endpoints.bookclubEvents.save(activeClubSlug.value, { ...f, title: f.title.trim() })
      ui.notify(f.id ? 'Event updated' : 'Event scheduled', 'success')
      resetEventForm()
      await loadEvents()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingEvent.value = false
    }
  }

  async function deleteEvent(ev: BookClubEvent): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${ev.title}"?`, {
        title: 'Delete event',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.bookclubEvents.delete(ev.id)
      ui.notify('Event deleted', 'info')
      if (eventForm.value.id === ev.id) resetEventForm()
      await loadEvents()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function postEventNow(ev: BookClubEvent): Promise<void> {
    if (
      !(await ui.confirm(`Post "${ev.title}" to Discord now?`, {
        title: 'Post event now',
        confirmText: 'Post now',
      }))
    )
      return
    postingEventId.value = ev.id
    try {
      await endpoints.bookclubEvents.postNow(ev.id)
      ui.notify('Event posted to Discord', 'success')
      await loadEvents()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      postingEventId.value = null
    }
  }

  async function uploadEventImage(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement
    const file = input.files && input.files[0]
    if (!file) return
    eventImageUploading.value = true
    try {
      const formData = new FormData()
      formData.append('image', file)
      const data = await endpoints.bookclubEvents.uploadImage(formData)
      eventForm.value.image = data.url
      ui.notify('Image uploaded', 'success')
      await loadEventImages()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      eventImageUploading.value = false
      input.value = ''
    }
  }

  /** Reuse an existing uploaded image (no duplicate upload). */
  function pickEventImage(url: string): void {
    eventForm.value.image = url
  }

  return {
    activeClubSlug,
    clubName,
    clubIcon,
    commentsLabel,
    view,
    lists,
    selectedList,
    newListTitle,
    itemForm,
    lookupQuery,
    lookupResults,
    events,
    eventForm,
    eventImages,
    listsLoading,
    detailLoading,
    creatingList,
    savingItem,
    coverUploading,
    looking,
    publishing,
    eventsLoading,
    savingEvent,
    eventImageUploading,
    postingEventId,
    openClub,
    setView,
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
    loadEvents,
    loadEventImages,
    resetEventForm,
    editEvent,
    saveEvent,
    deleteEvent,
    postEventNow,
    uploadEventImage,
    pickEventImage,
  }
})
