/**
 * Raffles store: public browsing + sign-up, and admin management (CRUD,
 * entries, winner picking, image upload). Mirrors all raffle logic from app.js.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { utcToDatetimeLocal, datetimeLocalToUtc, parseServerTimestamp } from '@/lib/datetime'
import type {
  FestivalMap,
  FestivalStallOccupant,
  Raffle,
  RaffleEnterResponse,
  RaffleEntry,
  RaffleForm,
  RaffleLookupEntry,
  RaffleMode,
} from '@/types/api'
import { useUiStore } from './ui'
import { withLoading } from '@/lib/withLoading'
import { saveRaffleSignup } from '@/lib/signups'
import { RAFFLE_MAX_ENTRIES, RAFFLE_LOOKUP_MIN_QUERY } from '@/lib/constants'

/**
 * Whether a raffle is enterable by the public right now: it must be `open` and
 * inside its availability window - past `available_from` (if set) and before
 * `available_to` (if set). This mirrors the backend's public list query
 * (store.ListRaffles, non-admin) so the two never disagree.
 *
 * The public `GET /api/raffles` already applies the same window for normal
 * visitors, so this is a second line of defence for the two cases the list
 * query can't cover: a raffle reached by a direct link to its detail page
 * (GetRaffle ignores the window), and an admin browsing the public pages (the
 * list endpoint returns every raffle in admin mode). A raffle keeps its `open`
 * status outside its window, so checking `status` alone is not enough. (Admin
 * tabs intentionally do not use this - admins manage out-of-window raffles.)
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

/**
 * A raffle's entry mode, defaulting anything unrecognized to 'single'. Raffles
 * saved before entry modes existed carry an empty string, and that has always
 * meant "flat cost_per_entry" - so an unknown value must never read as free or
 * as sign-ups-disabled. Mirrors the backend's NormalizeRaffleMode.
 */
export function raffleMode(r: Pick<Raffle, 'entry_mode'>): RaffleMode {
  return r.entry_mode === 'details' || r.entry_mode === 'custom' ? r.entry_mode : 'single'
}

/** Whether this raffle takes sign-ups through the site (false = details only). */
export function raffleAcceptsSignups(r: Pick<Raffle, 'entry_mode'>): boolean {
  return raffleMode(r) !== 'details'
}

/**
 * The price of the n-th entry (1-based). Tickets past the end of a custom ladder
 * cost nothing - max_entries is pinned to the ladder length server-side, so that
 * only guards a hand-edited raffle rather than a reachable state.
 */
export function raffleTicketCost(r: Raffle, n: number): number {
  switch (raffleMode(r)) {
    case 'details':
      return 0
    case 'custom':
      return n >= 1 && n <= r.tier_costs.length ? r.tier_costs[n - 1] : 0
    default:
      return r.cost_per_entry
  }
}

/**
 * What holding `n` entries costs in total: n x cost_per_entry in 'single' mode,
 * the sum of the first n rungs in 'custom' mode, 0 for details-only. Mirrors the
 * backend's Raffle.EntryCost so the previewed total can never disagree with the
 * one the server reports back.
 */
export function raffleEntryCost(r: Raffle, n: number): number {
  if (n < 1) return 0
  const mode = raffleMode(r)
  if (mode === 'details') return 0
  if (mode === 'single') return n * r.cost_per_entry
  // Walk the LADDER, not the ticket count - only rungs that exist carry a price,
  // so a raffle whose max_entries drifted above its ladder costs one pass over
  // the tiers rather than one per ticket.
  const rungs = Math.min(n, r.tier_costs.length)
  let total = 0
  for (let i = 1; i <= rungs; i++) total += raffleTicketCost(r, i)
  return total
}

/** Whether this raffle charges anything at all (drives the cost UI). */
export function raffleHasCost(r: Raffle): boolean {
  const mode = raffleMode(r)
  if (mode === 'details') return false
  if (mode === 'custom') return r.tier_costs.some((c) => c > 0)
  return r.cost_per_entry > 0
}

/** Ladder rungs a one-line summary spells out before it switches to a total. */
const LADDER_LABEL_LIMIT = 4

