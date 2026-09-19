/**
 * Festival Map store: admin management (a floor plan with stalls placed on the
 * base image, plus its publish status) and the public read side (the published
 * maps and one map's interactive view).
 *
 * Structurally the stamp-rally store's sibling - an event owning positioned
 * sub-entities, authored in a visual editor - with two differences: a map is
 * published rather than opened/closed, and its public side is a browsable page
 * rather than a per-participant token.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type {
  Affiliate,
  EventTimeForm,
  FestivalMap,
  FestivalMapForm,
  FestivalMapStatus,
  FestivalStallForm,
  FestivalStallOccupantForm,
  Placement,
  StallShape,
  PublicFestivalMap,
  PublicFestivalMapSummary,
  StallType,
} from '@/types/api'
import { datetimeLocalToUtc, utcToDatetimeLocal } from '@/lib/datetime'
import { isCircle, stallTypeMeta } from '@/lib/festivalmap'
import { nextUid } from '@/lib/uid'
import { withLoading } from '@/lib/withLoading'
import { useUiStore } from './ui'

/**
 * A fresh placement for a new stall: centred, modest size, no rotation. A CIRCLE
 * starts narrower because its height follows its width on screen (see
 * `stallStyle`), so an 8%-wide disc on a wide floor plan is already about as tall
 * as a 13%-tall rectangle - at the rectangle's 14% it would dominate the map.
 */
function defaultPlacement(shape: StallShape): Placement {
  return isCircle(shape)
    ? { x: 46, y: 44, width: 8, height: 8, rotation: 0 }
    : { x: 43, y: 44, width: 14, height: 8, rotation: 0 }
}

/** A blank occupant of the given type - the business standing in a pitch. */
export function blankOccupant(type: StallType): FestivalStallOccupantForm {
  return {
    id: 0,
    affiliate_id: null,
    title: '',
    description: '',
    event_carrd: '',
    stall_type: type,
    type_label: '',
    times: [],
    _uid: nextUid(),
  }
}

/**
 * A blank pitch of the given type, pre-shaped the way that type is usually drawn
 * and holding one occupant - the common case, a booth that keeps the same
 * business all festival. More occupants are added when it changes hands by day.
 */
function blankStall(type: StallType): FestivalStallForm {
  const meta = stallTypeMeta(type)
  return {
    id: 0,
    shape: meta.shape,
    // Left empty so the pitch follows its occupant's type color until someone
    // picks one - changing the type then still restyles it, which a copied
    // literal wouldn't.
    color: '',
    // Also empty: the selection halo falls back to the app's highlight until an
    // admin picks a colour that reads against this pitch's fill.
    selection_color: '',
    // Empty means white, which reads on most fills; an admin picks another where
    // it does not.
    text_color: '',
    placement: defaultPlacement(meta.shape),
    occupants: [blankOccupant(type)],
    _uid: nextUid(),
  }
}

/** A blank datetime range for the festival/stall time repeaters. */
export function blankEventTime(): EventTimeForm {
  return { label: '', start: '', end: '', _uid: nextUid() }
}

/** Stored UTC ranges -> the local wall-clock strings the datetime inputs bind to. */
function timesToForm(times: { label: string; start: string; end: string }[]): EventTimeForm[] {
  return times.map((t) => ({
    label: t.label,
    start: utcToDatetimeLocal(t.start),
    end: utcToDatetimeLocal(t.end),
    _uid: nextUid(),
  }))
}

/**
 * Form ranges -> the stored UTC shape, dropping any row with no start (an
 * unstarted range says nothing) and stripping the client-only `_uid`. The server
 * applies the same rule; doing it here keeps the payload honest either way.
 */
function timesToPayload(times: EventTimeForm[]): { label: string; start: string; end: string }[] {
  return times
    .filter((t) => t.start.trim())
    .map((t) => ({
      label: t.label.trim(),
      start: datetimeLocalToUtc(t.start),
      end: datetimeLocalToUtc(t.end),
    }))
}

