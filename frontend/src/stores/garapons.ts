/**
 * Garapons store: admin management (CRUD, prizes, per-player drawing links, draw
 * log) plus the public token-based player view + authoritative draw.
 *
 * Structurally a leaner cousin of the raffles store - a garapon has no ticket
 * sign-up or cost; instead an admin issues each player a tokenized link with a
 * draw allowance, and the server picks each prize. The grand-prize image is
 * picked from the "Garapon" image category, exactly like raffle prize images.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  api,
  unwrap,
  type Garapon,
  type GaraponDraw,
  type GaraponDrawResult,
  type GaraponPlayer,
  type GaraponPlayerSummary,
  type PublicGarapon,
  type PublicGaraponDraw,
  type PublicGaraponPrize,
  type PublicGaraponPlayer,
  type StampRally,
} from '@/api/client'
import type { GaraponForm, GaraponPrizeForm } from '@/types/api'
import { useUiStore } from './ui'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'

/** A sensible default ball color for a fresh prize row (a festival highlight). */
const DEFAULT_BALL_COLOR = '#e5b53f'

/** A fresh prize row for the editor (the first/grand row defaults to a higher weight). */
function blankPrize(rate: number, isGrand = false): GaraponPrizeForm {
  return { name: '', ballColor: DEFAULT_BALL_COLOR, rate, isGrand, _uid: nextUid() }
}