/**
 * One-line price summary for a raffle card: "50,000 gil per entry" for a flat
 * cost, or the ladder itself ("50,000 / 100,000 / 150,000 gil") so a browsing
 * player sees what a second and third entry actually cost. Empty when the raffle
 * charges nothing.
 *
 * A long ladder is summarised instead of spelled out - this lands in a card and
 * in an admin badge, and a raffle may carry up to 50 rungs, which would run off
 * the tile. The full ladder is always on the raffle's own page.
 */
export function raffleCostLabel(r: Raffle): string {
  if (!raffleHasCost(r)) return ''
  if (raffleMode(r) !== 'custom') return `${r.cost_per_entry.toLocaleString()} gil per entry`
  const tiers = r.tier_costs
  if (tiers.length > LADDER_LABEL_LIMIT) {
    const all = raffleEntryCost(r, tiers.length).toLocaleString()
    return `${tiers.length} entries, ${all} gil for all`
  }
  return `${tiers.map((c) => c.toLocaleString()).join(' / ')} gil`
}

/**
 * How far an entry has settled. Entries merge per character+world, so a row that
 * was paid can gain tickets later - `paid_entries` is what makes that middle
 * state visible instead of the new tickets hiding behind the old paid flag.
 */
export function entryPaymentState(e: RaffleEntry): 'unpaid' | 'partial' | 'paid' {
  if (e.paid_entries <= 0) return 'unpaid'
  return e.paid_entries < e.num_entries ? 'partial' : 'paid'
}

/** Sticker price of every ticket on an entry, before any waiver. */
export function entryTicketPrice(r: Raffle, e: RaffleEntry): number {
  return raffleEntryCost(r, e.num_entries)
}

/**
 * Gil this entry actually handed over: the price of the tickets it has settled,
 * less everything waived on it. Floored at zero so an over-generous waiver reads
 * as "collected nothing" rather than eating into another entry's contribution.
 * Mirrors the backend's Raffle.AmountCollected.
 */
export function entryAmountCollected(r: Raffle, e: RaffleEntry): number {
  return Math.max(0, raffleEntryCost(r, e.paid_entries) - e.amount_waived)
}

/**
 * Sticker price of the tickets this entry has NOT settled. Waivers aren't
 * predicted - one is chosen when a payment is recorded - so unsettled tickets
 * are quoted at full price.
 */
export function entryAmountOutstanding(r: Raffle, e: RaffleEntry): number {
  return Math.max(0, raffleEntryCost(r, e.num_entries) - raffleEntryCost(r, e.paid_entries))
}

