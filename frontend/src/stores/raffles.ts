/**
 * Raffles store: public browsing + sign-up, and admin management (CRUD,
 * entries, winner picking, image upload). Mirrors all raffle logic from app.js.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { utcToDatetimeLocal, datetimeLocalToUtc, parseServerTimestamp } from '@/lib/datetime'
import type { Raffle, RaffleEnterResponse, RaffleEntry, RaffleForm } from '@/types/api'
import { useUiStore } from './ui'
import { useImagesStore, IMAGE_DIR_RAFFLES } from './images'
import { withLoading } from '@/lib/withLoading'

/**
 * Whether a raffle is enterable by the public right now: it must be `open` and
 * inside its availability window — past `available_from` (if set) and before
 * `available_to` (if set). This mirrors the backend's public list query
 * (store.ListRaffles, non-admin) so the two never disagree.
 *
 * The public `GET /api/raffles` already applies the same window for normal
 * visitors, so this is a second line of defence for the two cases the list
 * query can't cover: a raffle reached by a direct link to its detail page
 * (GetRaffle ignores the window), and an admin browsing the public pages (the
 * list endpoint returns every raffle in admin mode). A raffle keeps its `open`
 * status outside its window, so checking `status` alone is not enough. (Admin
 * tabs intentionally do not use this — admins manage out-of-window raffles.)
 */
export function isRaffleEnterable(r: Raffle): boolean {
  if (r.status !== 'open') return false
  const now = Date.now()
  const startsAt = parseServerTimestamp(r.available_from)
  if (!Number.isNaN(startsAt) && startsAt > now) return false // not yet open
  const endsAt = parseServerTimestamp(r.available_to)
  if (!Number.isNaN(endsAt) && endsAt <= now) return false // already ended
  return true
}

