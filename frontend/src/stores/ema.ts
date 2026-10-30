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
import { endpoints } from '@/lib/endpoints'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'
import type { EmaBoard, EmaBoardForm, EmaPlaque, EmaWindowForm, FestivalMap } from '@/types/api'
import { useUiStore } from './ui'

/** A new window row, starting where the last one ended so days follow on. */
function blankWindow(after?: EmaWindowForm): EmaWindowForm {
  return {
    id: 0,
    label: '',
    start_at: after?.end_at ?? '',
    end_at: '',
    webhook_url: '',
    _uid: nextUid(),
  }
}

function blankForm(): EmaBoardForm {
  return {
    id: 0,
    festival_map_id: null,
    title: 'Ema Plaques',
    template_image: '',
    instructions: '',
    discord_link: '',
    allow_self: true,
    allow_site: true,
    allow_artist: true,
    artist_name: 'Tara',
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

  /** Site plaques waiting for review, across every board (the sidebar count). */
  const pendingTotal = computed(() => boards.value.reduce((n, b) => n + (b.pending_count ?? 0), 0))
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
        boards.value = (await endpoints.emaBoards.list()).boards
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function openBoard(id: number): Promise<void> {
    try {
      selectedBoard.value = (await endpoints.emaBoards.detail(id)).board
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return
    }
    await loadPlaques(id)
  }

  async function loadPlaques(id: number): Promise<void> {
    await withLoading(plaquesLoading, async () => {
      try {
        plaques.value = (await endpoints.emaBoards.plaques(id)).plaques
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function loadFestivals(): Promise<void> {
    try {
      festivals.value = (await endpoints.festivalMaps.list()).maps
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
      festival_map_id: b.festival_map_id,
      title: b.title,
      template_image: b.template_image,
      instructions: b.instructions,
      discord_link: b.discord_link,
      allow_self: b.allow_self,
      allow_site: b.allow_site,
      allow_artist: b.allow_artist,
      artist_name: b.artist_name,
      windows: (b.windows ?? []).map((w) => ({
        id: w.id,
        label: w.label,
        start_at: utcToDatetimeLocal(w.start_at),
        end_at: utcToDatetimeLocal(w.end_at),
        webhook_url: w.webhook_url,
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
    if (f) f.windows.push(blankWindow(f.windows.at(-1)))
  }

  function removeWindow(index: number): void {
    boardForm.value?.windows.splice(index, 1)
  }

  /** Saves the form. Returns the saved board's id, or null when it was refused. */
  async function saveBoard(): Promise<number | null> {
    const f = boardForm.value
    if (!f || saving.value) return null
    const body = {
      festival_map_id: f.festival_map_id ?? 0,
      title: f.title,
      template_image: f.template_image,
      instructions: f.instructions,
      discord_link: f.discord_link,
      allow_self: f.allow_self,
      allow_site: f.allow_site,
      allow_artist: f.allow_artist,
      artist_name: f.artist_name,
      windows: f.windows.map((w) => ({
        id: w.id,
        label: w.label,
        start_at: datetimeLocalToUtc(w.start_at),
        end_at: datetimeLocalToUtc(w.end_at),
        webhook_url: w.webhook_url,
      })),
    }
    saving.value = true
    try {
      let id = f.id
      if (id) await endpoints.emaBoards.update(id, body)
      else id = (await endpoints.emaBoards.create(body)).board.id
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
      await endpoints.emaBoards.setStatus(b.id, status)
      if (selectedBoard.value?.id === b.id) selectedBoard.value = { ...selectedBoard.value, status }
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
    if (!ok) return false
    try {
      await endpoints.emaBoards.delete(b.id)
      if (selectedBoard.value?.id === b.id) selectedBoard.value = null
      await loadBoards()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  // -- Review -------------------------------------------------------------------

  async function review(p: EmaPlaque, action: 'approve' | 'reject'): Promise<void> {
    if (reviewing.value.has(p.id)) return
    if (action === 'reject') {
      const ok = await ui.confirm(
        `Reject ${p.character_name}'s plaque? Its image is deleted, and they can send another for ${p.window_label}.`,
        { confirmText: 'Reject', danger: true },
      )
      if (!ok) return
    }
    reviewing.value = new Set(reviewing.value).add(p.id)
    try {
      const res =
        action === 'approve'
          ? await endpoints.emaBoards.approve(p.board_id, p.id)
          : await endpoints.emaBoards.reject(p.board_id, p.id)
      plaques.value = plaques.value.map((x) => (x.id === p.id ? res.plaque : x))
      ui.notify(action === 'approve' ? 'Posted to Discord' : 'Plaque rejected', 'success')
      void loadBoards()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      if (selectedBoard.value) void loadPlaques(selectedBoard.value.id)
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
        requests.value = (await endpoints.emaRequests.list()).plaques
      } catch (e) {
        ui.notify((e as Error).message, 'error')
      }
    })
  }

  async function setRequestComplete(p: EmaPlaque, complete: boolean): Promise<void> {
    try {
      const res = await endpoints.emaRequests.setComplete(p.id, complete)
      requests.value = requests.value.map((x) => (x.id === p.id ? res.plaque : x))
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      void loadRequests()
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
  }
})
