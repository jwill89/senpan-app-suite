/**
 * Shared helpers for the Festival Map canvas (admin editor + public view).
 *
 * A stall is drawn as a shape filling its {@link Placement} - x/y/width/height as
 * percentages of the base map image's box, rotation in degrees - with its title
 * rendered inside it. The base image is the bare floor plan (walls, rest areas,
 * the stage); every stall shape and label on top of it comes from these helpers,
 * so renaming or moving a stall never means re-exporting the artwork.
 */
import type { CSSProperties } from 'vue'
import type {
  EventTime,
  Placement,
  PublicFestivalStall,
  StallShape,
  StallType,
  StampType,
} from '@/types/api'
import { parseServerTimestamp } from '@/lib/datetime'

/** The stall types, in picker order, with the defaults each one seeds. */
export const STALL_TYPES: {
  value: StallType
  label: string
  /** Shape a new stall of this type starts as (still editable per stall). */
  shape: StallShape
  /** Color a stall of this type falls back to when it sets none of its own. */
  color: string
  icon: string
}[] = [
  { value: 'game', label: 'Game', shape: 'circle', color: '#a89298', icon: 'dice' },
  { value: 'food', label: 'Food', shape: 'rect', color: '#e0a480', icon: 'bowl-rice' },
  { value: 'both', label: 'Food & Game', shape: 'rect', color: '#c9a08c', icon: 'utensils' },
  { value: 'other', label: 'Other', shape: 'rect', color: '#b08a8a', icon: 'shop' },
]

/** The stall type's metadata, falling back to "Other" for anything unknown. */
export function stallTypeMeta(type: string): (typeof STALL_TYPES)[number] {
  return STALL_TYPES.find((t) => t.value === type) ?? STALL_TYPES[STALL_TYPES.length - 1]
}

/** Display label for a stall type ("Game", "Food", ...). */
export function stallTypeLabel(type: string): string {
  return stallTypeMeta(type).label
}

/**
 * The color a pitch is drawn in: its own when it set one, else the default of
 * whatever its LEADING occupant offers. Kept in one place so the editor preview,
 * the admin list and the public map can't render the same pitch three shades.
 *
 * A pitch that changes hands between days takes its color from the first occupant
 * rather than flickering between them - the shape is the fixed thing on the plan;
 * the names inside it are what change.
 */
export function stallColor(pitch: { color: string; occupants: { stall_type: string }[] }): string {
  return pitch.color || stallTypeMeta(pitch.occupants[0]?.stall_type ?? '').color
}

/**
 * Which stamp type a stall's rally stamp counts as. Only a pure game stall is a
 * game stamp; food, "both" and "other" seed a food stamp - the admin can still
 * change it by hand on the rally, so this is a starting point, not a rule.
 */
export function stampTypeForStall(type: string): StampType {
  return type === 'game' ? 'game' : 'food'
}

/**
 * The caption drawn under a stall's title - the second line the floor plan shows
 * ("GAME", "FOOD & GAME", "OMIKUJI").
 *
 * A named type captions itself. An "other" stall captions with its own wording
 * instead, since "Other" tells a visitor nothing; when that wording is blank the
 * caption is dropped rather than printing a placeholder. The label is kept
 * whatever the type, so flipping a stall to a named type and back doesn't lose
 * it, but only an "other" stall renders it.
 */
export function stallCaption(occupant: { stall_type: string; type_label: string }): string {
  if (occupant.stall_type === 'other') return occupant.type_label.trim()
  return stallTypeLabel(occupant.stall_type)
}

/** Whether a shape draws as a true circle, whose height follows its width. */
export function isCircle(shape: string): boolean {
  return shape === 'circle'
}