export const useRafflesStore = defineStore('raffles', () => {
  // -- Festival Map linkage -------------------------------------------------
  // A raffle can be filed under a Festival Map (the same grouping a Stamp Rally
  // gets) and pinned to one stall on it, so the stall's panel on the public plan
  // links to the raffle while it is running.
  /** Maps a raffle can be filed under. */
  const festivalMaps = ref<FestivalMap[]>([])
  /** The linked map's pitch occupants, flattened - the stall list to pick from. */
  const mapStalls = ref<FestivalStallOccupant[]>([])

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
  // Public "have I already entered?" search. `null` means no search has run yet,
  // which the page must not present as "nothing matched" - an empty ARRAY is the
  // fruitless search.
  const entryLookupResults = ref<RaffleLookupEntry[] | null>(null)
  const entryLookupTruncated = ref(false)
  const entryLookupLoading = ref(false)
  // Admin-only: manually add a player to the selected open raffle.
  const entryAdd = ref<{
    characterName: string
    world: string
    numEntries: number
    paid: boolean
    amountWaived: number
  }>({ characterName: '', world: '', numEntries: 1, paid: false, amountWaived: 0 })
  const addingEntry = ref(false)
  // In flight while a settlement is being recorded (drives the modal's button).
  const settlingEntry = ref(false)
  const raffleWinner = ref<RaffleEntry | null>(null)
  const raffleWinnerEntry = ref<RaffleEntry | null>(null) // public closed view
  const raffleTotalEntryCount = ref(0)
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

  // -- Computed ---------------------------------------------------------------

  const openRaffles = computed(() => raffles.value.filter((r) => r.status === 'open'))
  const closedRaffles = computed(() => raffles.value.filter((r) => r.status === 'closed'))

  /**
   * Public: can the currently-viewed raffle be entered here right now? It must be
   * open, inside its window, AND take sign-ups through the site - a details-only
   * raffle is browsable but has no form.
   */
  const selectedRaffleEnterable = computed(
    () =>
      selectedRaffle.value !== null &&
      isRaffleEnterable(selectedRaffle.value) &&
      raffleAcceptsSignups(selectedRaffle.value),
  )

  // -- Load -----------------------------------------------------------------

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
    entryAdd.value = { characterName: '', world: '', numEntries: 1, paid: false, amountWaived: 0 }
  }

  /** Public: open a raffle's detail view (loads winner + total entries). */
  function viewPublicRaffle(raffle: Raffle): void {
    selectedRaffle.value = raffle
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    clearEntryLookup()
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
    const reqId = ++detailSeq
    selectedRaffle.value = null
    raffleSignup.value = { characterName: '', world: '', numEntries: 1 }
    raffleSignupResult.value = null
    raffleWinnerEntry.value = null
    raffleTotalEntryCount.value = 0
    clearEntryLookup()
    detailLoading.value = true
    try {
      const data = await endpoints.raffles.detail(id)
      if (reqId !== detailSeq) return true // superseded; the newer load owns the state
      selectedRaffle.value = data.raffle
      raffleTotalEntryCount.value = data.total_entries || 0
      if (data.winner_entry) raffleWinnerEntry.value = data.winner_entry
      return true
    } catch {
      // A superseded request must not report failure (which would redirect the
      // view back to the list) - the newer load is in charge of the outcome.
      return reqId !== detailSeq
    } finally {
      if (reqId === detailSeq) detailLoading.value = false
    }
  }

  // -- Admin form -------------------------------------------------------------

  function newRaffleForm(): void {
    raffleForm.value = {
      id: 0,
      title: '',
      description: '',
      rules: '',
      max_entries: 1,
      signup_instructions: '',
      entry_mode: 'single',
      cost_per_entry: 0,
      tier_costs: [0],
      available_from: '',
      available_to: '',
      prize_image: '',
      pay_image: '',
      festival_map_id: null,
      occupant_id: null,
    }
  }

  function editRaffleForm(raffle: Raffle): void {
    // Availability dates are stored as UTC; convert to local time so the
    // datetime-local inputs show the correct wall-clock for *this* admin's
    // timezone (a window set by an admin in another zone reads correctly).
    raffleForm.value = {
      ...(raffle as unknown as RaffleForm),
      entry_mode: raffleMode(raffle),
      // The tier editor always needs a row to type into, even for a raffle
      // saved in another mode (whose stored ladder is empty by design).
      tier_costs: raffle.tier_costs.length ? [...raffle.tier_costs] : [0],
      available_from: utcToDatetimeLocal(raffle.available_from),
      available_to: utcToDatetimeLocal(raffle.available_to),
    }
  }

  /**
   * Seed a brand-new raffle form from an existing (e.g. closed) raffle - copies
   * the reusable content (title, markdown bodies, limits, cost, prize image) but
   * starts with a cleared availability window and a zero id, so saving creates a
   * fresh open raffle rather than editing the original.
   */
  function copyRaffleForm(raffle: Raffle): void {
    raffleForm.value = {
      id: 0,
      // Marked as a copy so saving it unchanged can't leave two identically named
      // raffles in the list - matches the rally + garapon duplicates.
      title: `${raffle.title} (Copy)`,
      description: raffle.description,
      rules: raffle.rules,
      max_entries: raffle.max_entries,
      signup_instructions: raffle.signup_instructions,
      entry_mode: raffleMode(raffle),
      cost_per_entry: raffle.cost_per_entry,
      tier_costs: raffle.tier_costs.length ? [...raffle.tier_costs] : [0],
      available_from: '',
      available_to: '',
      prize_image: raffle.prize_image,
      pay_image: raffle.pay_image,
      // The festival link points at the map the ORIGINAL raffle ran at. A copy is
      // almost always next year's, so keeping it would quietly file the new raffle
      // under the finished festival - the same reason a garapon copy drops its
      // stamp_rally_id. Pick the festival again on the copy.
      festival_map_id: null,
      occupant_id: null,
    }
  }

  function cancelRaffleForm(): void {
    raffleForm.value = null
  }

  /**
   * Loads the festival maps a raffle can be filed under, plus - when the form is
   * already linked to one - that map's stalls. A closed map is deliberately still
   * offered: a raffle is often authored alongside the map, and dropping the link
   * the moment the festival closed would silently unpin it.
   */
  async function loadFormSources(): Promise<void> {
    try {
      festivalMaps.value = (await endpoints.festivalMaps.list()).maps
    } catch {
      // No festival-map permission (or none exist) - the link select stays empty
      // and the raffle simply belongs to no festival.
      festivalMaps.value = []
    }
    await loadMapStalls(raffleForm.value?.festival_map_id ?? null)
  }

  /** Loads a festival map's stalls for the stall select (null clears them). */
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
   * Files the form's raffle under a festival map (or none) and reloads its stalls.
   * Switching maps clears the assigned stall: the id belongs to the map that was
   * linked before, and the server would drop it on save anyway - better the admin
   * sees the empty select now than a silent reset afterwards.
   */
  async function setFestivalMap(mapId: number | null): Promise<void> {
    const f = raffleForm.value
    if (!f || f.festival_map_id === mapId) return
    f.festival_map_id = mapId
    f.occupant_id = null
    await loadMapStalls(mapId)
  }

  /**
   * Appends an entry-cost rung to the custom ladder, seeded from the last one.
   * The ladder length is the per-player allowance, so it stops at the same cap
   * the server enforces.
   */
  function addRaffleTier(): void {
    const f = raffleForm.value
    if (!f || f.tier_costs.length >= RAFFLE_MAX_ENTRIES) return
    f.tier_costs.push(f.tier_costs[f.tier_costs.length - 1] ?? 0)
  }

  /** Removes one rung; the ladder always keeps at least one row to type into. */
  function removeRaffleTier(index: number): void {
    const f = raffleForm.value
    if (!f || f.tier_costs.length <= 1) return
    f.tier_costs.splice(index, 1)
  }

  /** Saves the raffle form. Returns true on success (caller navigates). */
  async function saveRaffle(): Promise<boolean> {
    if (!raffleForm.value) return false
    const f = raffleForm.value
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    // A cleared number input leaves NaN behind, which would serialize as null and
    // be rejected by the server - normalize the costs the chosen mode actually
    // uses, and drop the ones it doesn't so a mode switch leaves no shadow price.
    const tiers = f.tier_costs.map((c) => (Number.isFinite(c) ? c : 0))
    if (f.entry_mode === 'custom' && tiers.some((c) => c < 0)) {
      ui.notify('Entry costs cannot be negative', 'error')
      return false
    }
    if (f.entry_mode === 'single' && (!Number.isFinite(f.cost_per_entry) || f.cost_per_entry < 0)) {
      ui.notify('Cost per entry must be a non-negative number', 'error')
      return false
    }
    savingRaffle.value = true
    try {
      // The form holds local datetime-local values; convert the availability
      // window to UTC so the stored instant is timezone-unambiguous.
      const payload = {
        ...f,
        cost_per_entry: f.entry_mode === 'single' ? f.cost_per_entry : 0,
        tier_costs: f.entry_mode === 'custom' ? tiers : [],
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

  // -- Public sign-up ---------------------------------------------------------

  /**
   * The entry count clamped to a whole number in [1, max_entries]. The raw
   * `numEntries` can be out of range or non-integer (typed value, stepper, or a
   * cleared field), so both the live cost preview and the submitted request go
   * through this - the displayed total can never disagree with what's sent, and
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
    return raffleEntryCost(selectedRaffle.value, clampedEntries())
  }

  /**
   * Public: find this raffle's entries whose character name contains `name`, so a
   * returning entrant can copy back the exact spelling they used. Entries merge on
   * character+world, so a different spelling silently starts a second entry and
   * splits their tickets.
   *
   * The server needs at least two characters; a shorter query is rejected here
   * rather than sent, so the page can say so without a round trip.
   */
  async function lookupEntries(name: string): Promise<void> {
    if (!selectedRaffle.value) return
    const trimmed = name.trim()
    // Count CODE POINTS, matching the server's []rune check - a two-character
    // name in a script outside the BMP must not be rejected here for being
    // "one character" by UTF-16 measure.
    if (Array.from(trimmed).length < RAFFLE_LOOKUP_MIN_QUERY) {
      ui.notify(
        `Enter at least ${RAFFLE_LOOKUP_MIN_QUERY} characters of your character name`,
        'error',
      )
      return
    }
    entryLookupLoading.value = true
    try {
      const data = await endpoints.raffles.lookup(selectedRaffle.value.id, trimmed)
      entryLookupResults.value = data.entries
      entryLookupTruncated.value = data.truncated
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      // Back to "no search has run" - a failed search must not read as "nothing
      // matched", which would tell someone they hadn't entered when they had.
      entryLookupResults.value = null
      entryLookupTruncated.value = false
    } finally {
      entryLookupLoading.value = false
    }
  }

  /** Drops the search results (leaving the raffle page, or starting over). */
  function clearEntryLookup(): void {
    entryLookupResults.value = null
    entryLookupTruncated.value = false
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
      // Entries merge on character+world, so the exact spelling is what a repeat
      // visit needs: a different one silently starts a second entry and splits
      // this person's tickets. The server still owns the per-player cap.
      saveRaffleSignup({
        raffleId: selectedRaffle.value.id,
        raffleTitle: selectedRaffle.value.title,
        name: s.characterName.trim(),
        world: s.world.trim(),
      })
      ui.notify(data.message, 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      // Turnstile tokens are single-use - clear it so the widget re-issues one.
      signupTurnstileToken.value = ''
      entering.value = false
    }
  }

  // -- Admin entries ----------------------------------------------------------

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
        // Only meaningful alongside paid; the server ignores it otherwise.
        amount_waived: f.paid && Number.isFinite(f.amountWaived) ? Math.max(0, f.amountWaived) : 0,
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

  /**
   * Record a settlement on an entry, or clear one.
   *
   * `amountWaived` is what is being forgiven RIGHT NOW - the server adds it to
   * whatever the entry has already had waived, so this must never carry a running
   * total. `paidEntries` is how many of the entry's tickets the payment covers,
   * 0 meaning all of them. Clearing (`paid = false`) resets the entry to nothing
   * settled and nothing waived. The updated row comes back from the server, which
   * owns both derived values.
   */
  async function setEntryPaid(
    entry: RaffleEntry,
    paid: boolean,
    amountWaived = 0,
    paidEntries = 0,
  ): Promise<boolean> {
    if (!selectedRaffle.value) return false
    settlingEntry.value = true
    try {
      const data = await endpoints.raffles.markEntryPaid(
        selectedRaffle.value.id,
        entry.id,
        paid,
        paid ? amountWaived : 0,
        paid ? paidEntries : 0,
      )
      Object.assign(entry, data.entry)
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      settlingEntry.value = false
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

  /**
   * Close or reopen the selected raffle without picking a winner - what a raffle
   * that drew no entries needs, and what a details-only one always needs, since
   * its draw happens outside the app. Closing asks first: it takes the raffle off
   * the public list, and an accidental close on a live raffle is disruptive.
   */
  async function setRaffleClosed(closed: boolean): Promise<void> {
    const raffle = selectedRaffle.value
    if (!raffle) return
    if (
      closed &&
      !(await ui.confirm('Close this raffle? It will stop appearing on the public list.', {
        title: 'Close raffle',
        confirmText: 'Close',
      }))
    )
      return
    try {
      await endpoints.raffles.setStatus(raffle.id, closed)
      raffle.status = closed ? 'closed' : 'open'
      ui.notify(closed ? 'Raffle closed' : 'Raffle reopened', 'success')
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
    entryLookupResults,
    entryLookupTruncated,
    entryLookupLoading,
    entryAdd,
    addingEntry,
    settlingEntry,
    raffleWinner,
    raffleWinnerEntry,
    raffleTotalEntryCount,
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
    festivalMaps,
    mapStalls,
    loadFormSources,
    setFestivalMap,
    newRaffleForm,
    editRaffleForm,
    copyRaffleForm,
    cancelRaffleForm,
    addRaffleTier,
    removeRaffleTier,
    saveRaffle,
    deleteRaffle,
    raffleTotalCost,
    clampSignupEntries,
    lookupEntries,
    clearEntryLookup,
    enterRaffle,
    addRaffleEntry,
    setEntryPaid,
    deleteEntry,
    pickRaffleWinner,
    verifyRaffleWinner,
    setRaffleClosed,
    pickAnotherWinner,
  }
})
