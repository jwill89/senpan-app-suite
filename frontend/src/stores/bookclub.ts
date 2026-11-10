/**
 * Book club store: admin management of reading lists and their items for the
 * book clubs (Yaoi, Yuri, ...). Covers list CRUD, item CRUD (manual or pulled
 * from AniList), cover-image upload, the club's Discord webhook, and publishing a
 * list to Discord. Mirrors the structure of the raffles store.
 *
 * One store serves every club: the active club (set via `openClub` when a club
 * tab is entered) drives which club's lists are loaded/created and the
 * per-club labels (name, curator comments label) the tab renders.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  api,
  unwrap,
  type ReadingList,
  type ReadingListItem,
  type ReadingListItemInput,
  type ReadingListSource,
  type ReadingListSummary,
} from '@/api/client'
import { endpoints } from '@/lib/endpoints'
import { BOOK_CLUBS } from '@/lib/constants'
import { createFreshness } from '@/lib/freshness'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'
import { useUiStore } from './ui'

/**
 * A reading-list source row in the item form. `_uid` is a client-only stable key
 * for the repeater (see lib/uid.ts); it's stripped before the payload is sent.
 */
export interface ReadingListSourceForm extends ReadingListSource {
  _uid?: number
}

/** The editable form model for a reading-list item (id 0 = new). */
export interface ReadingListItemForm extends Omit<ReadingListItemInput, 'sources'> {
  id: number
  sources: ReadingListSourceForm[]
}

