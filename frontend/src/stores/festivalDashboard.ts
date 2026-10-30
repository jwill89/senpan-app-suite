/**
 * The festival dashboard: one participant's view of everything filed under a
 * festival - the plan, their stamp card, their garapon draws and their raffle
 * entries - with the actions on each.
 *
 * Two reads make it up. The plan is the same public map every visitor sees; what
 * depends on WHO is looking comes from POST /api/festival-dashboard/{id}, fed
 * whatever this device holds (lib/signups): the card and drawing tokens issued at
 * sign-up, and the name and world the visitor gave. The card and the drum are then
 * handed to the stamp-rally and garapon stores' public state, so the panels the
 * card and drawing pages render work here unchanged, and every action goes through
 * the endpoint that already owns it.
 *
 * Nothing kept on the device authorizes anything. A token the server doesn't
 * recognize for this festival simply comes back unmatched, and is dropped here.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import type { HistoryState } from 'vue-router'
import { endpoints } from '@/lib/endpoints'
import {
  forgetSignup,
  saveFestivalVisit,
  saveRaffleSignup,
  saveRallySignup,
  savedFestivalVisit,
  savedRallySignup,
} from '@/lib/signups'
import type { FestivalDashboard, PublicFestivalMap, RaffleEnterResponse } from '@/types/api'
import { useGaraponsStore } from './garapons'
import { useStampRalliesStore } from './stampRallies'
import { useUiStore } from './ui'

/** A link handed to the dashboard by the card or drawing page. */
export interface FestivalConnect {
  cardToken?: string
  garaponToken?: string
}

const CONNECT_KEY = 'festivalConnect'

/**
 * History state that hands a link to the dashboard without putting it in a URL,
 * where it would reach the web server's access log and the browser's history.
 */
export function festivalConnectState(connect: FestivalConnect): HistoryState {
  return { [CONNECT_KEY]: { ...connect } }
}

/**
 * Takes a hand-off off the current history entry, if one came with it. Removed as
 * it is read, so "forget this device" can't be undone by a reload replaying it.
 */
function takeConnect(): FestivalConnect | null {
  const state = window.history.state as Record<string, unknown> | null
  const raw = state?.[CONNECT_KEY]
  if (!state || !raw || typeof raw !== 'object') return null
  window.history.replaceState({ ...state, [CONNECT_KEY]: null }, '')
  const r = raw as Record<string, unknown>
  const pick = (v: unknown) => (typeof v === 'string' && v ? v : undefined)
  const connect = { cardToken: pick(r.cardToken), garaponToken: pick(r.garaponToken) }
  return connect.cardToken || connect.garaponToken ? connect : null
}

