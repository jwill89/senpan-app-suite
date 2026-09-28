/**
 * Stamp Rally store: admin management (events with stamps/prizes placed on the card,
 * tokenized participant cards, and the event-wide collection log) plus the public
 * token-based participant view (load a card, collect stamps by password).
 *
 * Structurally a cousin of the garapons store - an event owns sub-entities and the
 * admin issues each participant a tokenized link; the difference is the visual
 * placement of stamps/prizes and the password-driven public collection flow.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { saveRallySignup } from '@/lib/signups'
import type {
  Affiliate,
  FestivalMap,
  FestivalStallOccupant,
  Placement,
  PublicStampCard,
  StampRally,
  StampRallyCard,
  StampRallyForm,
  StampRallyLogEntry,
  StampRallyPrizeForm,
  StampRallyStamp,
  StampRallyStampForm,
  StampType,
  SignupRally,
  StampSignupResponse,
  StampLookupEntry,
} from '@/types/api'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'
import { stampTypeForStall } from '@/lib/festivalmap'
import { withLoading } from '@/lib/withLoading'
import { useUiStore } from './ui'

/** A fresh placement for a new stamp/prize: centred, modest size, no rotation. */
function defaultPlacement(): Placement {
  return { x: 42, y: 42, width: 16, height: 16, rotation: 0 }
}

function blankStamp(type: StampType): StampRallyStampForm {
  return {
    id: 0,
    occupant_id: null,
    affiliate_id: null,
    image: '',
    password: '',
    stamp_type: type,
    placement: defaultPlacement(),
    active_from: '',
    active_to: '',
    paused: false,
  }
}

function blankPrize(): StampRallyPrizeForm {
  return { id: 0, name: '', image: '', placement: defaultPlacement() }
}

/**
 * Settles whatever a number input left in a per-type requirement into a whole,
 * non-negative count. An emptied `<input type="number">` binds as '', which would
 * otherwise be sent as a string and rejected by the API. Exported for the form,
 * which clamps against the stamps on the card as well.
 */
export function toStampCount(value: number): number {
  return Number.isFinite(value) ? Math.max(0, Math.floor(value)) : 0
}

/**
 * Reorders a flat (already search+sorted) log so every participant's rows are
 * contiguous: groups by participant name (the snapshot, which survives card
 * deletion), ordering the groups by first appearance and keeping each group's rows in
 * their incoming (sorted) order. A permutation, so the row count is unchanged.
 * Exported for testing.
 */
export function groupedByParticipant(rows: StampRallyLogEntry[]): StampRallyLogEntry[] {
  const groups = new Map<string, StampRallyLogEntry[]>()
  const order: string[] = []
  for (const r of rows) {
    let g = groups.get(r.participant_name)
    if (!g) {
      g = []
      groups.set(r.participant_name, g)
      order.push(r.participant_name)
    }
    g.push(r)
  }
  return order.flatMap((name) => groups.get(name) as StampRallyLogEntry[])
}

