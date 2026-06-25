/**
 * Garapons store: admin management (CRUD, prizes, per-player drawing links, draw
 * log) plus the public token-based player view + authoritative draw.
 *
 * Structurally a leaner cousin of the raffles store — a garapon has no ticket
 * sign-up or cost; instead an admin issues each player a tokenized link with a
 * draw allowance, and the server picks each prize. The grand-prize image is
 * picked from the "Garapon" image category, exactly like raffle prize images.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type {
  Garapon,
  GaraponDraw,
  GaraponDrawResponse,
  GaraponForm,
  GaraponPlayer,
  GaraponPrize,
  GaraponPrizeForm,
} from '@/types/api'
import { useUiStore } from './ui'
import { useImagesStore, IMAGE_DIR_GARAPONS } from './images'

/** A sensible default ball color for a fresh prize row (a festival gold). */
const DEFAULT_BALL_COLOR = '#e5b53f'

/** A fresh prize row for the editor (the first/grand row defaults to a higher weight). */
function blankPrize(rate: number, isGrand = false): GaraponPrizeForm {
  return { name: '', ball_color: DEFAULT_BALL_COLOR, rate, is_grand: isGrand }
}

export const useGaraponsStore = defineStore('garapons', () => {
  const ui = useUiStore()

  // ── Admin state ──────────────────────────────────────────────────────────
  const garapons = ref<Garapon[]>([])
  const selectedGarapon = ref<Garapon | null>(null)
  const garaponPlayers = ref<GaraponPlayer[]>([])
  const garaponDraws = ref<GaraponDraw[]>([])
  const garaponForm = ref<GaraponForm | null>(null)
  /** Reusable grand-prize images (the "Garapon" category on System → Images). */
  const grandPrizeImages = ref<string[]>([])
  /** Admin "generate drawing" form (issue a new per-player link). */
  const playerAdd = ref<{ playerName: string; maxDraws: number }>({ playerName: '', maxDraws: 1 })

  const garaponsLoading = ref(false)
  const detailLoading = ref(false)
  const savingGarapon = ref(false)
  const creatingPlayer = ref(false)

  // ── Public (player view) state ───────────────────────────────────────────
  const publicGarapon = ref<Garapon | null>(null)
  const publicPlayer = ref<{ player_name: string; max_draws: number; draws_used: number } | null>(
    null,
  )
  const publicDraws = ref<GaraponDraw[]>([])
  /** The most recent win (drives the "congratulations" banner). */
  const lastWin = ref<GaraponDraw | null>(null)
  const publicLoading = ref(false)
  const drawing = ref(false)

  // ── Computed ─────────────────────────────────────────────────────────────
  const openGarapons = computed(() => garapons.value.filter((g) => g.status === 'open'))
  const closedGarapons = computed(() => garapons.value.filter((g) => g.status === 'closed'))

  /** The flagged grand prize of the currently-viewed public garapon (if any). */
  const grandPrize = computed<GaraponPrize | null>(
    () => publicGarapon.value?.prizes?.find((p) => p.is_grand) ?? null,
  )
  /** The non-grand prizes (the "other possible prizes" table). */
  const otherPrizes = computed<GaraponPrize[]>(
    () => publicGarapon.value?.prizes?.filter((p) => !p.is_grand) ?? [],
  )
  /** Draws the public player has left. */
  const drawsRemaining = computed(() =>
    publicPlayer.value ? Math.max(0, publicPlayer.value.max_draws - publicPlayer.value.draws_used) : 0,
  )
  /** Whether the public player can draw right now. */
  const canDraw = computed(
    () => publicGarapon.value?.status === 'open' && drawsRemaining.value > 0 && !drawing.value,
  )

  // ── Admin: load ──────────────────────────────────────────────────────────
  async function loadGarapons(): Promise<void> {
    garaponsLoading.value = true
    try {
      const data = await endpoints.garapons.list()
      garapons.value = data.garapons || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      garaponsLoading.value = false
    }
  }

  async function loadGaraponDetail(id: number): Promise<void> {
    detailLoading.value = true
    try {
      const data = await endpoints.garapons.detail(id)
      selectedGarapon.value = data.garapon
      garaponPlayers.value = data.players || []
      garaponDraws.value = data.draws || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      detailLoading.value = false
    }
  }

  /** Admin: open a garapon's detail view. */
  function viewGarapon(g: Garapon): void {
    selectedGarapon.value = g
    garaponPlayers.value = []
    garaponDraws.value = []
    resetPlayerAdd()
    loadGaraponDetail(g.id)
  }

  function resetPlayerAdd(): void {
    playerAdd.value = { playerName: '', maxDraws: 1 }
  }

  /** Loads the reusable grand-prize images (the "Garapon" category) for the picker. */
  async function loadGrandPrizeImages(): Promise<void> {
    try {
      const images = useImagesStore()
      await images.loadImages(IMAGE_DIR_GARAPONS)
      grandPrizeImages.value = (images.imagesByDir[IMAGE_DIR_GARAPONS] || []).map((i) => i.path)
    } catch {
      /* non-fatal: the picker just shows nothing */
    }
  }

  // ── Admin: form ──────────────────────────────────────────────────────────
  function newGaraponForm(): void {
    garaponForm.value = {
      id: 0,
      title: '',
      details: '',
      grand_prize_image: '',
      prizes: [blankPrize(50, true)],
    }
  }

  function editGaraponForm(g: Garapon): void {
    const prizes = (g.prizes || []).map((p) => ({
      name: p.name,
      ball_color: p.ball_color || DEFAULT_BALL_COLOR,
      rate: p.rate,
      is_grand: p.is_grand,
    }))
    // Guarantee exactly one grand prize for the radio group.
    if (prizes.length && !prizes.some((p) => p.is_grand)) prizes[0].is_grand = true
    garaponForm.value = {
      id: g.id,
      title: g.title,
      details: g.details,
      grand_prize_image: g.grand_prize_image,
      prizes: prizes.length ? prizes : [blankPrize(50, true)],
    }
  }

  function cancelGaraponForm(): void {
    garaponForm.value = null
  }

  /** Adds a blank prize row to the form. */
  function addPrizeRow(): void {
    garaponForm.value?.prizes.push(blankPrize(10))
  }

  /** Removes a prize row; if it was the grand prize, promotes the first remaining row. */
  function removePrizeRow(index: number): void {
    const f = garaponForm.value
    if (!f || f.prizes.length <= 1) return
    const wasGrand = f.prizes[index].is_grand
    f.prizes.splice(index, 1)
    if (wasGrand && !f.prizes.some((p) => p.is_grand)) f.prizes[0].is_grand = true
  }

  /** Marks one prize row as the grand prize (radio-style, single selection). */
  function setGrandPrize(index: number): void {
    const f = garaponForm.value
    if (!f) return
    f.prizes.forEach((p, i) => (p.is_grand = i === index))
  }

  /** Saves the garapon form. Returns true on success (caller navigates). */
  async function saveGarapon(): Promise<boolean> {
    const f = garaponForm.value
    if (!f) return false
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    const named = f.prizes.filter((p) => p.name.trim())
    if (!named.length) {
      ui.notify('Add at least one prize with a name', 'error')
      return false
    }
    savingGarapon.value = true
    try {
      const payload = { ...f, prizes: named }
      if (f.id) {
        await endpoints.garapons.update(payload)
        ui.notify('Garapon updated', 'success')
      } else {
        await endpoints.garapons.create(payload)
        ui.notify('Garapon created', 'success')
      }
      garaponForm.value = null
      await loadGarapons()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingGarapon.value = false
    }
  }

  async function deleteGarapon(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this garapon and all its drawing links and results?', {
        title: 'Delete garapon',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.garapons.delete(id)
      garapons.value = garapons.value.filter((g) => g.id !== id)
      if (selectedGarapon.value?.id === id) selectedGarapon.value = null
      ui.notify('Garapon deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function setGaraponStatus(id: number, status: 'open' | 'closed'): Promise<void> {
    try {
      await endpoints.garapons.setStatus(id, status)
      if (selectedGarapon.value?.id === id) selectedGarapon.value.status = status
      const inList = garapons.value.find((g) => g.id === id)
      if (inList) inList.status = status
      ui.notify(status === 'closed' ? 'Garapon closed' : 'Garapon reopened', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // ── Admin: drawing links ─────────────────────────────────────────────────
  async function createPlayer(): Promise<void> {
    if (!selectedGarapon.value) return
    const name = playerAdd.value.playerName.trim()
    if (!name) {
      ui.notify('Player name is required', 'error')
      return
    }
    const max = Math.max(1, Math.floor(Number(playerAdd.value.maxDraws) || 1))
    creatingPlayer.value = true
    try {
      const data = await endpoints.garapons.createPlayer(selectedGarapon.value.id, {
        player_name: name,
        max_draws: max,
      })
      // Copy the new link straight to the clipboard so the admin can paste it to
      // the player. The URL is too long for the toast, so we only confirm the
      // action; do the copy before the detail reload to stay within the click's
      // activation window, and fall back gracefully if the browser blocks it.
      let copied = false
      try {
        await navigator.clipboard.writeText(playerLinkUrl(data.player))
        copied = true
      } catch {
        /* clipboard blocked (insecure context / permissions) — the per-row Copy link button still works */
      }
      ui.notify(
        copied ? 'Drawing link created and copied to clipboard' : 'Drawing link created',
        'success',
      )
      resetPlayerAdd()
      await loadGaraponDetail(selectedGarapon.value.id)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      creatingPlayer.value = false
    }
  }

  async function deletePlayer(player: GaraponPlayer): Promise<void> {
    if (!selectedGarapon.value) return
    // Deleting a link keeps its draws in the log (they detach, not delete), so
    // reassure the admin when the player has already drawn.
    const msg =
      player.draws_used > 0
        ? `Delete the drawing link for ${player.player_name}? Their ${player.draws_used} draw${
            player.draws_used === 1 ? '' : 's'
          } stay in the draw log.`
        : `Delete the drawing link for ${player.player_name}?`
    if (!(await ui.confirm(msg, { title: 'Delete drawing link', confirmText: 'Delete' }))) return
    try {
      await endpoints.garapons.deletePlayer(selectedGarapon.value.id, player.id)
      garaponPlayers.value = garaponPlayers.value.filter((p) => p.id !== player.id)
      ui.notify('Drawing link deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** A player's full public drawing link (origin + tokenized path). */
  function playerLinkUrl(player: GaraponPlayer): string {
    return `${window.location.origin}/garapon/${player.token}`
  }

  /** Builds a player's full public link and copies it to the clipboard. */
  async function copyPlayerLink(player: GaraponPlayer): Promise<void> {
    const url = playerLinkUrl(player)
    try {
      await navigator.clipboard.writeText(url)
      ui.notify('Link copied to clipboard', 'success')
    } catch {
      // Clipboard blocked (insecure context / permissions) — surface the URL.
      ui.notify(url, 'info')
    }
  }

  // ── Public: player view + draw ───────────────────────────────────────────
  function resetPublic(): void {
    publicGarapon.value = null
    publicPlayer.value = null
    publicDraws.value = []
    lastWin.value = null
  }

  /** Loads a player's garapon by token. Returns true on success. */
  async function loadByToken(token: string): Promise<boolean> {
    resetPublic()
    publicLoading.value = true
    try {
      const data = await endpoints.garapon.get(token)
      publicGarapon.value = data.garapon
      publicPlayer.value = data.player
      publicDraws.value = data.draws || []
      return true
    } catch {
      return false
    } finally {
      publicLoading.value = false
    }
  }

  /**
   * Performs one authoritative draw via the token. Returns the server response
   * (the won prize + fresh counts) so the wheel can animate to it, or null on
   * error. State is committed separately via `commitDraw` once the ball lands, so
   * the record/counts reveal in sync with the animation.
   */
  async function draw(token: string): Promise<GaraponDrawResponse | null> {
    if (drawing.value) return null
    drawing.value = true
    try {
      return await endpoints.garapon.draw(token)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    } finally {
      drawing.value = false
    }
  }

  /** Commits a drawn result to the visible record + remaining count + banner. */
  function commitDraw(resp: GaraponDrawResponse): void {
    publicDraws.value = [...publicDraws.value, resp.draw]
    if (publicPlayer.value) {
      publicPlayer.value.draws_used = resp.draws_used
      publicPlayer.value.max_draws = resp.max_draws
    }
    lastWin.value = resp.draw
  }

  return {
    // admin state
    garapons,
    selectedGarapon,
    garaponPlayers,
    garaponDraws,
    garaponForm,
    grandPrizeImages,
    playerAdd,
    garaponsLoading,
    detailLoading,
    savingGarapon,
    creatingPlayer,
    // public state
    publicGarapon,
    publicPlayer,
    publicDraws,
    lastWin,
    publicLoading,
    drawing,
    // computed
    openGarapons,
    closedGarapons,
    grandPrize,
    otherPrizes,
    drawsRemaining,
    canDraw,
    // admin actions
    loadGarapons,
    loadGaraponDetail,
    viewGarapon,
    loadGrandPrizeImages,
    newGaraponForm,
    editGaraponForm,
    cancelGaraponForm,
    addPrizeRow,
    removePrizeRow,
    setGrandPrize,
    saveGarapon,
    deleteGarapon,
    setGaraponStatus,
    createPlayer,
    deletePlayer,
    copyPlayerLink,
    // public actions
    resetPublic,
    loadByToken,
    draw,
    commitDraw,
  }
})
