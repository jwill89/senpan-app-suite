/**
 * Ema Plaques, admin side: the festival plaque boards (setup, windows and their
 * webhooks), the review queue for plaques designed on the site, and the artist's
 * request queue.
 *
 * Approving a site plaque is what posts it: the server sends it to the window's
 * Discord webhook with the image attached, records it as posted, and deletes the
 * stored image. The public side (submitting a plaque) lives with the festival
 * dashboard, which knows who the visitor is.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  api,
  unwrap,
  type EmaBoard,
  type EmaBoardInput,
  type EmaPlaque,
  type FestivalMap,
} from '@/api/client'
import { apiUpload } from '@/lib/api'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'
import type { EmaBoardForm, EmaWindowForm } from '@/types/api'
import { useUiStore } from './ui'

/**
 * A new window row, starting where the last one ended so days follow on, and
 * taking the same number of artist requests.
 */
function blankWindow(after?: EmaWindowForm): EmaWindowForm {
  return {
    id: 0,
    label: '',
    startAt: after?.endAt ?? '',
    endAt: '',
    webhookUrl: '',
    artistLimit: after?.artistLimit ?? 0,
    _uid: nextUid(),
  }
}

function blankForm(): EmaBoardForm {
  return {
    id: 0,
    festivalMapId: null,
    title: 'Ema Plaques',
    templateImage: '',
    instructions: '',
    discordLink: '',
    allowSelf: true,
    allowSite: true,
    allowArtist: true,
    artistName: 'Tara',
    windows: [blankWindow()],
  }
}