/**
 * Absolute-position style for a stall at the given placement (rotated about its
 * centre).
 *
 * A CIRCLE takes its height from its own rendered width (`aspect-ratio: 1`)
 * rather than from `placement.height`: the map box is almost never square, so a
 * 12%-wide, 12%-tall box on a 16:10 floor plan draws a squashed oval, not a
 * circle. The stored height still describes the stall's rectangle form, so
 * switching a stall's shape back and forth doesn't lose it.
 */
export function stallStyle(
  placement: Placement,
  shape: string,
  color: string,
  selectionColor = '',
  textColor = '',
  occupantCount = 1,
): CSSProperties {
  const round = isCircle(shape)
  return {
    left: `${placement.x}%`,
    top: `${placement.y}%`,
    width: `${placement.width}%`,
    ...(round ? { aspectRatio: '1' } : { height: `${placement.height}%` }),
    transform: `rotate(${placement.rotation}deg)`,
    transformOrigin: 'center center',
    borderRadius: round ? '50%' : 'var(--radius)',
    background: color,
    // The selection halo is drawn in CSS but coloured per pitch, so the colour is
    // handed over as a variable rather than as a second background. Falls back to
    // the app highlight when the admin has not picked one.
    '--stall-selection': selectionColor || 'var(--highlight)',
    // The label's colour, likewise per pitch. White by default: a label fixed to a
    // dark colour only reads on a pale fill, and a pitch tinted anything deep left
    // its own name barely legible.
    '--stall-text': textColor || '#ffffff',
    // How many tenants share this pitch. The label's size ceiling is written for
    // ONE - roughly a name and a caption - so a pitch with two had its second
    // tenant clipped by the label's own overflow. Dividing by the count gives each
    // of them about the room one would have had.
    '--stall-occupants': String(Math.max(1, occupantCount)),
  }
}

/**
 * A circle's on-screen height as a percentage of the map box - what its width
 * works out to once `aspect-ratio: 1` squares it against a box `boxWidth` by
 * `boxHeight` pixels. The editor needs it to keep a circle inside the plan, since
 * `placement.height` no longer describes where its bottom edge lands.
 */
export function circleHeightPct(widthPct: number, boxWidth: number, boxHeight: number): number {
  if (boxHeight <= 0) return widthPct
  return (((widthPct / 100) * boxWidth) / boxHeight) * 100
}

/** Display name for a stall's operator: its affiliate, or the venue itself. */
export function stallOperator(affiliateName: string): string {
  return affiliateName.trim() || 'Senpan Tea House'
}

/**
 * A festival datetime range written the way a person reads one:
 *
 *   Sun, Aug 30, 2026, 8:00 PM - 1:00 AM EST
 *
 * FESTIVAL ONLY, which is why it lives here rather than in lib/datetime: the rest
 * of the app keeps its own formats, and this is not a house style being rolled out.
 *
 * The previous line came from `toLocaleString()`: "8/30/2026, 8:00:00 PM to
 * 8/31/2026, 1:00:00 AM" - two full timestamps, seconds nobody set, and the date
 * repeated for what a reader thinks of as one evening.
 *
 * The end is ALWAYS a time alone, never a second date. A festival evening that
 * runs past midnight is one sitting to the people at it, and repeating the date on
 * the far side of that reads as two separate things. The date stated once at the
 * front is the day to turn up.
 *
 * The zone is the viewer's own, named once at the end: these are wall-clock times
 * for deciding when to arrive.
 */

const eventDateParts: Intl.DateTimeFormatOptions = {
  weekday: 'short',
  month: 'short',
  day: 'numeric',
  year: 'numeric',
}
const eventTimeParts: Intl.DateTimeFormatOptions = { hour: 'numeric', minute: '2-digit' }

/** The viewer's zone abbreviation ("EST"), or '' when the runtime won't name it. */
function zoneAbbreviation(at: Date): string {
  const named = new Intl.DateTimeFormat(undefined, { ...eventTimeParts, timeZoneName: 'short' })
    .formatToParts(at)
    .find((part) => part.type === 'timeZoneName')
  return named?.value ?? ''
}