export const useFestivalMapsStore = defineStore('festivalMaps', () => {
  const ui = useUiStore()

  // -- Admin state ----------------------------------------------------------
  const maps = ref<FestivalMap[]>([])
  const selectedMap = ref<FestivalMap | null>(null)
  const mapForm = ref<FestivalMapForm | null>(null)
  /** Affiliates, for the per-stall operator select (null = Senpan Tea House). */
  const affiliates = ref<Affiliate[]>([])

  const mapsLoading = ref(false)
  const detailLoading = ref(false)
  const savingMap = ref(false)
  // Monotonic token guarding loadMapDetail against a last-write-wins race
  // (a slow earlier open overwriting a newer one). Only the latest applies.
  let detailSeq = 0

  // -- Public state ---------------------------------------------------------
  const publicMaps = ref<PublicFestivalMapSummary[]>([])
  const publicMap = ref<PublicFestivalMap | null>(null)
  const publicLoading = ref(false)

  // -- Computed -------------------------------------------------------------
  const publishedMaps = computed(() => maps.value.filter((m) => m.status === 'published'))
  const draftMaps = computed(() => maps.value.filter((m) => m.status === 'in_progress'))
  const closedMaps = computed(() => maps.value.filter((m) => m.status === 'closed'))

  // -- Admin: load ----------------------------------------------------------
  async function loadMaps(): Promise<void> {
    await withLoading(mapsLoading, async () => {
      maps.value = (await endpoints.festivalMaps.list()).maps
    })
  }

  async function loadMapDetail(id: number): Promise<void> {
    const reqId = ++detailSeq
    detailLoading.value = true
    try {
      const data = await endpoints.festivalMaps.detail(id)
      if (reqId !== detailSeq) return // a newer load superseded this one
      selectedMap.value = data.map
    } catch (e) {
      if (reqId === detailSeq) ui.notify((e as Error).message, 'error')
    } finally {
      if (reqId === detailSeq) detailLoading.value = false
    }
  }

  /** Admin: open a map's detail view. */
  function viewMap(m: FestivalMap): void {
    selectedMap.value = m
    void loadMapDetail(m.id)
  }

  /** Loads the affiliates list for the per-stall operator select. */
  async function loadFormSources(): Promise<void> {
    try {
      affiliates.value = (await endpoints.affiliates.list()).affiliates
    } catch {
      affiliates.value = []
    }
  }

  // -- Admin: form ----------------------------------------------------------
  function newMapForm(): void {
    // A brand-new map has nothing behind it, so nothing stays selected: cancelling
    // it must fall back to the list rather than to whichever map was last open.
    selectedMap.value = null
    mapForm.value = {
      id: 0,
      title: '',
      slug: '',
      description: '',
      map_image: '',
      times: [blankEventTime()],
      stalls: [],
    }
  }

  /**
   * True when `m` carries its pitches, i.e. it came from a detail fetch rather
   * than the list.
   *
   * The list omits `stalls` (`omitempty` server-side) and a save is a FULL
   * REPLACE, so seeding the form from a list row and saving it would delete every
   * pitch on the map along with the rally stamps and raffles pinned to them. The
   * server now leaves a collection alone when the request didn't carry it, but the
   * form must not offer an edit it cannot honor.
   */
  function hasMapDetail(m: FestivalMap | null | undefined): boolean {
    return !!m && Array.isArray(m.stalls)
  }

  function editMapForm(m: FestivalMap): boolean {
    if (!hasMapDetail(m)) {
      ui.notify('This map is still loading. Try again in a moment.', 'error')
      return false
    }
    mapForm.value = {
      id: m.id,
      title: m.title,
      slug: m.slug,
      description: m.description,
      map_image: m.map_image,
      times: timesToForm(m.times),
      stalls: (m.stalls || []).map((s) => ({
        id: s.id,
        shape: s.shape === 'circle' ? 'circle' : 'rect',
        color: s.color,
        selection_color: s.selection_color,
        text_color: s.text_color,
        placement: { ...s.placement },
        occupants: s.occupants.map((o) => ({
          id: o.id,
          affiliate_id: o.affiliate_id ?? null,
          title: o.title,
          description: o.description,
          event_carrd: o.event_carrd,
          stall_type: o.stall_type as StallType,
          type_label: o.type_label,
          times: timesToForm(o.times),
          _uid: nextUid(),
        })),
        _uid: nextUid(),
      })),
    }
    return true
  }

  /**
   * Seeds a brand-new map form from an existing one - the same venue laid out
   * again for next year's festival, with every stall already positioned.
   *
   * What it deliberately drops is what belonged to the run that already
   * happened: every id (so saving creates rather than overwrites) and every
   * datetime, on the map and on each stall alike. A stale window would quietly
   * mark the new festival as long over.
   */
  function copyMapForm(m: FestivalMap): boolean {
    if (!editMapForm(m)) return false
    const f = mapForm.value
    if (!f) return false
    f.id = 0
    f.title = `${m.title} (Copy)`
    // A shortcode names exactly one map, so a copy can't keep the original's -
    // saving would be refused, and silently inventing one would take over a URL
    // the original is already linked by.
    f.slug = ''
    f.times = [blankEventTime()]
    for (const stall of f.stalls) {
      stall.id = 0
      for (const occupant of stall.occupants) {
        occupant.id = 0
        occupant.times = []
      }
    }
    return true
  }

  function cancelMapForm(): void {
    mapForm.value = null
  }

  function addStall(type: StallType): FestivalStallForm | null {
    const f = mapForm.value
    if (!f) return null
    const stall = blankStall(type)
    f.stalls.push(stall)
    return stall
  }

  function removeStall(index: number): void {
    mapForm.value?.stalls.splice(index, 1)
  }

  /**
   * Adds another occupant to a pitch - the second business that stands there on a
   * different day. It starts as the same kind of thing the pitch already hosts,
   * since a booth that changes hands usually keeps doing roughly the same job.
   */
  function addOccupant(stallIndex: number): FestivalStallOccupantForm | null {
    const stall = mapForm.value?.stalls[stallIndex]
    if (!stall) return null
    const occupant = blankOccupant(stall.occupants[0]?.stall_type ?? 'other')
    stall.occupants.push(occupant)
    return occupant
  }

  /**
   * Removes one occupant from a pitch. The last one is kept: a pitch with nobody
   * in it is a coloured box the server drops on save, so the form never lets you
   * arrive there by accident.
   */
  function removeOccupant(stallIndex: number, occupantIndex: number): void {
    const stall = mapForm.value?.stalls[stallIndex]
    if (!stall || stall.occupants.length <= 1) return
    stall.occupants.splice(occupantIndex, 1)
  }

  /** Saves the map form. Returns true on success (caller navigates back). */
  async function saveMap(): Promise<boolean> {
    const f = mapForm.value
    if (!f) return false
    if (!f.title.trim()) {
      ui.notify('Title is required', 'error')
      return false
    }
    // Every stall names who runs it. The map LABELS a stall by its affiliate, so
    // one saved without leaves a blank shape on the plan - and the venue is an
    // affiliate in its own right now, so there is no default to fall back on.
    const missing = f.stalls.findIndex((stall) =>
      stall.occupants.some((o) => o.affiliate_id === null),
    )
    if (missing >= 0) {
      ui.notify(`Stall ${missing + 1} needs an affiliate`, 'error')
      return false
    }
    savingMap.value = true
    try {
      // Spread the form rather than listing fields. The list here was an
      // allow-list that had to be extended every time the editor gained a field,
      // and three of them - the event Carrd link, the selection colour and the
      // label colour - were silently dropped by it: set in the editor, saved
      // without complaint, gone on reload. Only the client-only `_uid` and the
      // datetime shapes need handling; anything else the form holds goes as-is,
      // and the server ignores what it does not know.
      const payload = {
        ...f,
        times: timesToPayload(f.times),
        stalls: f.stalls.map(({ _uid, occupants, ...stall }) => ({
          ...stall,
          occupants: occupants.map(({ _uid: _occupantUid, times, ...occupant }) => ({
            ...occupant,
            times: timesToPayload(times),
          })),
        })),
      }
      let savedId = f.id
      if (f.id) {
        await endpoints.festivalMaps.update(payload)
        ui.notify('Festival map updated', 'success')
      } else {
        savedId = (await endpoints.festivalMaps.create(payload)).map.id
        ui.notify('Festival map created', 'success')
      }
      mapForm.value = null
      await loadMaps()
      // Leave the saved map SELECTED, so the caller can show it rather than
      // dropping the admin back to the list. Saving is not "done with this map" -
      // publishing it, or checking how it reads, is the usual next step.
      await loadMapDetail(savedId)
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      savingMap.value = false
    }
  }

  async function deleteMap(id: number): Promise<void> {
    if (
      !(await ui.confirm(
        'Delete this festival map and all its stalls? A stamp rally linked to it is kept, but its stamps lose their stall.',
        { title: 'Delete festival map', confirmText: 'Delete' },
      ))
    )
      return
    try {
      await endpoints.festivalMaps.delete(id)
      maps.value = maps.value.filter((m) => m.id !== id)
      if (selectedMap.value?.id === id) selectedMap.value = null
      ui.notify('Festival map deleted', 'info')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Sets a map's publish status (in progress / published / closed). */
  async function setStatus(id: number, status: FestivalMapStatus): Promise<void> {
    try {
      await endpoints.festivalMaps.setStatus(id, status)
      if (selectedMap.value?.id === id) selectedMap.value.status = status
      const inList = maps.value.find((m) => m.id === id)
      if (inList) inList.status = status
      const said: Record<FestivalMapStatus, string> = {
        in_progress: 'Festival map moved back to in progress',
        published: 'Festival map published',
        closed: 'Festival map closed',
      }
      ui.notify(said[status], 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  // -- Public ---------------------------------------------------------------
  async function loadPublicMaps(): Promise<void> {
    publicLoading.value = true
    try {
      publicMaps.value = (await endpoints.festivalMaps.publicList()).maps
    } catch {
      publicMaps.value = []
    } finally {
      publicLoading.value = false
    }
  }

  /**
   * Loads one published map by its shortcode OR its numeric id - the server
   * resolves either, so a link posted before a shortcode existed still works.
   * Returns false when the map isn't public (or is gone).
   */
  async function loadPublicMap(idOrSlug: string): Promise<boolean> {
    publicMap.value = null
    publicLoading.value = true
    try {
      publicMap.value = await endpoints.festivalMaps.publicDetail(idOrSlug)
      return true
    } catch {
      return false
    } finally {
      publicLoading.value = false
    }
  }

  /**
   * The path segment a map is linked by: its shortcode when it has one, else its
   * id. One place decides it, so the admin's "copy public link", the public list
   * and the map page can't disagree about a map's URL.
   */
  function mapPath(m: { id: number; slug: string }): string {
    return m.slug || String(m.id)
  }

  return {
    // admin state
    maps,
    selectedMap,
    mapForm,
    affiliates,
    mapsLoading,
    detailLoading,
    savingMap,
    // public state
    publicMaps,
    publicMap,
    publicLoading,
    // computed
    publishedMaps,
    draftMaps,
    closedMaps,
    // admin actions
    loadMaps,
    loadMapDetail,
    viewMap,
    loadFormSources,
    newMapForm,
    hasMapDetail,
    editMapForm,
    copyMapForm,
    cancelMapForm,
    addStall,
    removeStall,
    addOccupant,
    removeOccupant,
    saveMap,
    deleteMap,
    setStatus,
    // public actions
    loadPublicMaps,
    loadPublicMap,
    mapPath,
  }
})