export const useEmaStore = defineStore('ema', () => {
  const ui = useUiStore()

  const boards = ref<EmaBoard[]>([])
  const boardsLoading = ref(false)
  const selectedBoard = ref<EmaBoard | null>(null)
  const plaques = ref<EmaPlaque[]>([])
  const plaquesLoading = ref(false)
  const boardForm = ref<EmaBoardForm | null>(null)
  const saving = ref(false)
  /** The festivals the form's picker offers. */
  const festivals = ref<FestivalMap[]>([])
  /** Plaque ids with a review (approve/reject) in flight. */
  const reviewing = ref<Set<number>>(new Set())

  const requests = ref<EmaPlaque[]>([])
  const requestsLoading = ref(false)
  /** A finished plaque is on its way to Discord. */
  const postingRequest = ref(false)

  /** Site plaques waiting for review, across every board (the sidebar count). */
  const pendingTotal = computed(() => boards.value.reduce((n, b) => n + b.pendingCount, 0))
  /** Artist requests not yet done (the sidebar count). */
  const openRequests = computed(() => requests.value.filter((r) => r.status === 'requested'))

  const pendingPlaques = computed(() =>
    plaques.value.filter(
      (p) => p.kind === 'site' && (p.status === 'pending' || p.status === 'posting'),
    ),
  )

  async function loadBoards(): Promise<void> {
    await withLoading(boardsLoading, async () => {
      try {
        boards.value = (await unwrap(api.GET('/ema-boards'))).boards
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function openBoard(id: number): Promise<void> {
    try {
      selectedBoard.value = await unwrap(api.GET('/ema-boards/{id}', { params: { path: { id } } }))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return
    }
    await loadPlaques(id)
  }

  async function loadPlaques(id: number): Promise<void> {
    await withLoading(plaquesLoading, async () => {
      try {
        plaques.value = (
          await unwrap(api.GET('/ema-boards/{id}/plaques', { params: { path: { id } } }))
        ).plaques
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function loadFestivals(): Promise<void> {
    try {
      festivals.value = (await unwrap(api.GET('/festival-maps'))).maps
    } catch {
      festivals.value = []
    }
  }

  // -- The board form -------------------------------------------------------------

  function newBoardForm(): void {
    boardForm.value = blankForm()
    void loadFestivals()
  }

  function editBoardForm(b: EmaBoard): void {
    boardForm.value = {
      id: b.id,
      festivalMapId: b.festivalMapId,
      title: b.title,
      templateImage: b.templateImage,
      instructions: b.instructions,
      discordLink: b.discordLink,
      allowSelf: b.allowSelf,
      allowSite: b.allowSite,
      allowArtist: b.allowArtist,
      artistName: b.artistName,
      windows: b.windows.map((w) => ({
        id: w.id,
        label: w.label,
        startAt: utcToDatetimeLocal(w.startAt),
        endAt: utcToDatetimeLocal(w.endAt),
        webhookUrl: w.webhookUrl,
        artistLimit: w.artistLimit,
        artistCount: w.artistCount,
        _uid: nextUid(),
      })),
    }
    void loadFestivals()
  }

  function cancelBoardForm(): void {
    boardForm.value = null
  }

  function addWindow(): void {
    const f = boardForm.value
    if (f) {
      f.windows.push(blankWindow(f.windows.at(-1)))
    }
  }

  function removeWindow(index: number): void {
    boardForm.value?.windows.splice(index, 1)
  }

  /** Saves the form. Returns the saved board's id, or null when it was refused. */
  async function saveBoard(): Promise<number | null> {
    const f = boardForm.value
    if (!f || saving.value) {
      return null
    }
    const body: EmaBoardInput = {
      festivalMapId: f.festivalMapId ?? 0,
      title: f.title,
      templateImage: f.templateImage,
      instructions: f.instructions,
      discordLink: f.discordLink,
      allowSelf: f.allowSelf,
      allowSite: f.allowSite,
      allowArtist: f.allowArtist,
      artistName: f.artistName,
      windows: f.windows.map((w) => ({
        id: w.id,
        label: w.label,
        startAt: datetimeLocalToUtc(w.startAt),
        endAt: datetimeLocalToUtc(w.endAt),
        webhookUrl: w.webhookUrl,
        artistLimit: w.artistLimit,
      })),
    }
    saving.value = true
    try {
      let id = f.id
      if (id) {
        await unwrap(api.PUT('/ema-boards/{id}', { params: { path: { id } }, body }))
      } else {
        id = (await unwrap(api.POST('/ema-boards', { body }))).id
      }
      ui.notify('Plaque board saved', 'success')
      boardForm.value = null
      await loadBoards()
      return id
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    } finally {
      saving.value = false
    }
  }

  async function setBoardStatus(b: EmaBoard, status: 'open' | 'closed'): Promise<void> {
    try {
      const params = { params: { path: { id: b.id } } }
      const changed = await unwrap(
        status === 'closed'
          ? api.POST('/ema-boards/{id}/close', params)
          : api.POST('/ema-boards/{id}/reopen', params),
      )
      if (selectedBoard.value?.id === b.id) {
        selectedBoard.value = changed
      }
      await loadBoards()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteBoard(b: EmaBoard): Promise<boolean> {
    const ok = await ui.confirm(
      `Delete "${b.title || 'this board'}"? Its days, its plaque record and any plaques still waiting for review go with it.`,
      { confirmText: 'Delete', danger: true },
    )
    if (!ok) {
      return false
    }
    try {
      await unwrap(api.DELETE('/ema-boards/{id}', { params: { path: { id: b.id } } }))
      if (selectedBoard.value?.id === b.id) {
        selectedBoard.value = null
      }
      await loadBoards()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  // -- Review -------------------------------------------------------------------

  async function review(p: EmaPlaque, action: 'approve' | 'reject'): Promise<void> {
    if (reviewing.value.has(p.id)) {
      return
    }
    if (action === 'reject') {
      const ok = await ui.confirm(
        `Reject ${p.characterName}'s plaque? Its image is deleted, and they can send another for ${p.windowLabel}.`,
        { confirmText: 'Reject', danger: true },
      )
      if (!ok) {
        return
      }
    }
    reviewing.value = new Set(reviewing.value).add(p.id)
    try {
      const params = { params: { path: { id: p.boardId, plaqueId: p.id } } }
      const res = await unwrap(
        action === 'approve'
          ? api.POST('/ema-boards/{id}/plaques/{plaqueId}/approve', params)
          : api.POST('/ema-boards/{id}/plaques/{plaqueId}/reject', params),
      )
      plaques.value = plaques.value.map((x) => (x.id === p.id ? res : x))
      ui.notify(action === 'approve' ? 'Posted to Discord' : 'Plaque rejected', 'success')
      void loadBoards()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      if (selectedBoard.value) {
        void loadPlaques(selectedBoard.value.id)
      }
    } finally {
      const next = new Set(reviewing.value)
      next.delete(p.id)
      reviewing.value = next
    }
  }

  // -- The artist's queue ---------------------------------------------------------

  async function loadRequests(): Promise<void> {
    await withLoading(requestsLoading, async () => {
      try {
        requests.value = (await unwrap(api.GET('/ema-requests'))).plaques
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function setRequestComplete(p: EmaPlaque, complete: boolean): Promise<void> {
    try {
      const params = { params: { path: { id: p.id } } }
      const res = await unwrap(
        complete
          ? api.POST('/ema-requests/{id}/complete', params)
          : api.POST('/ema-requests/{id}/reopen', params),
      )
      requests.value = requests.value.map((x) => (x.id === p.id ? res : x))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      void loadRequests()
    }
  }

  /**
   * Posts the artist's finished plaque for a request to its day's Discord channel,
   * which also marks the request complete. The server keeps no copy. Returns
   * whether it posted.
   */
  async function postRequest(p: EmaPlaque, image: File): Promise<boolean> {
    if (postingRequest.value) {
      return false
    }
    const form = new FormData()
    form.append('image', image)
    postingRequest.value = true
    try {
      const res = await apiUpload<EmaPlaque>(`ema-requests/${p.id}/post`, form)
      requests.value = requests.value.map((x) => (x.id === p.id ? res : x))
      ui.notify('Posted to Discord', 'success')
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      void loadRequests()
      return false
    } finally {
      postingRequest.value = false
    }
  }

  return {
    boards,
    boardsLoading,
    selectedBoard,
    plaques,
    plaquesLoading,
    boardForm,
    saving,
    festivals,
    reviewing,
    requests,
    requestsLoading,
    postingRequest,
    pendingTotal,
    openRequests,
    pendingPlaques,
    loadBoards,
    openBoard,
    loadPlaques,
    newBoardForm,
    editBoardForm,
    cancelBoardForm,
    addWindow,
    removeWindow,
    saveBoard,
    setBoardStatus,
    deleteBoard,
    review,
    loadRequests,
    setRequestComplete,
    postRequest,
  }
})