export function formatEventRange(start: string | null | undefined, end?: string | null): string {
  const startMs = parseServerTimestamp(start)
  if (!Number.isFinite(startMs)) return ''
  const from = new Date(startMs)

  const date = from.toLocaleDateString(undefined, eventDateParts)
  const fromTime = from.toLocaleTimeString(undefined, eventTimeParts)
  const zone = zoneAbbreviation(from)
  const suffix = zone ? ` ${zone}` : ''

  const endMs = parseServerTimestamp(end)
  if (!Number.isFinite(endMs)) return `${date}, ${fromTime}${suffix}`

  const toTime = new Date(endMs).toLocaleTimeString(undefined, eventTimeParts)
  return `${date}, ${fromTime} - ${toTime}${suffix}`
}

/**
 * One labelled festival range as a single line. An unlabeled one drops the prefix
 * rather than showing a stray dash.
 */
export function formatEventTime(time: EventTime): string {
  const span = formatEventRange(time.start, time.end)
  return time.label.trim() ? `${time.label.trim()} - ${span}` : span
}

/** Whether `now` falls inside a range, treating a missing bound as unbounded. */
export function isWithin(time: EventTime, now: number): boolean {
  const start = parseServerTimestamp(time.start)
  if (Number.isFinite(start) && now < start) return false
  const end = parseServerTimestamp(time.end)
  if (Number.isFinite(end) && now > end) return false
  return true
}

/**
 * Whether two datetime ranges overlap at all, treating a missing bound as
 * unbounded on that side. Used to decide which of a pitch's occupants is standing
 * there on a given festival day.
 */
export function rangesOverlap(a: EventTime, b: EventTime): boolean {
  const aStart = parseServerTimestamp(a.start)
  const aEnd = parseServerTimestamp(a.end)
  const bStart = parseServerTimestamp(b.start)
  const bEnd = parseServerTimestamp(b.end)
  if (Number.isFinite(aStart) && Number.isFinite(bEnd) && aStart > bEnd) return false
  if (Number.isFinite(bStart) && Number.isFinite(aEnd) && bStart > aEnd) return false
  return true
}

/**
 * The occupants standing in a pitch on the given festival day, or every one of
 * them when no day is picked.
 *
 * An occupant that states no times of its own runs the WHOLE festival, so it
 * matches every day - that is the common case, and it would be wrong to hide a
 * permanent stall just because it never spelled out its hours.
 */
export function occupantsOnDay<T extends { times: EventTime[] }>(
  occupants: T[],
  day: EventTime | null,
  reference: EventTime[] = [],
): T[] {
  if (!day) return occupants
  // Assigned by GREATEST overlap against the whole day list, the same way the day
  // label is decided - see occupantDays. Matching on any overlap at all put a Day 1
  // vendor under the Day 2 filter, because real festival days are broad spans that
  // run minutes into one another. With no day list to compare against there is
  // nothing to be principal about, so fall back to plain overlap.
  const days = reference.length > 1 ? reference : [day]
  return occupants.filter((o) =>
    occupantDays(o, days).some((d) => d === day || rangesOverlap(d, day)),
  )
}

/**
 * Whether `now` falls inside any of the ranges. An empty list is unbounded -
 * the same reading an empty availability window gets everywhere else in the app:
 * nothing was stated, so nothing is excluded. Mirrors the server's
 * withinAnyEventTime, which is what actually decides `is_active` / `is_open`;
 * this is for previewing an unsaved form.
 */
export function isWithinAny(times: EventTime[], now: number): boolean {
  return times.length === 0 || times.some((t) => isWithin(t, now))
}

/** A width/height pair in CSS pixels. */
export interface Size {
  width: number
  height: number
}

/** A pan offset in CSS pixels: the canvas's top-left within the viewport. */
export interface Pan {
  x: number
  y: number
}