export const useRafflesStore = defineStore('raffles', () => {
  const ui = useUiStore()

  const homeRaffles = ref<Raffle[]>([]) // open raffles for home card visibility
  const raffles = ref<Raffle[]>([])
  const selectedRaffle = ref<Raffle | null>(null)
  const raffleEntries = ref<RaffleEntry[]>([])
  const raffleForm = ref<RaffleForm | null>(null)
  const raffleSignup = ref<{ characterName: string; world: string; numEntries: number }>({
    characterName: '',
    world: '',
    numEntries: 1,
  })
  const raffleSignupResult = ref<RaffleEnterResponse | null>(null)
  /** Cloudflare Turnstile token for the public sign-up (empty when disabled/unset). */
  const signupTurnstileToken = ref('')
  // Admin-only: manually add a player to the selected open raffle.
  const entryAdd = ref<{ characterName: string; world: string; numEntries: number; paid: boolean }>(
    { characterName: '', world: '', numEntries: 1, paid: false },
  )
  const addingEntry = ref(false)
  const raffleWinner = ref<RaffleEntry | null>(null)
  const raffleWinnerEntry = ref<RaffleEntry | null>(null) // public closed view
  const raffleTotalEntryCount = ref(0)
  /** Reusable prize-image paths (the "Raffle" category on the Images page). */
  const prizeImages = ref<string[]>([])
  // In-flight flags driving spinners / button disabling.
  const rafflesLoading = ref(false)
  const detailLoading = ref(false)
  // Monotonic token guarding loadRaffleDetail against a last-write-wins race:
  // two rapid opens each fire a request, and a slow earlier one could otherwise
  // overwrite a newer one. Only the latest request applies its result.
  let detailSeq = 0
  const savingRaffle = ref(false)
  const entering = ref(false)
  const pickingWinner = ref(false)

  // ── Computed ───────────────────────────────────────────────────────────────

  const openRaffles = computed(() => raffles.value.filter((r) => r.status === 'open'))
  const closedRaffles = computed(() => raffles.value.filter((r) => r.status === 'closed'))

  /** Public: is the currently-viewed raffle still enterable (open + not ended)? */
  const selectedRaffleEnterable = computed(
    () => selectedRaffle.value !== null && isRaffleEnterable(selectedRaffle.value),
  )

  // ── Load ─────────────────────────────────────────────────────────────────

  async function loadRaffles(): Promise<void> {
    await withLoading(rafflesLoading, async () => {
      const data = await endpoints.raffles.list()
      raffles.value = data.raffles
    })
  }

  /** Preloads open raffles (home page card visibility). */
  async function loadHomeRaffles(): Promise<void> {
    try {
      const data = await endpoints.raffles.list()
      homeRaffles.value = data.raffles.filter(isRaffleEnterable)
    } catch {
      /* silent */
    }
  }

  async function loadRaffleDetail(id: number): Promise<void> {
    const reqId = ++detailSeq
    detailLoading.value = true
    try {
      const data = await endpoints.raffles.detail(id)
      if (reqId !== detailSeq) return // a newer load superseded this one
      selectedRaffle.value = data.raffle
      raffleEntries.value = data.entries || []
      raffleWinner.value = null
      if (data.raffle.winner_entry_id && raffleEntries.value.length) {
        raffleWinner.value =
          raffleEntries.value.find((e) => e.id === data.raffle.winner_entry_id) || null
      }
    } catch (e) {
      if (reqId === detailSeq) ui.notify((e as Error).message, 'error')
    } finally {
      if (reqId === detailSeq) detailLoading.value = false
    }
  }

  /** Admin: open a raffle's detail view. */
  function viewRaffle(raffle: Raffle): void {
    selectedRaffle.value = raffle
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleEntries.value = []
    raffleWinner.value = null
    resetEntryAdd()
    void loadRaffleDetail(raffle.id)
  }

  /** Clears the admin "add entry" form. */
  function resetEntryAdd(): void {
    entryAdd.value = { characterName: '', world: '', numEntries: 1, paid: false }
  }

  /** Public: open a raffle's detail view (loads winner + total entries). */
  function viewPublicRaffle(raffle: Raffle): void {
    selectedRaffle.value = raffle
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    endpoints.raffles
      .detail(raffle.id)
      .then((data) => {
        selectedRaffle.value = data.raffle
        raffleTotalEntryCount.value = data.total_entries || 0
        if (data.winner_entry) raffleWinnerEntry.value = data.winner_entry
      })
      .catch(() => {})
  }

  /**
   * Public: load a raffle's detail by id (used when navigating directly to
   * /raffles/:id, e.g. on refresh or a shared link). Resets the sign-up state,
   * then fetches the raffle + winner + total entry count. Returns true on
   * success, false on failure (so the view can redirect back to the list).
   */
  async function loadPublicRaffleById(id: number): Promise<boolean> {
    selectedRaffle.value = null
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    detailLoading.value = true
    try {
      const data = await endpoints.raffles.detail(id)
      selectedRaffle.value = data.raffle
      raffleTotalEntryCount.value = data.total_entries || 0
      if (data.winner_entry) raffleWinnerEntry.value = data.winner_entry
      return true
    } catch {
      return false
    } finally {
      detailLoading.value = false
    }
  }

  // ── Admin form ─────────────────────────────────────────────────────────────

  function newRaffleForm(): void {
    raffleForm.value = {
      id: 0,
      title: '',
      description: '',
      rules: '',
      max_entries: 1,
      signup_instructions: '',
      cost_per_entry: 0,
      available_from: '',
      available_to: '',
      prize_image: '',
    }
  }

  function editRaffleForm(raffle: Raffle): void {
    // Availability dates are stored as UTC; convert to local time so the
    // datetime-local inputs show the correct wall-clock for *this* admin's
    // timezone (a window set by an admin in another zone reads correctly).
    raffleForm.value = {
      ...(raffle as unknown as RaffleForm),
      available_from: utcToDatetimeLocal(raffle.available_from),
      available_to: utcToDatetimeLocal(raffle.available_to),
    }
  }

  /**
   * Seed a brand-new raffle form from an existing (e.g. closed) raffle — copies
   * the reusable content (title, markdown bodies, limits, cost, prize image) but
   * starts with a cleared availability window and a zero id, so saving creates a
   * fresh open raffle rather than editing the original.
   */
  function copyRaffleForm(raffle: Raffle): void {
    raffleForm.value = {
      id: 0,
      title: raffle.title,
      description: raffle.description,
      rules: raffle.rules,
      max_entries: raffle.max_entries,
      signup_instructions: raffle.signup_instructions,
      cost_per_entry: raffle.cost_per_entry,
      available_from: '',
      available_to: '',
      prize_image: raffle.prize_image,
    }
  }

  function cancelRaffleForm(): void {
    raffleForm.value = null
  }

  /** Saves the raffle form. Returns true on success (caller navigates). */
  async function saveRaffle(): Promise<boolean> {
    if (!raffleForm.value) return false
    const f = raffleForm.value
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    savingRaffle.value = true
    try {
      // The form holds local datetime-local values; convert the availability
      // window to UTC so the stored instant is timezone-unambiguous.
      const payload = {
        ...f,
        available_from: datetimeLocalToUtc(f.available_from),
        available_to: datetimeLocalToUtc(f.available_to),
      }
      if (f.id) {
        await endpoints.raffles.update(f.id, payload)
        ui.notify('Raffle updated', 'success')
      } else {
        await endpoints.raffles.create(payload)
        ui.notify('Raffle created', 'success')
      }
      raffleForm.value = null
      await loadRaffles()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingRaffle.value = false
    }
  }

  async function deleteRaffle(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this raffle and all its entries?', {
        title: 'Delete raffle',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.raffles.delete(id)
      raffles.value = raffles.value.filter((r) => r.id !== id)
      if (selectedRaffle.value && selectedRaffle.value.id === id) selectedRaffle.value = null
      ui.notify('Raffle deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Loads the reusable prize images (the "Raffle" category) for the form picker.
   *  Prize images are stored as root-relative paths, so the picker uses `.path`. */
  async function loadPrizeImages(): Promise<void> {
    try {
      const images = useImagesStore()
      await images.loadImages(IMAGE_DIR_RAFFLES)
      prizeImages.value = images.imagesByDir[IMAGE_DIR_RAFFLES].map((i) => i.path)
    } catch {
      /* non-fatal: the picker just shows nothing */
    }
  }

  // ── Public sign-up ─────────────────────────────────────────────────────────

  /**
   * The entry count clamped to a whole number in [1, max_entries]. The raw
   * `numEntries` can be out of range or non-integer (typed value, stepper, or a
   * cleared field), so both the live cost preview and the submitted request go
   * through this — the displayed total can never disagree with what's sent, and
   * the server's own bound is never the first line of defence.
   */
  function clampedEntries(): number {
    if (!selectedRaffle.value) return 1
    const max = Math.max(1, Math.floor(selectedRaffle.value.max_entries || 1))
    const raw = Math.floor(raffleSignup.value.numEntries || 1)
    return Math.min(Math.max(raw, 1), max)
  }

  /** Writes the clamped entry count back to the field (call on input blur/change). */
  function clampSignupEntries(): void {
    raffleSignup.value.numEntries = clampedEntries()
  }

  function raffleTotalCost(): number {
    if (!selectedRaffle.value) return 0
    return clampedEntries() * selectedRaffle.value.cost_per_entry
  }

  async function enterRaffle(): Promise<void> {
    if (!selectedRaffle.value) return
    const s = raffleSignup.value
    if (!s.characterName.trim() || !s.world.trim()) {
      ui.notify('Character name and world are required', 'error')
      return
    }
    clampSignupEntries()
    entering.value = true
    try {
      const data = await endpoints.raffles.enter(selectedRaffle.value.id, {
        character_name: s.characterName.trim(),
        world: s.world.trim(),
        num_entries: s.numEntries,
        turnstile_token: signupTurnstileToken.value || undefined,
      })
      raffleSignupResult.value = data
      ui.notify(data.message, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      // Turnstile tokens are single-use — clear it so the widget re-issues one.
      signupTurnstileToken.value = ''
      entering.value = false
    }
  }

  // ── Admin entries ──────────────────────────────────────────────────────────

  /**
   * Admin: manually add a player to the selected open raffle (optionally already
   * paid). The entry count is clamped to [1, max_entries] to match the field's
   * bound; the server enforces the same limit. On success the form resets and the
   * detail view reloads so the entries table + counts reflect the new entry.
   */
  async function addRaffleEntry(): Promise<void> {
    if (!selectedRaffle.value) return
    const f = entryAdd.value
    if (!f.characterName.trim() || !f.world.trim()) {
      ui.notify('Character name and world are required', 'error')
      return
    }
    const max = Math.max(1, Math.floor(selectedRaffle.value.max_entries || 1))
    const num = Math.min(Math.max(Math.floor(f.numEntries || 1), 1), max)
    addingEntry.value = true
    try {
      await endpoints.raffles.addEntry(selectedRaffle.value.id, {
        character_name: f.characterName.trim(),
        world: f.world.trim(),
        num_entries: num,
        paid: f.paid,
      })
      ui.notify('Entry added', 'success')
      resetEntryAdd()
      await loadRaffleDetail(selectedRaffle.value.id)
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      addingEntry.value = false
    }
  }

  async function toggleEntryPaid(entry: RaffleEntry): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      await endpoints.raffles.markEntryPaid(selectedRaffle.value.id, entry.id, !entry.paid)
      entry.paid = !entry.paid
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteEntry(entry: RaffleEntry): Promise<void> {
    if (!selectedRaffle.value) return
    if (!(await ui.confirm('Delete this entry?', { title: 'Delete entry', confirmText: 'Delete' })))
      return
    try {
      await endpoints.raffles.deleteEntry(selectedRaffle.value.id, entry.id)
      raffleEntries.value = raffleEntries.value.filter((e) => e.id !== entry.id)
      ui.notify('Entry deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function pickRaffleWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    pickingWinner.value = true
    try {
      const data = await endpoints.raffles.pickWinner(selectedRaffle.value.id)
      raffleWinner.value = data.winner
      selectedRaffle.value.winner_entry_id = data.winner.id
      ui.notify('Winner picked!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      pickingWinner.value = false
    }
  }

  async function verifyRaffleWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    try {
      await endpoints.raffles.verifyWinner(selectedRaffle.value.id)
      selectedRaffle.value.status = 'closed'
      ui.notify('Winner verified! Raffle closed.', 'success')
      await loadRaffles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function pickAnotherWinner(): Promise<void> {
    if (!selectedRaffle.value) return
    pickingWinner.value = true
    try {
      const data = await endpoints.raffles.pickAnotherWinner(selectedRaffle.value.id)
      raffleWinner.value = data.winner
      selectedRaffle.value.winner_entry_id = data.winner.id
      ui.notify('New winner picked!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      pickingWinner.value = false
    }
  }

  return {
    homeRaffles,
    raffles,
    selectedRaffle,
    raffleEntries,
    raffleForm,
    raffleSignup,
    raffleSignupResult,
    signupTurnstileToken,
    entryAdd,
    addingEntry,
    raffleWinner,
    raffleWinnerEntry,
    raffleTotalEntryCount,
    prizeImages,
    rafflesLoading,
    detailLoading,
    savingRaffle,
    entering,
    pickingWinner,
    openRaffles,
    closedRaffles,
    selectedRaffleEnterable,
    loadRaffles,
    loadHomeRaffles,
    loadRaffleDetail,
    viewRaffle,
    viewPublicRaffle,
    loadPublicRaffleById,
    newRaffleForm,
    editRaffleForm,
    copyRaffleForm,
    cancelRaffleForm,
    saveRaffle,
    deleteRaffle,
    loadPrizeImages,
    raffleTotalCost,
    clampSignupEntries,
    enterRaffle,
    addRaffleEntry,
    toggleEntryPaid,
    deleteEntry,
    pickRaffleWinner,
    verifyRaffleWinner,
    pickAnotherWinner,
  }
})