export const useGaraponsStore = defineStore('garapons', () => {
  const ui = useUiStore()

  // -- Admin state ----------------------------------------------------------
  const garapons = ref<Garapon[]>([])
  const selectedGarapon = ref<Garapon | null>(null)
  const garaponPlayers = ref<GaraponPlayer[]>([])
  const garaponDraws = ref<GaraponDraw[]>([])
  const garaponForm = ref<GaraponForm | null>(null)
  /** Open stamp rallies offered in the "Linked Stamp Rally" picker on the form. */
  const stampRallyOptions = ref<StampRally[]>([])
  /** Admin "generate drawing" form (issue a new per-player link). */
  const playerAdd = ref<{ playerName: string; maxDraws: number }>({ playerName: '', maxDraws: 1 })

  const garaponsLoading = ref(false)
  const detailLoading = ref(false)
  // Monotonic token guarding loadGaraponDetail against a last-write-wins race
  // (a slow earlier open overwriting a newer one). Only the latest request applies.
  let detailSeq = 0
  const savingGarapon = ref(false)
  const creatingPlayer = ref(false)

  // -- Public (player view) state -------------------------------------------
  // The public token view returns the trimmed PublicGarapon (no odds/aggregates).
  const publicGarapon = ref<PublicGarapon | null>(null)
  const publicPlayer = ref<GaraponPlayerSummary | null>(null)
  const publicDraws = ref<PublicGaraponDraw[]>([])
  /** Where the festival this drum's rally is filed under lives ('' = none). */
  const publicFestivalLink = ref('')
  /** The most recent win (drives the "congratulations" banner). */
  const lastWin = ref<PublicGaraponDraw | null>(null)
  const publicLoading = ref(false)
  const drawing = ref(false)

  // -- Computed -------------------------------------------------------------
  const openGarapons = computed(() => garapons.value.filter((g) => g.status === 'open'))
  const closedGarapons = computed(() => garapons.value.filter((g) => g.status === 'closed'))

  /** The flagged grand prize of the currently-viewed public garapon (if any). */
  const grandPrize = computed<PublicGaraponPrize | null>(
    () => publicGarapon.value?.prizes.find((p) => p.isGrand) ?? null,
  )
  /** The non-grand prizes (the "other possible prizes" table). */
  const otherPrizes = computed<PublicGaraponPrize[]>(
    () => publicGarapon.value?.prizes.filter((p) => !p.isGrand) ?? [],
  )
  /** Draws the public player has left. */
  const drawsRemaining = computed(() =>
    publicPlayer.value
      ? Math.max(0, publicPlayer.value.maxDraws - publicPlayer.value.drawsUsed)
      : 0,
  )
  /** Whether the public player can draw right now. */
  const canDraw = computed(
    () => publicGarapon.value?.status === 'open' && drawsRemaining.value > 0 && !drawing.value,
  )
  /**
   * The token of the Stamp Rally card linked to this drawing link (empty when the
   * garapon isn't tied to a rally). Its own token, not the drawing one, so the
   * public view can link straight to /stamp-card/<token>.
   */
  const publicStampCardToken = computed(() => publicPlayer.value?.stampCardToken ?? '')

  // -- Admin: load ----------------------------------------------------------
  async function loadGarapons(): Promise<void> {
    await withLoading(garaponsLoading, async () => {
      garapons.value = (await unwrap(api.GET('/garapons'))).garapons
    })
  }

  async function loadGaraponDetail(id: number): Promise<void> {
    const reqId = ++detailSeq
    detailLoading.value = true
    try {
      const data = await unwrap(api.GET('/garapons/{id}', { params: { path: { id } } }))
      if (reqId !== detailSeq) {
        return // a newer load superseded this one
      }
      selectedGarapon.value = data.garapon
      garaponPlayers.value = data.players
      garaponDraws.value = data.draws
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

  /** Admin: open a garapon's detail view. */
  function viewGarapon(g: Garapon): void {
    selectedGarapon.value = g
    garaponPlayers.value = []
    garaponDraws.value = []
    resetPlayerAdd()
    void loadGaraponDetail(g.id)
  }

  /**
   * Clears the issue-link form, starting the draw count at the garapon's own
   * default so the common case ("everyone gets N draws at this event") is typed
   * once on the garapon rather than re-entered for every link.
   */
  function resetPlayerAdd(): void {
    playerAdd.value = {
      playerName: '',
      maxDraws: Math.max(1, selectedGarapon.value?.defaultDraws ?? 1),
    }
  }

  /** Loads the OPEN stamp rallies for the form's "Linked Stamp Rally" picker. */
  async function loadStampRallyOptions(): Promise<void> {
    try {
      const data = await unwrap(api.GET('/stamp-rallies'))
      stampRallyOptions.value = data.stampRallies.filter((r) => r.status !== 'closed')
    } catch {
      stampRallyOptions.value = []
    }
  }

  // -- Admin: form ----------------------------------------------------------
  function newGaraponForm(): void {
    garaponForm.value = {
      id: 0,
      title: '',
      details: '',
      grandPrizeImage: '',
      stampRallyId: null,
      defaultDraws: 1,
      prizes: [blankPrize(50, true)],
    }
  }

  function editGaraponForm(g: Garapon): void {
    const prizes = g.prizes.map((p) => ({
      name: p.name,
      ballColor: p.ballColor || DEFAULT_BALL_COLOR,
      rate: p.rate,
      isGrand: p.isGrand,
      _uid: nextUid(),
    }))
    // Guarantee exactly one grand prize for the radio group.
    const firstPrize = prizes[0]
    if (firstPrize && !prizes.some((p) => p.isGrand)) {
      firstPrize.isGrand = true
    }
    garaponForm.value = {
      id: g.id,
      title: g.title,
      details: g.details,
      grandPrizeImage: g.grandPrizeImage,
      stampRallyId: g.stampRallyId,
      defaultDraws: Math.max(1, g.defaultDraws),
      prizes: prizes.length ? prizes : [blankPrize(50, true)],
    }
  }

  /**
   * Seed a brand-new garapon form from an existing one - the same drum run again,
   * with its prize table, ball colours, rates and artwork already in place.
   *
   * The stamp-rally link is deliberately dropped: it points at the rally the
   * ORIGINAL drum served, and carrying it over would quietly leave two garapons
   * issuing draws off one rally's sign-ups. Pick the new rally on the form. The
   * original's draw log and issued links stay with it.
   */
  function copyGaraponForm(g: Garapon): void {
    editGaraponForm(g)
    const f = garaponForm.value
    if (!f) {
      return
    }
    f.id = 0
    f.title = `${g.title} (Copy)`
    f.stampRallyId = null
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
    if (!f || f.prizes.length <= 1) {
      return
    }
    const row = f.prizes[index]
    if (!row) {
      return
    }
    f.prizes.splice(index, 1)
    // More than one row was there, so a first one remains.
    const first = f.prizes[0]
    if (row.isGrand && first && !f.prizes.some((p) => p.isGrand)) {
      first.isGrand = true
    }
  }

  /** Marks one prize row as the grand prize (radio-style, single selection). */
  function setGrandPrize(index: number): void {
    const f = garaponForm.value
    if (!f) {
      return
    }
    f.prizes.forEach((p, i) => (p.isGrand = i === index))
  }

  /** Saves the garapon form. Returns true on success (caller navigates). */
  async function saveGarapon(): Promise<boolean> {
    const f = garaponForm.value
    if (!f) {
      return false
    }
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
      const body = {
        title: f.title,
        details: f.details,
        grandPrizeImage: f.grandPrizeImage,
        stampRallyId: f.stampRallyId,
        defaultDraws: Math.max(1, Math.floor(f.defaultDraws || 1)),
        // The client-only `_uid` stays behind.
        prizes: named.map((p) => ({
          name: p.name,
          ballColor: p.ballColor,
          rate: p.rate,
          isGrand: p.isGrand,
        })),
      }
      if (f.id) {
        await unwrap(api.PUT('/garapons/{id}', { params: { path: { id: f.id } }, body }))
        ui.notify('Garapon updated', 'success')
      } else {
        await unwrap(api.POST('/garapons', { body }))
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
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/garapons/{id}', { params: { path: { id } } }))
      garapons.value = garapons.value.filter((g) => g.id !== id)
      if (selectedGarapon.value?.id === id) {
        selectedGarapon.value = null
      }
      ui.notify('Garapon deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function setGaraponStatus(id: number, status: 'open' | 'closed'): Promise<void> {
    try {
      const params = { params: { path: { id } } }
      const changed = await unwrap(
        status === 'closed'
          ? api.POST('/garapons/{id}/close', params)
          : api.POST('/garapons/{id}/reopen', params),
      )
      if (selectedGarapon.value?.id === id) {
        selectedGarapon.value.status = changed.status
      }
      const inList = garapons.value.find((g) => g.id === id)
      if (inList) {
        inList.status = changed.status
      }
      ui.notify(status === 'closed' ? 'Garapon closed' : 'Garapon reopened', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Admin: drawing links -------------------------------------------------
  async function createPlayer(): Promise<void> {
    if (!selectedGarapon.value) {
      return
    }
    const name = playerAdd.value.playerName.trim()
    if (!name) {
      ui.notify('Player name is required', 'error')
      return
    }
    const max = Math.max(1, Math.floor(playerAdd.value.maxDraws || 1))
    creatingPlayer.value = true
    try {
      const player = await unwrap(
        api.POST('/garapons/{id}/players', {
          params: { path: { id: selectedGarapon.value.id } },
          body: { playerName: name, maxDraws: max },
        }),
      )
      // Copy the new link straight to the clipboard so the admin can paste it to
      // the player. The URL is too long for the toast, so we only confirm the
      // action; do the copy before the detail reload to stay within the click's
      // activation window, and fall back gracefully if the browser blocks it.
      let copied = false
      try {
        await navigator.clipboard.writeText(playerLinkUrl(player))
        copied = true
      } catch {
        /* clipboard blocked (insecure context / permissions) - the per-row Copy link button still works */
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
    if (!selectedGarapon.value) {
      return
    }
    // Deleting a link keeps its draws in the log (they detach, not delete), so
    // reassure the admin when the player has already drawn.
    const msg =
      player.drawsUsed > 0
        ? `Delete the drawing link for ${player.playerName}? Their ${player.drawsUsed} draw${
            player.drawsUsed === 1 ? '' : 's'
          } stay in the draw log.`
        : `Delete the drawing link for ${player.playerName}?`
    if (!(await ui.confirm(msg, { title: 'Delete drawing link', confirmText: 'Delete' }))) {
      return
    }
    try {
      await unwrap(
        api.DELETE('/garapons/{id}/players/{playerId}', {
          params: { path: { id: selectedGarapon.value.id, playerId: player.id } },
        }),
      )
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
      // Clipboard blocked (insecure context / permissions) - surface the URL.
      ui.notify(url, 'info')
    }
  }

  /** The stamp card link issued with this drawing link, on a token of its own, if any. */
  function stampCardLinkUrl(player: GaraponPlayer): string {
    return player.stampCardToken
      ? `${window.location.origin}/stamp-card/${player.stampCardToken}`
      : ''
  }

  /** Copies a player's stamp card link, which opens the card and nothing else. */
  async function copyStampCardLink(player: GaraponPlayer): Promise<void> {
    const url = stampCardLinkUrl(player)
    if (!url) {
      return
    }
    try {
      await navigator.clipboard.writeText(url)
      ui.notify('Stamp card link copied to clipboard', 'success')
    } catch {
      ui.notify(url, 'info')
    }
  }

  // -- Public: player view + draw -------------------------------------------
  function resetPublic(): void {
    publicGarapon.value = null
    publicPlayer.value = null
    publicDraws.value = []
    publicFestivalLink.value = ''
    lastWin.value = null
  }

  /**
   * Shows a drum loaded by someone else - the festival dashboard, which reads it
   * as part of the participant's whole standing - through the same state the
   * drawing page uses, so the one panel renders both.
   */
  function showPublic(
    data: Pick<PublicGaraponPlayer, 'garapon' | 'draws'> & { player: GaraponPlayerSummary | null },
  ): void {
    // A fresh drum, or the same one reloaded: either way the banner belongs to a
    // draw made on THIS view, so it only survives a reload of the same player.
    if (publicGarapon.value?.id !== data.garapon.id) {
      lastWin.value = null
    }
    publicGarapon.value = data.garapon
    publicPlayer.value = data.player
    publicDraws.value = data.draws
  }

  /** Loads a player's garapon by token. Returns true on success. */
  async function loadByToken(token: string): Promise<boolean> {
    resetPublic()
    publicLoading.value = true
    try {
      const data = await unwrap(
        api.GET('/garapon-players/{token}', { params: { path: { token } } }),
      )
      publicGarapon.value = data.garapon
      publicPlayer.value = data.player
      publicDraws.value = data.draws
      publicFestivalLink.value = data.festivalLink
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
  async function draw(token: string): Promise<GaraponDrawResult | null> {
    if (drawing.value) {
      return null
    }
    drawing.value = true
    try {
      return await unwrap(
        api.POST('/garapon-players/{token}/draw', { params: { path: { token } } }),
      )
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    } finally {
      drawing.value = false
    }
  }

  /** Commits a drawn result to the visible record + remaining count + banner. */
  function commitDraw(resp: GaraponDrawResult): void {
    publicDraws.value = [...publicDraws.value, resp.draw]
    if (publicPlayer.value) {
      publicPlayer.value.drawsUsed = resp.drawsUsed
      publicPlayer.value.maxDraws = resp.maxDraws
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
    stampRallyOptions,
    playerAdd,
    garaponsLoading,
    detailLoading,
    savingGarapon,
    creatingPlayer,
    // public state
    publicGarapon,
    publicPlayer,
    publicDraws,
    publicFestivalLink,
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
    publicStampCardToken,
    // admin actions
    loadGarapons,
    loadGaraponDetail,
    viewGarapon,
    loadStampRallyOptions,
    newGaraponForm,
    editGaraponForm,
    copyGaraponForm,
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
    stampCardLinkUrl,
    copyStampCardLink,
    // public actions
    resetPublic,
    showPublic,
    loadByToken,
    draw,
    commitDraw,
  }
})