/**
 * The zoom at which the whole plan fits inside the viewport.
 *
 * The viewport is a fixed shape but the canvas takes its height from the plan
 * image, so the two only match when the image happens to share that aspect. This
 * used to be assumed rather than computed, which is why a taller plan opened
 * cropped and "Fit" only restored the same crop.
 *
 * Returns 1 for a canvas that has not been measured yet, so a caller has a sane
 * value before the image has loaded.
 */
export function fitZoom(canvas: Size, viewport: Size): number {
  if (canvas.width <= 0 || canvas.height <= 0) return 1
  if (viewport.width <= 0 || viewport.height <= 0) return 1
  return Math.min(viewport.width / canvas.width, viewport.height / canvas.height)
}

/**
 * Keeps the scaled canvas covering the viewport, so panning can never leave the
 * visitor looking at empty background.
 *
 * An axis where the scaled canvas is SMALLER than the viewport is centred rather
 * than pinned to 0 - otherwise a plan narrower than its viewport hugs the left edge
 * with dead space beside it. Where it is larger, the offset is bounded so the
 * leading edge cannot come past 0 and the trailing edge cannot come inside the
 * viewport.
 */
export function clampPan(pan: Pan, canvas: Size, viewport: Size, zoom: number): Pan {
  const scaledWidth = canvas.width * zoom
  const scaledHeight = canvas.height * zoom
  if (scaledWidth <= 0 || scaledHeight <= 0) return pan

  const bound = (offset: number, slack: number) =>
    slack >= 0 ? slack / 2 : Math.min(0, Math.max(slack, offset))
  return {
    x: bound(pan.x, viewport.width - scaledWidth),
    y: bound(pan.y, viewport.height - scaledHeight),
  }
}

/**
 * Splits a trailing parenthetical off an occupant title.
 *
 * Admins write titles like "Flora Teahouse (Day 1)" - the form has always
 * suggested exactly that - so the day is part of the typed text rather than a
 * field. The map labels a pitch by its AFFILIATE, and that parenthetical belongs
 * on its own line underneath; this is only about WHERE it renders, so a title
 * without one yields no note and nothing is invented for it.
 *
 * Only a parenthetical that CLOSES the string counts, and only when something
 * precedes it: "Flora (Day 1) Annex" is one name, not a name and a note, and
 * "(Day 1)" alone is the whole title.
 */
export function splitTitleNote(title: string): { title: string; note: string } {
  const trimmed = title.trim()
  const match = /^(.*\S)\s*\(([^()]*)\)$/.exec(trimmed)
  if (!match) return { title: trimmed, note: '' }
  const note = match[2].trim()
  return note ? { title: match[1].trim(), note } : { title: trimmed, note: '' }
}

/**
 * What the map draws as a pitch occupant's label: the affiliate, always.
 *
 * The affiliate is who a visitor is looking for, and a stall title is optional -
 * so labelling by title left untitled pitches blank. An occupant with no affiliate
 * is the venue's own booth.
 */
export function occupantMapLabel(occupant: {
  title?: string
  affiliate?: { name?: string } | null
  affiliate_name?: string
}): string {
  const affiliate = (occupant.affiliate?.name ?? occupant.affiliate_name ?? '').trim()
  if (affiliate) return affiliate
  // No affiliate: a record from before one was required. Its own title is the only
  // thing that names it - and naming it after the venue instead was actively wrong,
  // since a pitch with no affiliate is not necessarily the venue's own. One such
  // pitch is titled "Atelier YAO" and was being drawn as "Senpan Tea House".
  return (occupant.title ?? '').trim()
}

