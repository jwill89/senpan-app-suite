/**
 * Stamp Rally store: admin management (events with stamps/prizes placed on the card,
 * tokenized participant cards, and the event-wide collection log) plus the public
 * token-based participant view (load a card, collect stamps by password).
 *
 * Structurally a cousin of the garapons store — an event owns sub-entities and the
 * admin issues each participant a tokenized link; the difference is the visual
 * placement of stamps/prizes and the password-driven public collection flow.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type {
  Affiliate,
  Placement,
  PublicStampCard,
  StampRally,
  StampRallyCard,
  StampRallyForm,
  StampRallyLogEntry,
  StampRallyPrizeForm,
  StampRallyStamp,
  StampRallyStampForm,
} from '@/types/api'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'
import { useUiStore } from './ui'
import {
  useImagesStore,
  IMAGE_DIR_STAMP_CARDS,
  IMAGE_DIR_STAMP_STAMPS,
  IMAGE_DIR_STAMP_PRIZES,
} from './images'

/** A fresh placement for a new stamp/prize: centred, modest size, no rotation. */
function defaultPlacement(): Placement {
  return { x: 42, y: 42, width: 16, height: 16, rotation: 0 }
}

function blankStamp(): StampRallyStampForm {
  return {
    id: 0,
    affiliate_id: null,
    image: '',
    password: '',
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

  // ── Admin state ──────────────────────────────────────────────────────────
  const rallies = ref<StampRally[]>([])
  const selectedRally = ref<StampRally | null>(null)
  const rallyCards = ref<StampRallyCard[]>([])
  const rallyLogs = ref<StampRallyLogEntry[]>([])
  const rallyForm = ref<StampRallyForm | null>(null)
  /** Affiliates, for the per-stamp "stall" select (null = Senpan Tea House). */
  const affiliates = ref<Affiliate[]>([])
  /** Reusable images for the three pickers. */
  const cardImages = ref<string[]>([])
  const stampImages = ref<string[]>([])
  const prizeImages = ref<string[]>([])
  /** New-card form state. */
  const cardAdd = ref<{ participantName: string }>({ participantName: '' })
  /** Stamps loaded for an expanded "Manage stalls" panel on a list card, by rally id. */
  const cardStamps = ref<Record<number, StampRallyStamp[]>>({})

  const ralliesLoading = ref(false)
  const detailLoading = ref(false)
  const logsLoading = ref(false)
  const savingRally = ref(false)
  const creatingCard = ref(false)

  // ── Public state ─────────────────────────────────────────────────────────
  const publicCard = ref<PublicStampCard | null>(null)
  const publicLoading = ref(false)
  const submitting = ref(false)
  /** The most recently collected stamp id (drives the reveal animation/highlight). */
  const lastCollectedId = ref<number | null>(null)

  // ── Computed ─────────────────────────────────────────────────────────────
  const openRallies = computed(() => rallies.value.filter((r) => r.status !== 'closed'))
  const closedRallies = computed(() => rallies.value.filter((r) => r.status === 'closed'))

  const drawsRemaining = computed(() => {
    const c = publicCard.value
    if (!c) return 0
    return c.stamps.filter((s) => !s.collected && s.available).length
  })

  // ── Admin: load ──────────────────────────────────────────────────────────
  async function loadRallies(): Promise<void> {
    ralliesLoading.value = true
    try {
      const data = await endpoints.stampRallies.list()
      rallies.value = data.stamp_rallies || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      ralliesLoading.value = false
    }
  }

  async function loadRallyDetail(id: number): Promise<void> {
    detailLoading.value = true
    try {
      const data = await endpoints.stampRallies.detail(id)
      selectedRally.value = data.stamp_rally
      rallyCards.value = data.cards || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      detailLoading.value = false
    }
  }

  async function loadRallyLogs(id: number): Promise<void> {
    logsLoading.value = true
    try {
      const data = await endpoints.stampRallies.logs(id)
      rallyLogs.value = data.logs || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      logsLoading.value = false
    }
  }

  /** Admin: open a rally's detail view (loads detail + cards). */
  function viewRally(r: StampRally): void {
    selectedRally.value = r
    rallyCards.value = []
    rallyLogs.value = []
    cardAdd.value = { participantName: '' }
    loadRallyDetail(r.id)
  }

  /** Loads the three picker image categories + the affiliates list (for the form). */
  async function loadFormSources(): Promise<void> {
    try {
      const images = useImagesStore()
      await Promise.all([
        images.loadImages(IMAGE_DIR_STAMP_CARDS),
        images.loadImages(IMAGE_DIR_STAMP_STAMPS),
        images.loadImages(IMAGE_DIR_STAMP_PRIZES),
      ])
      cardImages.value = (images.imagesByDir[IMAGE_DIR_STAMP_CARDS] || []).map((i) => i.path)
      stampImages.value = (images.imagesByDir[IMAGE_DIR_STAMP_STAMPS] || []).map((i) => i.path)
      prizeImages.value = (images.imagesByDir[IMAGE_DIR_STAMP_PRIZES] || []).map((i) => i.path)
    } catch {
      /* non-fatal: pickers show nothing */
    }
    try {
      const data = await endpoints.affiliates.list()
      affiliates.value = data.affiliates || []
    } catch {
      affiliates.value = []
    }
  }

  // ── Admin: form ──────────────────────────────────────────────────────────
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
      stamps: [],
      prizes: [],
    }
  }

  function editRallyForm(r: StampRally): void {
    rallyForm.value = {
      id: r.id,
      title: r.title,
      card_image: r.card_image,
      not_stamped_image: r.not_stamped_image,
      // Stored UTC → this admin's local wall-clock for the datetime-local inputs.
      available_from: utcToDatetimeLocal(r.available_from),
      available_to: utcToDatetimeLocal(r.available_to),
      details: r.details,
      redeem_instructions: r.redeem_instructions,
      stamps: (r.stamps || []).map((s) => ({
        id: s.id,
        affiliate_id: s.affiliate_id ?? null,
        image: s.image,
        password: s.password ?? '',
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
  }

  function cancelRallyForm(): void {
    rallyForm.value = null
  }

  function addStamp(): void {
    rallyForm.value?.stamps.push(blankStamp())
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
    savingRally.value = true
    try {
      // The form holds local datetime-local values; convert the event + per-stamp
      // windows to stored UTC before sending (mirrors the raffle store).
      const payload = {
        ...f,
        available_from: datetimeLocalToUtc(f.available_from),
        available_to: datetimeLocalToUtc(f.available_to),
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
    if (cardStamps.value[rallyId]) return // already loaded
    try {
      const data = await endpoints.stampRallies.detail(rallyId)
      cardStamps.value = { ...cardStamps.value, [rallyId]: data.stamp_rally.stamps || [] }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Pause/resume a stall from the list card's inline panel, updating that card's
   *  loaded stamps and its "active stalls" count without a reload. */
  async function setStampPausedInList(rallyId: number, stampId: number, paused: boolean): Promise<void> {
    try {
      await endpoints.stampRallies.setStampPaused(rallyId, stampId, paused)
      const st = cardStamps.value[rallyId]?.find((s) => s.id === stampId)
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

  // ── Admin: participant cards ─────────────────────────────────────────────
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
        /* clipboard blocked — the per-row copy button still works */
      }
      ui.notify(copied ? 'Card link created and copied to clipboard' : 'Card link created', 'success')
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
      !(await ui.confirm(`Delete the card for ${card.participant_name}? Their stamp record is removed.`, {
        title: 'Delete card',
        confirmText: 'Delete',
      }))
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

  async function copyCardLink(card: StampRallyCard): Promise<void> {
    const url = cardLinkUrl(card)
    try {
      await navigator.clipboard.writeText(url)
      ui.notify('Link copied to clipboard', 'success')
    } catch {
      ui.notify(url, 'info')
    }
  }

  // ── Public: card view + collect ──────────────────────────────────────────
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
    submitting.value = true
    try {
      const data = await endpoints.stampCard.stamp(token, pw)
      publicCard.value = data.card
      lastCollectedId.value = data.collected_stamp_id
      ui.notify('Stamp collected!', 'success')
      if (data.card.completed) {
        ui.notify('Card complete — your prizes are revealed below!', 'success')
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
    cardStamps,
    cardImages,
    stampImages,
    prizeImages,
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
    // admin actions
    loadRallies,
    loadRallyDetail,
    loadRallyLogs,
    viewRally,
    loadFormSources,
    newRallyForm,
    editRallyForm,
    cancelRallyForm,
    addStamp,
    removeStamp,
    addPrize,
    removePrize,
    saveRally,
    deleteRally,
    setRallyStatus,
    loadCardStamps,
    setStampPausedInList,
    setStampPaused,
    createCard,
    deleteCard,
    cardLinkUrl,
    copyCardLink,
    // public actions
    resetPublic,
    loadByToken,
    submitPassword,
  }
})