export const useStampRalliesStore = defineStore('stampRallies', () => {
  const ui = useUiStore()

  // -- Admin state ----------------------------------------------------------
  const rallies = ref<StampRally[]>([])
  const selectedRally = ref<StampRally | null>(null)
  const rallyCards = ref<StampRallyCard[]>([])
  const rallyLogs = ref<StampRallyLogEntry[]>([])
  const rallyForm = ref<StampRallyForm | null>(null)
  /** Affiliates, for the per-stamp "stall" select (null = Senpan Tea House). */
  const affiliates = ref<Affiliate[]>([])
  /** Festival maps a rally can be linked to (published or still in progress). */
  const festivalMaps = ref<FestivalMap[]>([])
  /**
   * The linked map's pitch OCCUPANTS - the stall list a linked rally picks from,
   * flattened across pitches. A pitch that changes hands between days contributes
   * one entry per occupant, since each is a different business running a
   * different activity and so needs its own stamp.
   */
  const mapStalls = ref<FestivalStallOccupant[]>([])
  /** New-card form state. */
  const cardAdd = ref<{ participantName: string }>({ participantName: '' })
  /** Stamps loaded for an expanded "Manage stalls" panel on a list card, by rally id. */
  const cardStamps = ref<Record<number, StampRallyStamp[]>>({})

  const ralliesLoading = ref(false)
  const detailLoading = ref(false)
  // Monotonic token guarding loadRallyDetail against a last-write-wins race
  // (a slow earlier open overwriting a newer one). Only the latest request applies.
  let detailSeq = 0
  const logsLoading = ref(false)
  const savingRally = ref(false)
  const creatingCard = ref(false)

  // -- Public state ---------------------------------------------------------
  const publicCard = ref<PublicStampCard | null>(null)
  const publicLoading = ref(false)
  const submitting = ref(false)
  /** The most recently collected stamp id (drives the reveal animation/highlight). */
  const lastCollectedId = ref<number | null>(null)

  // -- Computed -------------------------------------------------------------
  const openRallies = computed(() => rallies.value.filter((r) => r.status !== 'closed'))
  const closedRallies = computed(() => rallies.value.filter((r) => r.status === 'closed'))

  const drawsRemaining = computed(() => {
    const c = publicCard.value
    if (!c) return 0
    return c.stamps.filter((s) => !s.collected && s.available).length
  })

  /**
   * Progress on the loaded public card, phrased the way the rally's completion
   * rule works: a "counts" rally is measured per type (so many food stamps, so
   * many game stamps, the rest optional), anything else against the whole card.
   */
  const cardProgress = computed(() => {
    const c = publicCard.value
    const stamps = c?.stamps ?? []
    /**
     * One type's tally. `unreachable` means the requirement can no longer be met -
     * what is collected plus what is still collectable falls short, because stalls
     * closed for good (the server's `expired`, not a stall that merely reopens
     * later). The card says so rather than leaving the participant to work out why
     * it never finishes.
     */
    const tally = (type: StampType, required: number) => {
      const of = stamps.filter((s) => (s.stamp_type === 'game' ? 'game' : 'food') === type)
      const collected = of.filter((s) => s.collected).length
      const open = of.filter((s) => !s.collected && !s.expired).length
      return {
        collected,
        open,
        required,
        total: of.length,
        unreachable: required > 0 && collected < required && collected + open < required,
      }
    }
    return {
      byType: c?.rally.completion_mode === 'counts',
      food: tally('food', c?.rally.required_food ?? 0),
      game: tally('game', c?.rally.required_game ?? 0),
      collected: stamps.filter((s) => s.collected).length,
      total: stamps.length,
    }
  })

  // -- Admin: load ----------------------------------------------------------
  async function loadRallies(): Promise<void> {
    await withLoading(ralliesLoading, async () => {
      const data = await endpoints.stampRallies.list()
      rallies.value = data.stamp_rallies
    })
    // A live invalidation (admin.ts) re-runs loadRallies; refresh any expanded
    // "Manage stalls" panels too so their cached stamps don't go stale forever.
    await refreshLoadedCardStamps()
  }

  async function loadRallyDetail(id: number): Promise<void> {
    const reqId = ++detailSeq
    detailLoading.value = true
    try {
      const data = await endpoints.stampRallies.detail(id)
      if (reqId !== detailSeq) return // a newer load superseded this one
      selectedRally.value = data.stamp_rally
      rallyCards.value = data.cards
    } catch (e) {
      if (reqId === detailSeq) ui.notify((e as Error).message, 'error')
    } finally {
      if (reqId === detailSeq) detailLoading.value = false
    }
  }

  async function loadRallyLogs(id: number): Promise<void> {
    await withLoading(logsLoading, async () => {
      const data = await endpoints.stampRallies.logs(id)
      rallyLogs.value = data.logs
    })
  }

  /** Admin: open a rally's detail view (loads detail + cards). */
  function viewRally(r: StampRally): void {
    selectedRally.value = r
    rallyCards.value = []
    rallyLogs.value = []
    cardAdd.value = { participantName: '' }
    void loadRallyDetail(r.id)
  }

  /**
   * Loads what the form's per-stamp "stall" select offers: the affiliates, plus
   * the festival maps a rally can be linked to. A rally already linked to a map
   * also loads that map's stalls, which then REPLACE the affiliate list - a
   * linked rally names stalls, not bare partners.
   *
   * A closed map is deliberately still offered: a rally is usually authored
   * against the map at the same time, and dropping the link the moment the
   * festival closed would silently strip every stamp's stall.
   */
  async function loadFormSources(): Promise<void> {
    // All three are independent - none consumes another's result - so run them
    // together rather than paying two or three sequential round trips every time
    // the form opens. Each keeps its own fallback, so one failing (typically the
    // map list, for a grantee without that page) still leaves the others loaded.
    const mapID = rallyForm.value?.festival_map_id ?? null
    await Promise.all([
      (async () => {
        try {
          affiliates.value = (await endpoints.affiliates.list()).affiliates
        } catch {
          affiliates.value = []
        }
      })(),
      (async () => {
        try {
          festivalMaps.value = (await endpoints.festivalMaps.list()).maps
        } catch {
          // No festival-map permission (or none exist) - the link select just
          // stays empty and the rally keeps naming affiliates.
          festivalMaps.value = []
        }
      })(),
      loadMapStalls(mapID),
    ])
  }

  /** Loads a festival map's stalls for the stall select ('' / null clears them). */
  async function loadMapStalls(mapId: number | null): Promise<void> {
    if (!mapId) {
      mapStalls.value = []
      return
    }
    try {
      const stalls = (await endpoints.festivalMaps.detail(mapId)).map.stalls ?? []
      mapStalls.value = stalls.flatMap((stall) => stall.occupants)
    } catch {
      mapStalls.value = []
    }
  }

  /**
   * Links (or unlinks) the form's rally to a festival map and reloads its stalls.
   * Switching maps clears every stamp's stall: the ids belong to the map that was
   * linked before, and the server would drop them on save anyway - better the
   * admin sees the empty selects now than a silent reset afterwards.
   */
  async function setFestivalMap(mapId: number | null): Promise<void> {
    const f = rallyForm.value
    if (!f) return
    if (f.festival_map_id === mapId) return
    f.festival_map_id = mapId
    for (const stamp of f.stamps) stamp.occupant_id = null
    await loadMapStalls(mapId)
  }

  /**
   * Points a stamp at one of the linked map's stall OCCUPANTS, taking its
   * affiliate and seeding the stamp type from what it offers. Both stay editable
   * afterwards - the occupant is the starting point, not a lock.
   */
  function setStampStall(stampIndex: number, occupantId: number | null): void {
    const stamp = rallyForm.value?.stamps[stampIndex]
    if (!stamp) return
    stamp.occupant_id = occupantId
    const occupant = mapStalls.value.find((o) => o.id === occupantId)
    if (!occupant) return
    stamp.affiliate_id = occupant.affiliate_id ?? null
    stamp.stamp_type = stampTypeForStall(occupant.stall_type)
  }

  // -- Admin: form ----------------------------------------------------------
  function newRallyForm(): void {
    rallyForm.value = {
      id: 0,
      title: '',
      card_image: '',
      not_stamped_image: '',
      available_from: '',
      available_to: '',
      details: '',
      redeem_instructions: '',
      redeem_image: '',
      public_signup: false,
      completion_mode: 'all',
      required_food: 0,
      required_game: 0,
      festival_map_id: null,
      stamps: [],
      prizes: [],
    }
  }

  /**
   * True when `r` carries its child collections, i.e. it came from a detail fetch
   * rather than the list.
   *
   * The list omits `stamps`/`prizes` entirely (`omitempty` server-side), and a
   * save is a FULL REPLACE - so seeding the form from a list row and saving it
   * would delete every stamp on the rally and every participant's collected rows.
   * The server now refuses to touch a collection a request didn't carry, but the
   * form must not offer the edit in the first place: silently saving fewer stamps
   * than the admin can see is its own bug.
   */
  function hasRallyDetail(r: StampRally | null | undefined): boolean {
    return !!r && Array.isArray(r.stamps) && Array.isArray(r.prizes)
  }

  function editRallyForm(r: StampRally): boolean {
    if (!hasRallyDetail(r)) {
      ui.notify('This rally is still loading. Try again in a moment.', 'error')
      return false
    }
    rallyForm.value = {
      id: r.id,
      title: r.title,
      card_image: r.card_image,
      not_stamped_image: r.not_stamped_image,
      // Stored UTC -> this admin's local wall-clock for the datetime-local inputs.
      available_from: utcToDatetimeLocal(r.available_from),
      available_to: utcToDatetimeLocal(r.available_to),
      details: r.details,
      redeem_instructions: r.redeem_instructions,
      redeem_image: r.redeem_image,
      public_signup: r.public_signup,
      completion_mode: r.completion_mode === 'counts' ? 'counts' : 'all',
      required_food: r.required_food,
      required_game: r.required_game,
      festival_map_id: r.festival_map_id ?? null,
      stamps: (r.stamps || []).map((s) => ({
        id: s.id,
        occupant_id: s.occupant_id ?? null,
        affiliate_id: s.affiliate_id ?? null,
        image: s.image,
        password: s.password ?? '',
        stamp_type: s.stamp_type === 'game' ? 'game' : 'food',
        placement: { ...s.placement },
        active_from: utcToDatetimeLocal(s.active_from),
        active_to: utcToDatetimeLocal(s.active_to),
        paused: s.paused,
      })),
      prizes: (r.prizes || []).map((p) => ({
        id: p.id,
        name: p.name,
        image: p.image,
        placement: { ...p.placement },
      })),
    }
    return true
  }

  /**
   * Seed a brand-new rally form from an existing one - the same event run again,
   * with its stalls, prizes, artwork and instructions already in place.
   *
   * What it deliberately does NOT carry over is everything tied to the run that
   * already happened: every id (so saving creates rather than overwrites), the
   * availability window, and the per-stamp active windows - a stale window would
   * silently gate a stall on dates from the last event. Issued cards belong to the
   * original rally and are not touched at all.
   */
  function copyRallyForm(r: StampRally): boolean {
    if (!editRallyForm(r)) return false
    const f = rallyForm.value
    if (!f) return false
    f.id = 0
    f.title = `${r.title} (Copy)`
    f.available_from = ''
    f.available_to = ''
    // The festival link points at the map the ORIGINAL rally ran at, whose stalls
    // still exist - so nothing on the server would clear it, and next year's rally
    // would quietly file itself under the finished festival with its stamps
    // naming last year's stalls. Both go, the same way a garapon copy drops its
    // stamp_rally_id; pick the festival again on the copy.
    f.festival_map_id = null
    for (const stamp of f.stamps) {
      stamp.id = 0
      stamp.occupant_id = null
      stamp.active_from = ''
      stamp.active_to = ''
      stamp.paused = false
    }
    for (const prize of f.prizes) prize.id = 0
    return true
  }

  function cancelRallyForm(): void {
    rallyForm.value = null
  }

  function addStamp(type: StampType = 'food'): void {
    rallyForm.value?.stamps.push(blankStamp(type))
  }
  function removeStamp(index: number): void {
    rallyForm.value?.stamps.splice(index, 1)
  }
  function addPrize(): void {
    rallyForm.value?.prizes.push(blankPrize())
  }
  function removePrize(index: number): void {
    rallyForm.value?.prizes.splice(index, 1)
  }

  /** Saves the rally form. Returns true on success (caller navigates back). */
  async function saveRally(): Promise<boolean> {
    const f = rallyForm.value
    if (!f) return false
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    // The number inputs can hold anything the admin typed (including an empty
    // field), so settle them into whole counts before they are validated or sent.
    const requiredFood = toStampCount(f.required_food)
    const requiredGame = toStampCount(f.required_game)
    // Per-type completion that requires nothing of either type would finish every
    // card at its first stamp - the server refuses it, so say so here rather than
    // bouncing the admin off a 400.
    if (f.completion_mode === 'counts' && requiredFood + requiredGame === 0) {
      ui.notify(
        'Set how many food or game stamps a card needs, or switch completion back to every stamp.',
        'error',
      )
      return false
    }
    savingRally.value = true
    try {
      // The form holds local datetime-local values; convert the event + per-stamp
      // windows to stored UTC before sending (mirrors the raffle store).
      const payload = {
        ...f,
        available_from: datetimeLocalToUtc(f.available_from),
        available_to: datetimeLocalToUtc(f.available_to),
        required_food: requiredFood,
        required_game: requiredGame,
        stamps: f.stamps.map((s) => ({
          ...s,
          active_from: datetimeLocalToUtc(s.active_from),
          active_to: datetimeLocalToUtc(s.active_to),
        })),
      }
      if (f.id) {
        await endpoints.stampRallies.update(payload)
        ui.notify('Stamp rally updated', 'success')
      } else {
        await endpoints.stampRallies.create(payload)
        ui.notify('Stamp rally created', 'success')
      }
      rallyForm.value = null
      await loadRallies()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingRally.value = false
    }
  }

  async function deleteRally(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this stamp rally and all its cards and stamp records?', {
        title: 'Delete stamp rally',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.stampRallies.delete(id)
      rallies.value = rallies.value.filter((r) => r.id !== id)
      if (selectedRally.value?.id === id) selectedRally.value = null
      ui.notify('Stamp rally deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Open/close a rally (closed = read-only, moves to the closed table, unlinkable). */
  async function setRallyStatus(id: number, status: 'open' | 'closed'): Promise<void> {
    try {
      await endpoints.stampRallies.setStatus(id, status)
      if (selectedRally.value?.id === id) selectedRally.value.status = status
      const inList = rallies.value.find((r) => r.id === id)
      if (inList) inList.status = status
      ui.notify(status === 'closed' ? 'Stamp rally closed' : 'Stamp rally reopened', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Loads a rally's stamps for the inline "Manage stalls" panel on a list card. */
  async function loadCardStamps(rallyId: number): Promise<void> {
    if (rallyId in cardStamps.value) return // already loaded
    try {
      const data = await endpoints.stampRallies.detail(rallyId)
      cardStamps.value = { ...cardStamps.value, [rallyId]: data.stamp_rally.stamps || [] }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Silently re-fetches the stamps for every rally whose inline "Manage stalls"
   * panel is currently loaded. Called after a list reload (initial load is a
   * no-op - nothing is expanded yet) so a live invalidation refreshes the cached
   * panels instead of leaving them stale. Errors are swallowed: a background
   * refresh must not toast, and the existing cache stays put on a transient fail.
   */
  async function refreshLoadedCardStamps(): Promise<void> {
    const ids = Object.keys(cardStamps.value).map(Number)
    await Promise.all(
      ids.map(async (rallyId) => {
        try {
          const data = await endpoints.stampRallies.detail(rallyId)
          cardStamps.value = { ...cardStamps.value, [rallyId]: data.stamp_rally.stamps || [] }
        } catch {
          /* leave the cached stamps in place on a transient error */
        }
      }),
    )
  }

  /** Pause/resume a stall from the list card's inline panel, updating that card's
   *  loaded stamps and its "active stalls" count without a reload. */
  async function setStampPausedInList(
    rallyId: number,
    stampId: number,
    paused: boolean,
  ): Promise<void> {
    try {
      await endpoints.stampRallies.setStampPaused(rallyId, stampId, paused)
      const st = cardStamps.value[rallyId].find((s) => s.id === stampId)
      if (st) st.paused = paused
      const r = rallies.value.find((rr) => rr.id === rallyId)
      if (r) r.active_stamp_count = Math.max(0, (r.active_stamp_count ?? 0) + (paused ? -1 : 1))
      ui.notify(paused ? 'Stall paused' : 'Stall resumed', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Pause/resume a single stamp on the selected rally (live availability toggle). */
  async function setStampPaused(stampId: number, paused: boolean): Promise<void> {
    if (!selectedRally.value) return
    try {
      await endpoints.stampRallies.setStampPaused(selectedRally.value.id, stampId, paused)
      const st = selectedRally.value.stamps?.find((s) => s.id === stampId)
      if (st) st.paused = paused
      ui.notify(paused ? 'Stall paused' : 'Stall resumed', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Admin: participant cards ---------------------------------------------
  async function createCard(): Promise<void> {
    if (!selectedRally.value) return
    const name = cardAdd.value.participantName.trim()
    if (!name) {
      ui.notify('Participant name is required', 'error')
      return
    }
    creatingCard.value = true
    try {
      const data = await endpoints.stampRallies.createCard(selectedRally.value.id, name)
      let copied = false
      try {
        await navigator.clipboard.writeText(cardLinkUrl(data.card))
        copied = true
      } catch {
        /* clipboard blocked - the per-row copy button still works */
      }
      ui.notify(
        copied ? 'Card link created and copied to clipboard' : 'Card link created',
        'success',
      )
      cardAdd.value = { participantName: '' }
      await loadRallyDetail(selectedRally.value.id)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      creatingCard.value = false
    }
  }

  async function deleteCard(card: StampRallyCard): Promise<void> {
    if (!selectedRally.value) return
    if (
      !(await ui.confirm(
        `Delete the card for ${card.participant_name}? Their stamp record is removed.`,
        {
          title: 'Delete card',
          confirmText: 'Delete',
        },
      ))
    )
      return
    try {
      await endpoints.stampRallies.deleteCard(selectedRally.value.id, card.id)
      rallyCards.value = rallyCards.value.filter((c) => c.id !== card.id)
      ui.notify('Card deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** A participant card's full public link (origin + tokenized path). */
  function cardLinkUrl(card: StampRallyCard): string {
    return `${window.location.origin}/stamp-card/${card.token}`
  }

  /**
   * Copies a link to the clipboard, falling back to showing it in a toast when
   * the clipboard is unavailable (an insecure origin, or a browser that refuses
   * without a user gesture) - the participant can still read and copy it by hand
   * rather than being told nothing happened.
   */
  async function copyLink(url: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(url)
      ui.notify('Link copied to clipboard', 'success')
    } catch {
      ui.notify(url, 'info')
    }
  }

  async function copyCardLink(card: StampRallyCard): Promise<void> {
    await copyLink(cardLinkUrl(card))
  }

  // -- Public: self-service sign-up + link lookup ----------------------------
  //
  // The tokens the server hands back are turned into links here rather than by
  // each view, so the participant-facing URLs are built in exactly one place (the
  // same shape cardLinkUrl produces for the admin card list).

  /** Rallies currently open to public sign-up (empty until loadSignupRallies). */
  const signupRallies = ref<SignupRally[]>([])
  const signupLoading = ref(false)
  /** The result of a successful sign-up, held so the view can show the links. */
  const signupResult = ref<StampSignupResponse | null>(null)
  /** Lookup results; null = no search run yet, [] = searched and found nothing. */
  const lookupResults = ref<StampLookupEntry[] | null>(null)
  const lookupLoading = ref(false)

  /** A stamp card's public link for a bare token. */
  function stampCardUrl(token: string): string {
    return `${window.location.origin}/stamp-card/${token}`
  }

  /** A garapon drawing link for a bare token. */
  function garaponUrl(token: string): string {
    return `${window.location.origin}/garapon/${token}`
  }

  async function loadSignupRallies(): Promise<void> {
    signupLoading.value = true
    try {
      signupRallies.value = (await endpoints.stampSignup.list()).rallies
    } catch {
      signupRallies.value = []
    } finally {
      signupLoading.value = false
    }
  }

  /**
   * Signs the participant up for a rally. Returns true on success, leaving the
   * issued tokens in signupResult. A duplicate name comes back as a 409 whose
   * message names the fix (the lookup page), so it is surfaced verbatim rather
   * than replaced with a generic failure.
   */
  async function signUp(
    rallyId: number,
    name: string,
    world: string,
    turnstileToken = '',
  ): Promise<boolean> {
    if (submitting.value) return false
    const trimmed = name.trim()
    if (!trimmed) {
      ui.notify('Enter your character name', 'error')
      return false
    }
    // Both halves are required: the world is what tells two players who share a
    // character name apart, here and in every other system that records one.
    const trimmedWorld = world.trim()
    if (!trimmedWorld) {
      ui.notify('Pick your home world', 'error')
      return false
    }
    submitting.value = true
    try {
      const issued = await endpoints.stampSignup.signUp(
        rallyId,
        trimmed,
        trimmedWorld,
        turnstileToken,
      )
      signupResult.value = issued
      // The only moment the drawing token exists client-side: no name-keyed lookup
      // returns it, because a draw is irreversible and a character name is public.
      // Saving it here is what lets this browser show the link again later.
      saveRallySignup({
        rallyId,
        rallyTitle: issued.rally_title,
        name: issued.participant_name,
        world: issued.world,
        cardToken: issued.card_token,
        garaponToken: issued.garapon_token || undefined,
        garaponTitle: issued.garapon_title || undefined,
      })
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      submitting.value = false
    }
  }

  /** Looks up a participant's links by the exact name they signed up with. */
  async function lookupLinks(name: string, world: string): Promise<void> {
    const trimmed = name.trim()
    if (!trimmed) {
      ui.notify('Enter the name you signed up with', 'error')
      return
    }
    lookupLoading.value = true
    try {
      lookupResults.value = (await endpoints.stampSignup.lookup(trimmed, world.trim())).entries
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      lookupResults.value = null
    } finally {
      lookupLoading.value = false
    }
  }

  function resetSignup(): void {
    signupResult.value = null
  }

  function resetLookup(): void {
    lookupResults.value = null
  }

  // -- Public: card view + collect ------------------------------------------
  function resetPublic(): void {
    publicCard.value = null
    lastCollectedId.value = null
  }

  async function loadByToken(token: string): Promise<boolean> {
    resetPublic()
    publicLoading.value = true
    try {
      publicCard.value = await endpoints.stampCard.get(token)
      return true
    } catch {
      return false
    } finally {
      publicLoading.value = false
    }
  }

  /** Submits a stamp password. Returns true on a successful collection. */
  async function submitPassword(token: string, password: string): Promise<boolean> {
    if (submitting.value) return false
    const pw = password.trim()
    if (!pw) {
      ui.notify('Enter a password', 'error')
      return false
    }
    // A "counts" card can be finished with stalls left to visit, so completion is
    // announced only when this stamp is the one that finished it - otherwise every
    // later stamp re-congratulates the participant.
    const wasComplete = publicCard.value?.completed ?? false
    submitting.value = true
    try {
      const data = await endpoints.stampCard.stamp(token, pw)
      publicCard.value = data.card
      lastCollectedId.value = data.collected_stamp_id
      ui.notify('Stamp collected!', 'success')
      if (data.card.completed && !wasComplete) {
        ui.notify('Card complete - your prizes are revealed below!', 'success')
      }
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      submitting.value = false
    }
  }

  return {
    // admin state
    rallies,
    selectedRally,
    rallyCards,
    rallyLogs,
    rallyForm,
    affiliates,
    festivalMaps,
    mapStalls,
    cardStamps,
    cardAdd,
    ralliesLoading,
    detailLoading,
    logsLoading,
    savingRally,
    creatingCard,
    // public state
    publicCard,
    publicLoading,
    submitting,
    lastCollectedId,
    // computed
    openRallies,
    closedRallies,
    drawsRemaining,
    cardProgress,
    // admin actions
    loadRallies,
    loadRallyDetail,
    loadRallyLogs,
    viewRally,
    loadFormSources,
    setFestivalMap,
    setStampStall,
    newRallyForm,
    hasRallyDetail,
    editRallyForm,
    copyRallyForm,
    cancelRallyForm,
    addStamp,
    removeStamp,
    addPrize,
    removePrize,
    saveRally,
    deleteRally,
    setRallyStatus,
    loadCardStamps,
    refreshLoadedCardStamps,
    setStampPausedInList,
    setStampPaused,
    createCard,
    deleteCard,
    cardLinkUrl,
    copyLink,
    copyCardLink,
    // public actions
    resetPublic,
    loadByToken,
    submitPassword,
    // public self-service sign-up + lookup
    signupRallies,
    signupLoading,
    signupResult,
    lookupResults,
    lookupLoading,
    stampCardUrl,
    garaponUrl,
    loadSignupRallies,
    signUp,
    lookupLinks,
    resetSignup,
    resetLookup,
  }
})