/**
 * Bounds on the shape the map frame may take.
 *
 * These are a guard against a plan nobody would draw, NOT a house shape. Anything
 * the frame refuses to become is a shape the plan then has to be letterboxed
 * inside, and those empty bands are exactly the border a fitted map should not
 * have - so the bounds sit out where no real floor plan lives. Every ordinary one,
 * from a tall 9:16 poster to a 4:1 panorama, is taken at its own aspect and fills
 * its frame.
 *
 * What is still caught is the ribbon: a 1:20 plan capped to 80vh would be a column
 * around 40px wide, which is not a map anyone can read however faithfully it is
 * proportioned. Those are letterboxed inside the nearest allowed shape instead,
 * which the fit and the centring already handle.
 */
export const MIN_VIEWPORT_ASPECT = 0.4 // 2:5, a tall poster
export const MAX_VIEWPORT_ASPECT = 5 // 5:1, a long banner

/**
 * The shape to give the map frame for a plan of the given aspect (width / height).
 *
 * The frame used to be a fixed 16/10 whatever the plan was, so anything else had to
 * be zoomed out to fit and sat letterboxed between empty margins. Taking the shape
 * from the plan means it fills the frame at the size it was drawn. A plan outside
 * the bounds is letterboxed inside the nearest allowed shape instead, which the fit
 * and the centring already handle.
 *
 * Returns 0 for a plan whose size is not known yet, meaning "leave the CSS default".
 */
export function viewportAspect(imageAspect: number): number {
  if (!(imageAspect > 0)) return 0
  return Math.min(MAX_VIEWPORT_ASPECT, Math.max(MIN_VIEWPORT_ASPECT, imageAspect))
}

/** How long two ranges overlap, in ms. 0 when they do not. */
function overlapMs(a: EventTime, b: EventTime): number {
  const bound = (value: number, fallback: number) => (Number.isFinite(value) ? value : fallback)
  const start = Math.max(
    bound(parseServerTimestamp(a.start), -Infinity),
    bound(parseServerTimestamp(b.start), -Infinity),
  )
  const end = Math.min(
    bound(parseServerTimestamp(a.end), Infinity),
    bound(parseServerTimestamp(b.end), Infinity),
  )
  if (!(end > start)) return 0
  return Number.isFinite(end - start) ? end - start : Number.MAX_SAFE_INTEGER
}

/**
 * The festival days an occupant actually belongs to.
 *
 * Each of the occupant's ranges is assigned to the ONE day it overlaps most,
 * rather than to every day it touches at all. Real festival days are entered as
 * broad spans that run into one another - a "Day 1" of 7:35pm to 7:35pm the next
 * evening ends after "Day 2" has begun - so a vendor booked for Day 1 clips the
 * start of Day 2 by minutes. Counting that as "present on Day 2" put the vendor on
 * both days: it showed under the Day 2 filter, and named no day at all, because it
 * appeared to be there throughout.
 *
 * An occupant keeping no hours of its own follows the festival, so it belongs to
 * every day.
 */
export function occupantDays(
  occupant: { times: EventTime[] },
  reference: EventTime[],
): EventTime[] {
  if (occupant.times.length === 0) return reference
  const chosen = new Set<EventTime>()
  for (const time of occupant.times) {
    let best: EventTime | null = null
    let bestOverlap = 0
    for (const day of reference) {
      const overlap = overlapMs(time, day)
      if (overlap > bestOverlap) {
        bestOverlap = overlap
        best = day
      }
    }
    if (best) chosen.add(best)
  }
  return reference.filter((day) => chosen.has(day))
}

/**
 * Whether an occupant is tied to particular days of the festival, rather than
 * standing there for the whole of it.
 *
 * This is what decides whether a pitch's label names a day. Naming one on an
 * occupant who is present every day tells a visitor nothing and is actively
 * misleading - "(Day 1)" on a stall that is also there on Day 2 reads as "come
 * back tomorrow and it's gone".
 *
 * Judged from the occupant's own hours against the festival's days, not from the
 * label text: an admin writes the day into the title by hand, and hand-written
 * text can disagree with the schedule it is describing. Three cases return false:
 * an occupant that keeps no hours of its own (it follows the festival), a festival
 * with no days or only one (naming the only day says nothing), and an occupant
 * whose hours reach every day there is.
 */