/** A blank item form (used for "Add item" and reset after save). */
function emptyItemForm(): ReadingListItemForm {
  return {
    id: 0,
    coverImage: '',
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

  const lists = ref<ReadingListSummary[]>([])
  const selectedList = ref<ReadingList | null>(null)
  const newListTitle = ref('')
  const itemForm = ref<ReadingListItemForm>(emptyItemForm())
  /** The active club's Discord webhook ('' = none). */
  const webhookUrl = ref('')
  // AniList lookup state.
  const lookupQuery = ref('')
  const lookupResults = ref<ReadingListItemInput[]>([])

  // In-flight flags.
  const listsLoading = ref(false)
  const detailLoading = ref(false)
  // Monotonic token guarding loadListDetail against a last-write-wins race
  // (a slow earlier open overwriting a newer one). Only the latest request applies.
  let detailSeq = 0
  const creatingList = ref(false)
  const savingItem = ref(false)
  const savingWebhook = ref(false)
  const coverUploading = ref(false)
  const looking = ref(false)
  const publishing = ref(false)

  /** The path parameters of the active club's calls. */
  function club(): { club: string } {
    return { club: activeClubSlug.value }
  }

  // -- Lists ----------------------------------------------------------------

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
      void loadLists()
    }
  }

  /**
   * Applies a live "club changed" event (another admin edited one of club `slug`'s
   * reading lists or its items). When that club's tab is open, its lists are read
   * again now (and the open list's detail); otherwise its freshness stamp is
   * dropped so entering it reads afresh.
   */
  function applyExternalChange(slug: string, viewing: boolean): void {
    const key = `${slug}:lists`
    if (viewing && slug === activeClubSlug.value) {
      void loadLists()
      clubFresh.touch(key)
      if (selectedList.value) {
        void loadListDetail(selectedList.value.id)
      }
    } else {
      clubFresh.invalidate(key)
    }
  }

  async function loadLists(): Promise<void> {
    await withLoading(listsLoading, async () => {
      const params = { path: club() }
      const [data, hook] = await Promise.all([
        unwrap(api.GET('/book-clubs/{club}/reading-lists', { params })),
        unwrap(api.GET('/book-clubs/{club}/webhook', { params })),
      ])
      lists.value = data.readingLists
      webhookUrl.value = hook.webhookUrl
    })
  }

  async function loadListDetail(id: number): Promise<void> {
    const reqId = ++detailSeq
    detailLoading.value = true
    try {
      const data = await unwrap(
        api.GET('/book-clubs/{club}/reading-lists/{id}', { params: { path: { ...club(), id } } }),
      )
      if (reqId !== detailSeq) {
        return // a newer load superseded this one
      }
      selectedList.value = data
      resetItemForm()
    } catch (e) {
      if (reqId === detailSeq) {
        ui.notify((e as Error).message, 'error')
      }
    } finally {
      if (reqId === detailSeq) {
        detailLoading.value = false
      }
    }
  }

  /** Open a list's detail view (loads items). */
  function selectList(list: ReadingListSummary | ReadingList): void {
    selectedList.value = { ...list, items: 'items' in list ? list.items : [] }
    resetItemForm()
    lookupResults.value = []
    lookupQuery.value = ''
    void loadListDetail(list.id)
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
      const data = await unwrap(
        api.POST('/book-clubs/{club}/reading-lists', { params: { path: club() }, body: { title } }),
      )
      newListTitle.value = ''
      ui.notify('Reading list created', 'success')
      await loadLists()
      selectList(data)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      creatingList.value = false
    }
  }

  /** Rename a reading list to `title` (the component supplies it via inline edit). */
  async function renameList(list: ReadingListSummary, title: string): Promise<boolean> {
    const trimmed = title.trim()
    if (!trimmed || trimmed === list.title) {
      return false
    }
    try {
      await unwrap(
        api.PUT('/book-clubs/{club}/reading-lists/{id}', {
          params: { path: { ...club(), id: list.id } },
          body: { title: trimmed },
        }),
      )
      list.title = trimmed
      if (selectedList.value?.id === list.id) {
        selectedList.value.title = trimmed
      }
      ui.notify('Reading list renamed', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  async function deleteList(list: ReadingListSummary): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${list.title}" and all its items?`, {
        title: 'Delete reading list',
        confirmText: 'Delete',
      }))
    ) {
      return
    }
    try {
      await unwrap(
        api.DELETE('/book-clubs/{club}/reading-lists/{id}', {
          params: { path: { ...club(), id: list.id } },
        }),
      )
      lists.value = lists.value.filter((l) => l.id !== list.id)
      if (selectedList.value?.id === list.id) {
        selectedList.value = null
      }
      ui.notify('Reading list deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function publishList(list: ReadingListSummary): Promise<void> {
    if (!webhookUrl.value.trim()) {
      ui.notify("Set this club's Discord webhook first (Webhook button).", 'error')
      return
    }
    if (
      !(await ui.confirm(
        `Publish "${list.title}" to Discord? Each item will be posted as an embed.`,
        {
          title: 'Publish reading list',
          confirmText: 'Publish',
        },
      ))
    ) {
      return
    }
    publishing.value = true
    try {
      const data = await unwrap(
        api.POST('/book-clubs/{club}/reading-lists/{id}/publish', {
          params: { path: { ...club(), id: list.id } },
        }),
      )
      ui.notify(`Published ${data.published} item(s) to Discord`, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      publishing.value = false
    }
  }

  /** Save the active club's Discord webhook. Returns true on success. */
  async function saveWebhook(url: string): Promise<boolean> {
    savingWebhook.value = true
    try {
      const data = await unwrap(
        api.PUT('/book-clubs/{club}/webhook', {
          params: { path: club() },
          body: { webhookUrl: url.trim() },
        }),
      )
      webhookUrl.value = data.webhookUrl
      ui.notify('Discord webhook saved', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingWebhook.value = false
    }
  }

  // -- Items ------------------------------------------------------------------

  function resetItemForm(): void {
    itemForm.value = emptyItemForm()
  }

  /** Load an existing item into the form for editing. */
  function editItem(item: ReadingListItem): void {
    itemForm.value = {
      id: item.id,
      coverImage: item.coverImage,
      title: item.title,
      summary: item.summary,
      format: item.format,
      genres: item.genres,
      tropes: item.tropes,
      chapters: item.chapters,
      comments: item.comments,
      sources: item.sources.map((s) => ({ ...s, _uid: nextUid() })),
    }
  }

  function addSourceRow(): void {
    itemForm.value.sources.push({ title: '', url: '', _uid: nextUid() })
  }

  function removeSourceRow(index: number): void {
    itemForm.value.sources.splice(index, 1)
  }

  async function saveItem(): Promise<void> {
    if (!selectedList.value) {
      return
    }
    const f = itemForm.value
    if (!f.title.trim()) {
      ui.notify('Item title is required', 'error')
      return
    }
    savingItem.value = true
    try {
      const listId = selectedList.value.id
      const body: ReadingListItemInput = {
        coverImage: f.coverImage,
        title: f.title.trim(),
        summary: f.summary,
        format: f.format,
        genres: f.genres,
        tropes: f.tropes,
        chapters: f.chapters,
        comments: f.comments,
        // Drop blank rows and strip the client-only `_uid` before sending.
        sources: f.sources.filter((s) => s.url.trim()).map((s) => ({ title: s.title, url: s.url })),
      }
      if (f.id) {
        await unwrap(
          api.PUT('/book-clubs/{club}/reading-lists/{id}/items/{itemId}', {
            params: { path: { ...club(), id: listId, itemId: f.id } },
            body,
          }),
        )
      } else {
        await unwrap(
          api.POST('/book-clubs/{club}/reading-lists/{id}/items', {
            params: { path: { ...club(), id: listId } },
            body,
          }),
        )
      }
      ui.notify(f.id ? 'Item updated' : 'Item added', 'success')
      await loadListDetail(listId)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingItem.value = false
    }
  }

  async function deleteItem(item: ReadingListItem): Promise<void> {
    if (!selectedList.value) {
      return
    }
    if (
      !(await ui.confirm(`Delete "${item.title}"?`, {
        title: 'Delete item',
        confirmText: 'Delete',
      }))
    ) {
      return
    }
    try {
      const listId = selectedList.value.id
      await unwrap(
        api.DELETE('/book-clubs/{club}/reading-lists/{id}/items/{itemId}', {
          params: { path: { ...club(), id: listId, itemId: item.id } },
        }),
      )
      ui.notify('Item deleted', 'info')
      await loadListDetail(listId)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function uploadCover(event: Event): Promise<void> {
    const input = event.target as HTMLInputElement
    const file = input.files?.[0]
    if (!file) {
      return
    }
    coverUploading.value = true
    try {
      const formData = new FormData()
      formData.append('image', file)
      const data = await endpoints.bookclub.uploadImage(formData)
      itemForm.value.coverImage = data.url
      ui.notify('Cover uploaded', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      coverUploading.value = false
      input.value = ''
    }
  }

  // -- AniList lookup ---------------------------------------------------------

  async function runLookup(): Promise<void> {
    const q = lookupQuery.value.trim()
    if (!q) {
      return
    }
    looking.value = true
    lookupResults.value = []
    try {
      const data = await unwrap(
        api.GET('/book-clubs/{club}/lookup', { params: { path: club(), query: { q } } }),
      )
      lookupResults.value = data.results
      if (!lookupResults.value.length) {
        ui.notify('No matches found', 'info')
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      looking.value = false
    }
  }

  /** Fill the item form from a chosen AniList result (keeps any existing id). */
  function applyLookupResult(result: ReadingListItemInput): void {
    const sources: ReadingListSourceForm[] = result.sources.map((s) => ({ ...s, _uid: nextUid() }))
    itemForm.value = {
      id: itemForm.value.id,
      coverImage: result.coverImage || '',
      title: result.title || '',
      summary: result.summary || '',
      format: result.format || '',
      genres: result.genres || '',
      // AniList has no "tropes" concept - keep whatever the admin typed.
      tropes: itemForm.value.tropes,
      chapters: result.chapters || '',
      comments: itemForm.value.comments,
      sources,
    }
    lookupResults.value = []
    lookupQuery.value = ''
    ui.notify('Form filled from AniList - review and submit', 'info')
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
    webhookUrl,
    lookupQuery,
    lookupResults,
    listsLoading,
    detailLoading,
    creatingList,
    savingItem,
    savingWebhook,
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
    saveWebhook,
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