export const useFestivalDashboardStore = defineStore('festivalDashboard', () => {
  const ui = useUiStore()
  const stampRallies = useStampRalliesStore()
  const garapons = useGaraponsStore()

  /** The route segment the festival was opened by (shortcode or id). */
  const festivalKey = ref('')
  const map = ref<PublicFestivalMap | null>(null)
  const dashboard = ref<FestivalDashboard | null>(null)
  const loading = ref(false)
  const notFound = ref(false)
  /** The drawing token in use, when this device holds one the server accepted. */
  const garaponToken = ref('')
  /** When the last read finished (ms), for the page's refresh-on-return. */
  const loadedAt = ref(0)
  const busy = ref(false)
  /** The confirmation of the latest raffle entry made here, keyed by raffle id. */
  const entered = ref<Partial<Record<number, RaffleEnterResponse>>>({})

  const card = computed(() => stampRallies.publicCard)
  const cardToken = computed(() => dashboard.value?.card_token ?? '')
  const visitorName = computed(() => dashboard.value?.participant_name ?? '')
  const visitorWorld = computed(() => dashboard.value?.world ?? '')
  /** A card fixes who the visitor is; without one they may say. */
  const identityFromCard = computed(() => !!dashboard.value?.card)

  /** Occupants whose rally stamp the visitor holds - the map marks them done. */
  const collectedOccupantIds = computed(() => {
    const ids = new Set<number>()
    for (const s of card.value?.stamps ?? []) {
      if (s.collected && s.occupant_id != null) ids.add(s.occupant_id)
    }
    return ids
  })

  const runningRaffles = computed(() => dashboard.value?.raffles.filter((r) => r.running) ?? [])
  /** Raffles the visitor holds tickets in, running or not. */
  const enteredRaffles = computed(() => dashboard.value?.raffles.filter((r) => r.entry) ?? [])

  function reset(): void {
    map.value = null
    dashboard.value = null
    garaponToken.value = ''
    entered.value = {}
    notFound.value = false
    stampRallies.resetPublic()
    garapons.resetPublic()
  }

  /** Opens a festival by its route segment, taking any link handed over with it. */
  async function open(key: string): Promise<void> {
    reset()
    festivalKey.value = key
    loading.value = true
    try {
      map.value = await endpoints.festivalMaps.publicDetail(key)
    } catch {
      notFound.value = true
      loading.value = false
      return
    }
    try {
      await refresh(takeConnect())
    } finally {
      loading.value = false
    }
  }

  /**
   * Re-reads the visitor's standing. With `connect`, a link handed over from the
   * card or drawing page takes the place of whatever this device held for the
   * festival's rally - it is the one the visitor just chose to open.
   */
  async function refresh(
    connect: FestivalConnect | null = null,
    knownRallyId?: number,
  ): Promise<void> {
    const m = map.value
    if (!m) return
    const visit = savedFestivalVisit(m.id)
    const rallyId = knownRallyId ?? visit?.rallyId ?? m.stamp_rally_id ?? 0
    const signup = rallyId ? savedRallySignup(rallyId) : undefined

    let sentCard = signup?.cardToken ?? ''
    let sentDraw = signup?.garaponToken ?? ''
    if (connect?.cardToken && connect.cardToken !== sentCard) {
      // Somebody else's card, or a card from before: a drawing link kept for the
      // old one belongs to that one, not to this.
      sentCard = connect.cardToken
      sentDraw = connect.garaponToken ?? ''
    } else if (connect?.garaponToken) {
      // A drawing link carries its own card, which the server returns.
      sentDraw = connect.garaponToken
      if (connect.garaponToken !== signup?.garaponToken) sentCard = ''
    }

    let data: FestivalDashboard
    try {
      data = await endpoints.festivalDashboard.get(festivalKey.value, {
        card_token: sentCard,
        garapon_token: sentDraw,
        name: visit?.name ?? signup?.name ?? '',
        world: visit?.world ?? signup?.world ?? '',
      })
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return
    }

    dashboard.value = data
    loadedAt.value = Date.now()
    garaponToken.value = data.garapon?.player ? sentDraw : ''
    remember(data, sentCard, sentDraw, rallyId)

    if (data.card) stampRallies.showPublic(data.card)
    else stampRallies.resetPublic()
    // The drum shows whether or not this device can spin it: its prizes are worth
    // seeing before there is a link to draw with.
    if (data.garapon) {
      garapons.showPublic({
        garapon: data.garapon.garapon,
        player: data.garapon.player ?? null,
        draws: data.garapon.draws,
      })
    } else garapons.resetPublic()

    // The tokens were looked up under the rally the map named, and the map names
    // a rally only inside its window. If the server names one this device holds a
    // card for, read again with it.
    if (
      knownRallyId === undefined &&
      !sentCard &&
      !data.card &&
      data.rally &&
      savedRallySignup(data.rally.id)
    ) {
      await refresh(null, data.rally.id)
    }
  }

  /** Keeps what the read confirmed, and drops the links it didn't recognize. */
  function remember(data: FestivalDashboard, sentCard: string, sentDraw: string, rallyId: number) {
    const m = map.value
    if (!m) return
    if (data.card && data.card_token && data.rally) {
      saveRallySignup({
        rallyId: data.rally.id,
        rallyTitle: data.rally.title,
        name: data.participant_name,
        world: data.world,
        cardToken: data.card_token,
        garaponToken: data.garapon?.player ? sentDraw : undefined,
        garaponTitle: data.garapon?.player ? data.garapon.garapon.title : undefined,
      })
    } else if (sentCard && rallyId && sentCard === savedRallySignup(rallyId)?.cardToken) {
      // The card this device kept is one the festival no longer knows - deleted by
      // staff, most likely. Keeping it would hide the sign-up form behind a card
      // that's gone.
      forgetSignup('rally', rallyId)
    }
    if (data.participant_name) {
      saveFestivalVisit({
        mapId: m.id,
        name: data.participant_name,
        world: data.world,
        rallyId: data.rally?.id,
      })
    }
  }

  /** Says who the visitor is, when no card says it for them. */
  async function setVisitor(name: string, world: string): Promise<boolean> {
    const m = map.value
    const n = name.trim()
    const w = world.trim()
    if (!m || !n || !w) {
      ui.notify('Enter your character name and pick your home world', 'error')
      return false
    }
    saveFestivalVisit({ mapId: m.id, name: n, world: w, rallyId: dashboard.value?.rally?.id })
    await refresh()
    return true
  }

  /**
   * Takes this festival's links and name off the device. Offered because the
   * drawing link spends something: a visitor on a friend's phone needs a way not
   * to leave it there. The sign-ups themselves stand.
   */
  async function forgetDevice(): Promise<void> {
    const m = map.value
    if (!m) return
    const rallyId = dashboard.value?.rally?.id ?? savedFestivalVisit(m.id)?.rallyId
    if (rallyId) forgetSignup('rally', rallyId)
    for (const r of dashboard.value?.raffles ?? []) forgetSignup('raffle', r.raffle.id)
    forgetSignup('festival', m.id)
    entered.value = {}
    await refresh()
    ui.notify('This device no longer holds your festival links.', 'success')
  }

  /** Issues the visitor a card for the festival's rally. */
  async function signUp(name: string, world: string, turnstileToken: string): Promise<boolean> {
    const m = map.value
    const rally = dashboard.value?.rally
    if (!m || !rally) return false
    // The stamp-rally store saves the issued links on this device, which is what
    // the dashboard reads them back from.
    const ok = await stampRallies.signUp(rally.id, name, world, turnstileToken)
    if (!ok) return false
    saveFestivalVisit({ mapId: m.id, name: name.trim(), world: world.trim(), rallyId: rally.id })
    stampRallies.resetSignup()
    await refresh()
    ui.notify('Your stamp card is ready.', 'success')
    return true
  }

  /**
   * Finds the visitor's card on the festival's rally by the exact name they signed
   * up with, for a device that never held it. The lookup returns the card, never
   * the drawing link - that is only ever on the device it was issued to.
   */
  async function findCard(name: string, world: string): Promise<boolean> {
    const m = map.value
    const rally = dashboard.value?.rally
    if (!m || !rally || busy.value) return false
    busy.value = true
    try {
      const { entries } = await endpoints.stampSignup.lookup(name.trim(), world.trim())
      const hit = entries.find((e) => e.rally_id === rally.id)
      if (!hit) {
        ui.notify("There's no card on this rally under that name.", 'error')
        return false
      }
      saveRallySignup({
        rallyId: rally.id,
        rallyTitle: rally.title,
        name: hit.participant_name,
        world: hit.world,
        cardToken: hit.card_token,
      })
      saveFestivalVisit({
        mapId: m.id,
        name: hit.participant_name,
        world: hit.world,
        rallyId: rally.id,
      })
      await refresh()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      busy.value = false
    }
  }

  /** Collects a stamp by its stall's password. */
  function stamp(password: string): Promise<boolean> {
    return stampRallies.submitPassword(cardToken.value, password)
  }

  /** Enters a raffle as the visitor. Returns true once the entry is recorded. */
  async function enterRaffle(
    raffleId: number,
    numEntries: number,
    turnstileToken: string,
  ): Promise<boolean> {
    const name = visitorName.value
    const world = visitorWorld.value
    const row = dashboard.value?.raffles.find((r) => r.raffle.id === raffleId)
    if (!row || !name || busy.value) return false
    busy.value = true
    try {
      const res = await endpoints.raffles.enter(raffleId, {
        character_name: name,
        world,
        num_entries: numEntries,
        turnstile_token: turnstileToken || undefined,
      })
      entered.value = { ...entered.value, [raffleId]: res }
      saveRaffleSignup({ raffleId, raffleTitle: row.raffle.title, name, world })
      await refresh()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      busy.value = false
    }
  }

  return {
    festivalKey,
    map,
    dashboard,
    loading,
    notFound,
    garaponToken,
    loadedAt,
    busy,
    entered,
    card,
    cardToken,
    visitorName,
    visitorWorld,
    identityFromCard,
    collectedOccupantIds,
    runningRaffles,
    enteredRaffles,
    open,
    refresh,
    setVisitor,
    forgetDevice,
    signUp,
    findCard,
    stamp,
    enterRaffle,
  }
})