export function isDayRestricted(
  occupant: { times: EventTime[] },
  festivalDays: EventTime[],
): boolean {
  if (occupant.times.length === 0) return false
  if (festivalDays.length <= 1) return false
  return occupantDays(occupant, festivalDays).length < festivalDays.length
}

/**
 * The days a pitch's occupants should be judged against.
 *
 * Normally the festival's own days. But a festival need not declare any - plenty
 * are set up with the hours only on the stalls - and then there was nothing to
 * compare an occupant to, so no occupant ever counted as day-restricted and no
 * pitch ever named a day. In that case the pitch's own occupants describe the days
 * between them: a booth held by one vendor on Saturday and another on Sunday
 * defines a two-day reference without the festival saying a word.
 */
export function dayReference(
  festivalDays: EventTime[],
  occupants: { times: EventTime[] }[],
): EventTime[] {
  if (festivalDays.length > 1) return festivalDays
  const seen = new Set<string>()
  const fromOccupants: EventTime[] = []
  for (const time of occupants.flatMap((o) => o.times)) {
    const key = `${time.start}|${time.end}`
    if (seen.has(key)) continue
    seen.add(key)
    fromOccupants.push(time)
  }
  return fromOccupants.length > festivalDays.length ? fromOccupants : festivalDays
}

/**
 * The day to name under a pitch's label, or '' for none.
 *
 * Shown only for an occupant who is NOT there every day - naming one on a vendor
 * present throughout reads as "gone tomorrow". The wording is the admin's own
 * where they wrote it into the title ("Tea Bar (Day 1)"), and otherwise comes from
 * the occupant's own hours, whose labels are what the admin named those days. That
 * fallback is what makes a multi-vendor pitch work: the second vendor rarely has
 * the day typed into its title, yet is exactly the case a visitor needs it for.
 */
export function occupantDayNote(
  occupant: { title: string; times: EventTime[] },
  reference: EventTime[],
): string {
  if (!isDayRestricted(occupant, reference)) return ''

  // The admin's own words first, where they wrote them into the title.
  const typed = splitTitleNote(occupant.title).note
  if (typed) return typed

  // Then the names of the REFERENCE days this occupant actually overlaps. This is
  // the case that matters in practice: a festival names its days ("Day 1", "Day 2")
  // and its vendors just carry hours, so the vendor's own ranges are often
  // unlabelled - and reading the name off the vendor found nothing to say.
  const overlapped = occupantDays(occupant, reference)
    .map((day) => day.label.trim())
    .filter(Boolean)
  if (overlapped.length) return overlapped.join(', ')

  // Failing both, whatever the occupant named its own hours.
  return occupant.times
    .map((t) => t.label.trim())
    .filter(Boolean)
    .join(', ')
}

/**
 * How an occupant is named in a LIST - an admin's stall table, or a picker that
 * attaches a raffle or a stamp to one: "Affiliate - Title", or just the affiliate
 * where there is no title.
 *
 * Distinct from occupantMapLabel, which is the affiliate alone because that is what
 * the map draws. A list has room for both, and needs it: two pitches held by the
 * same affiliate are told apart only by their titles.
 *
 * Neither ever falls back to "Untitled stall". A title is optional by design, and
 * naming an untitled pitch after the fact that it has no name tells a reader
 * nothing when the affiliate running it is right there.
 */
export function occupantListLabel(occupant: {
  title?: string
  affiliate?: { name?: string } | null
  affiliate_name?: string
}): string {
  const affiliate = (occupant.affiliate?.name ?? occupant.affiliate_name ?? '').trim()
  const title = (occupant.title ?? '').trim()
  // With no affiliate the title IS the name, so it is not repeated after itself.
  if (!affiliate) return title
  // Nor when the two say the same thing: a stall titled after the business running
  // it is common, and "Lunaria - Lunaria" reads as a mistake.
  if (!title || title.toLowerCase() === affiliate.toLowerCase()) return affiliate
  return `${affiliate} - ${title}`
}

/**
 * An admin map's stalls in the shape the public canvas draws.
 *
 * The admin detail screen used to re-implement the plan in its own markup, which
 * is how it drifted: it still labelled stalls by title long after the map itself
 * had moved to labelling by affiliate, so every untitled stall showed up blank.
 * Rendering the real component against adapted data means there is one drawing of
 * a festival map, not two that have to be kept in step by hand.
 *
 * The fields the canvas does not read - a stall's rally badge, its raffle, whether
 * it is open right now - are left off rather than invented: this is a preview of
 * the PLAN, and a preview that fabricated live state would be lying about it.
 */
export function toPublicStalls(
  stalls: {
    id: number
    shape: string
    color: string
    text_color?: string
    selection_color?: string
    placement: Placement
    occupants: {
      id: number
      title: string
      description: string
      event_carrd?: string
      stall_type: string
      type_label: string
      times: EventTime[]
      affiliate_name?: string
    }[]
  }[],
): PublicFestivalStall[] {
  return stalls.map((stall) => ({
    id: stall.id,
    shape: stall.shape,
    color: stall.color,
    text_color: stall.text_color ?? '',
    selection_color: stall.selection_color ?? '',
    ...stall.placement,
    placement: stall.placement,
    occupants: stall.occupants.map((o) => ({
      id: o.id,
      title: o.title,
      description: o.description,
      event_carrd: o.event_carrd ?? '',
      stall_type: o.stall_type,
      type_label: o.type_label,
      times: o.times,
      is_open: false,
      in_stamp_rally: false,
      affiliate: o.affiliate_name ? { name: o.affiliate_name } : undefined,
    })) as PublicFestivalStall['occupants'],
  }))
}

/**
 * The path one map is EMBEDDED at: the same map drawn as nothing but the widget,
 * for an <iframe> on someone else's page.
 *
 * A path of its own rather than a flag on the public one, because the two really
 * are different pages. The public URL is somewhere a visitor LANDS, and it keeps
 * the site's header, description and footer around the plan; the embedded one is
 * dropped into a page that already has its own of each, and must bring none of
 * ours with it.
 */
export function mapEmbedPath(idOrSlug: string): string {
  return `/embed/festival-maps/${idOrSlug}`
}

/**
 * The height the pasted snippet starts at, in pixels. Tall enough that a plan is
 * worth looking at without scrolling, short enough to sit in a page rather than
 * take it over - and it is plain text in the snippet, so an admin can change it.
 */
export const MAP_EMBED_HEIGHT = 640

/** Escapes a value for use inside a double-quoted HTML attribute. */
function escapeAttribute(value: string): string {
  return value
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

/**
 * The <iframe> tag to paste into another site - a Carrd embed block, or anything
 * else that takes raw HTML - to put the interactive map on it.
 *
 * Both attributes are ESCAPED rather than trusted. A festival called
 * `Yoey's "Big" Night` would otherwise close the title attribute early and paste
 * a broken tag, and an admin copying this has no way to see that until it is
 * already live on someone else's page.
 *
 * `width="100%"` with a stated height is the shape embed hosts expect: the host
 * owns the column and we cannot know how wide it is, while a height has to be
 * declared because an iframe has no content to size itself from. The map fits
 * itself to whatever box it ends up in.
 */
export function mapEmbedSnippet(url: string, title: string): string {
  const name = title.trim() ? `${title.trim()} - festival map` : 'Festival map'
  return (
    `<iframe src="${escapeAttribute(url)}" title="${escapeAttribute(name)}"` +
    ` width="100%" height="${MAP_EMBED_HEIGHT}" loading="lazy"` +
    ` style="border:0;display:block;max-width:100%"></iframe>`
  )
}
