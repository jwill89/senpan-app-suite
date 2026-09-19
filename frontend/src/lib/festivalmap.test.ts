import { describe, it, expect } from 'vitest'
import {
  clampPan,
  occupantListLabel,
  occupantDays,
  occupantsOnDay as occupantsOnDayFn,
  dayReference,
  isDayRestricted,
  occupantDayNote,
  formatEventRange,
  fitZoom,
  viewportAspect,
  occupantMapLabel,
  splitTitleNote,
  STALL_TYPES,
  circleHeightPct,
  occupantsOnDay,
  rangesOverlap,
  formatEventTime,
  isCircle,
  isWithin,
  MAP_EMBED_HEIGHT,
  mapEmbedPath,
  mapEmbedSnippet,
  isWithinAny,
  stallColor,
  stallCaption,
  stallOperator,
  stallStyle,
  stallTypeLabel,
  stallTypeMeta,
  stampTypeForStall,
} from './festivalmap'
import type { EventTime } from '@/types/api'

const time = (over: Partial<EventTime> = {}): EventTime => ({
  label: '',
  start: '',
  end: '',
  ...over,
})

describe('stall type metadata', () => {
  it('gives a game stall a circle and a food stall a rectangle', () => {
    expect(stallTypeMeta('game').shape).toBe('circle')
    expect(stallTypeMeta('food').shape).toBe('rect')
  })

  it('falls back to Other for an unknown type rather than throwing', () => {
    expect(stallTypeMeta('hovercraft').value).toBe('other')
    expect(stallTypeLabel('hovercraft')).toBe('Other')
  })

  it('gives every offered type a distinct default color', () => {
    const colors = new Set(STALL_TYPES.map((t) => t.color))
    expect(colors.size).toBe(STALL_TYPES.length)
  })
})

describe('stallCaption', () => {
  it('captions a named type with its own label', () => {
    expect(stallCaption({ stall_type: 'game', type_label: '' })).toBe('Game')
    expect(stallCaption({ stall_type: 'both', type_label: '' })).toBe('Food & Game')
  })

  it('captions an \'other\' stall with its own wording, since "Other" says nothing', () => {
    expect(stallCaption({ stall_type: 'other', type_label: 'Omikuji' })).toBe('Omikuji')
  })

  it("drops the caption entirely for an 'other' stall with no wording", () => {
    expect(stallCaption({ stall_type: 'other', type_label: '   ' })).toBe('')
  })

  it('ignores the wording on a named type, so flipping type back is predictable', () => {
    expect(stallCaption({ stall_type: 'food', type_label: 'Omikuji' })).toBe('Food')
  })
})

describe('stallColor', () => {
  it("uses the pitch's own color when it set one", () => {
    expect(stallColor({ color: '#123456', occupants: [{ stall_type: 'game' }] })).toBe('#123456')
  })

  it("falls back to the LEADING occupant's type default when the pitch sets none", () => {
    // A pitch that changes hands takes its color from the first occupant rather
    // than flickering between them - the shape is the fixed thing on the plan.
    const pitch = { color: '', occupants: [{ stall_type: 'food' }, { stall_type: 'game' }] }
    expect(stallColor(pitch)).toBe(stallTypeMeta('food').color)
  })

  it('falls back to the Other default for a pitch with nobody in it', () => {
    expect(stallColor({ color: '', occupants: [] })).toBe(stallTypeMeta('other').color)
  })
})

describe('rangesOverlap', () => {
  const day1 = time({ start: '2026-08-01T00:00:00Z', end: '2026-08-01T23:59:00Z' })
  const day2 = time({ start: '2026-08-02T00:00:00Z', end: '2026-08-02T23:59:00Z' })

  it('separates two festival days', () => {
    expect(rangesOverlap(day1, day1)).toBe(true)
    expect(rangesOverlap(day1, day2)).toBe(false)
  })

  it('treats a missing bound as unbounded on that side', () => {
    // "From Day 2 onward" overlaps Day 2 but not Day 1.
    const fromDay2 = time({ start: '2026-08-02T00:00:00Z' })
    expect(rangesOverlap(fromDay2, day2)).toBe(true)
    expect(rangesOverlap(fromDay2, day1)).toBe(false)
    // A range with no bounds at all overlaps everything.
    expect(rangesOverlap(time(), day1)).toBe(true)
  })
})

describe('occupantsOnDay', () => {
  const day1 = time({ start: '2026-08-01T00:00:00Z', end: '2026-08-01T23:59:00Z' })
  const day2 = time({ start: '2026-08-02T00:00:00Z', end: '2026-08-02T23:59:00Z' })
  const flora = { title: 'Flora Teahouse', times: [day1] }
  const below = { title: 'The Great Below', times: [day2] }
  const allWeek = { title: 'Senpan Tea House', times: [] as EventTime[] }

  it('returns everyone when no day is picked', () => {
    expect(occupantsOnDay([flora, below], null)).toHaveLength(2)
  })

  it('picks out the occupant standing there that day', () => {
    expect(occupantsOnDay([flora, below], day1)).toEqual([flora])
    expect(occupantsOnDay([flora, below], day2)).toEqual([below])
  })

  it('keeps an occupant that states no times - it runs the whole festival', () => {
    // Hiding a permanent stall just because it never spelled out its hours would
    // take it off the plan on every day.
    expect(occupantsOnDay([allWeek, flora], day2)).toEqual([allWeek])
  })
})

describe('stampTypeForStall', () => {
  it('counts only a pure game stall as a game stamp', () => {
    expect(stampTypeForStall('game')).toBe('game')
  })

  it('counts food, both and other as food stamps', () => {
    for (const type of ['food', 'both', 'other', '']) {
      expect(stampTypeForStall(type)).toBe('food')
    }
  })
})

describe('stallStyle', () => {
  it('positions and rotates the stall as a share of the map box', () => {
    const style = stallStyle({ x: 10, y: 20, width: 30, height: 40, rotation: 15 }, 'rect', '#abc')
    expect(style.left).toBe('10%')
    expect(style.top).toBe('20%')
    expect(style.width).toBe('30%')
    expect(style.height).toBe('40%')
    expect(style.transform).toBe('rotate(15deg)')
    expect(style.background).toBe('#abc')
  })

  it('rounds a circle stall fully and leaves a rectangle on the chrome radius', () => {
    const p = { x: 0, y: 0, width: 10, height: 10, rotation: 0 }
    expect(stallStyle(p, 'circle', '#abc').borderRadius).toBe('50%')
    expect(stallStyle(p, 'rect', '#abc').borderRadius).toBe('var(--radius)')
  })

  it('squares a circle against its own width instead of the map box', () => {
    // The map box is almost never square, so a percentage height would draw an
    // oval - the whole reason a circle sizes itself from its rendered width.
    const style = stallStyle({ x: 0, y: 0, width: 10, height: 40, rotation: 0 }, 'circle', '#abc')
    expect(style.aspectRatio).toBe('1')
    expect(style.height).toBeUndefined()
  })

  it('keeps the stored height for a rectangle, so switching shapes back restores it', () => {
    const p = { x: 0, y: 0, width: 10, height: 40, rotation: 0 }
    expect(stallStyle(p, 'rect', '#abc').height).toBe('40%')
    expect(stallStyle(p, 'rect', '#abc').aspectRatio).toBeUndefined()
  })
})

describe('isCircle', () => {
  it('is true only for the circle shape', () => {
    expect(isCircle('circle')).toBe(true)
    expect(isCircle('rect')).toBe(false)
    expect(isCircle('')).toBe(false)
  })
})

describe('circleHeightPct', () => {
  it('converts a width percentage into the height it renders as', () => {
    // 10% of a 1000px-wide box is 100px; on a 500px-tall box that is 20%.
    expect(circleHeightPct(10, 1000, 500)).toBeCloseTo(20)
    // A square box leaves the percentage unchanged.
    expect(circleHeightPct(10, 800, 800)).toBeCloseTo(10)
  })

  it('falls back to the width rather than dividing by a zero-height box', () => {
    expect(circleHeightPct(10, 1000, 0)).toBe(10)
  })
})

describe('stallOperator', () => {
  it('names the venue itself when the stall has no affiliate', () => {
    expect(stallOperator('')).toBe('Senpan Tea House')
    expect(stallOperator('   ')).toBe('Senpan Tea House')
  })

  it('names the affiliate otherwise', () => {
    expect(stallOperator('Flora Teahouse')).toBe('Flora Teahouse')
  })
})

describe('formatEventTime', () => {
  it('prefixes the label when there is one', () => {
    const line = formatEventTime(time({ label: 'Day 1', start: '2026-08-01T18:00:00Z' }))
    expect(line.startsWith('Day 1 - ')).toBe(true)
  })

  it('drops the prefix entirely when unlabeled, rather than leaving a stray dash', () => {
    // Checked on an OPEN-ENDED range: a range with an end now legitimately
    // contains " - " between its two times, so that is no longer the tell.
    const line = formatEventTime(time({ start: '2026-08-01T18:00:00Z' }))
    expect(line.startsWith('-')).toBe(false)
    expect(line).not.toContain(' - ')
  })

  it('reads as a start time alone when there is no end', () => {
    // The two ends are joined by a dash now, not the word "to" - see
    // formatEventRange. An open-ended range still shows one moment.
    const withEnd = formatEventTime(
      time({ start: '2026-08-01T18:00:00Z', end: '2026-08-01T22:00:00Z' }),
    )
    const openEnded = formatEventTime(time({ start: '2026-08-01T18:00:00Z' }))
    expect(withEnd).toMatch(/\d{1,2}:\d{2} (AM|PM) - \d{1,2}:\d{2} (AM|PM)/)
    expect(openEnded).not.toMatch(/ - \d{1,2}:\d{2}/)
  })
})

describe('isWithin / isWithinAny', () => {
  const noon = Date.parse('2026-08-01T12:00:00Z')

  it('treats a missing bound as unbounded on that side', () => {
    expect(isWithin(time({ start: '2026-08-01T09:00:00Z' }), noon)).toBe(true)
    expect(isWithin(time({ end: '2026-08-01T09:00:00Z' }), noon)).toBe(false)
  })

  it('excludes a range that has not started or has ended', () => {
    expect(isWithin(time({ start: '2026-08-01T13:00:00Z' }), noon)).toBe(false)
    expect(
      isWithin(time({ start: '2026-08-01T09:00:00Z', end: '2026-08-01T11:00:00Z' }), noon),
    ).toBe(false)
  })

  it('reads an empty list as unbounded - nothing stated excludes nothing', () => {
    expect(isWithinAny([], noon)).toBe(true)
  })

  it('matches when any one range covers now', () => {
    const times = [
      time({ label: 'Day 1', start: '2026-07-01T09:00:00Z', end: '2026-07-01T22:00:00Z' }),
      time({ label: 'Day 2', start: '2026-08-01T09:00:00Z', end: '2026-08-01T22:00:00Z' }),
    ]
    expect(isWithinAny(times, noon)).toBe(true)
    expect(isWithinAny([times[0]], noon)).toBe(false)
  })
})

/**
 * The public map opened cropped, could not be dragged, and "Fit" only restored the
 * same crop. All three came from one assumption: that the canvas exactly fills the
 * viewport. It does not - the viewport is a fixed shape while the canvas takes its
 * height from the plan image - so a plan taller than that shape overflowed with a
 * pan range of zero and a zoom floor of 1 that could not reach it.
 */
describe('festival map pan and zoom', () => {
  const viewport = { width: 1152, height: 720 }

  it('fits a plan taller than the viewport by zooming out below 1', () => {
    // 800x1400 rendered to the viewport width: 1150x2013. Height is the binding
    // axis, so the whole plan only fits at ~0.36.
    const zoom = fitZoom({ width: 1150, height: 2013 }, viewport)
    expect(zoom).toBeCloseTo(720 / 2013, 5)
    expect(zoom).toBeLessThan(1)
    // ...and at that zoom it really does fit, which is what "Fit" has to deliver.
    expect(1150 * zoom).toBeLessThanOrEqual(viewport.width + 0.01)
    expect(2013 * zoom).toBeLessThanOrEqual(viewport.height + 0.01)
  })

  it('fits a wide plan on the width instead', () => {
    expect(fitZoom({ width: 4000, height: 500 }, viewport)).toBeCloseTo(1152 / 4000, 5)
  })

  it('reports 1 for a canvas that has not been measured yet', () => {
    expect(fitZoom({ width: 0, height: 0 }, viewport)).toBe(1)
  })

  it('allows panning exactly as far as the overflow, and no further', () => {
    const canvas = { width: 1150, height: 2013 }
    // Dragged far past the bottom edge: pinned to the real overflow, not to 0.
    const overflowY = viewport.height - canvas.height
    expect(clampPan({ x: 0, y: -99999 }, canvas, viewport, 1).y).toBeCloseTo(overflowY, 5)
    // Dragged past the top edge: the leading edge cannot come inside the viewport.
    expect(clampPan({ x: 0, y: 500 }, canvas, viewport, 1).y).toBe(0)
    // In range: left where the visitor put it. This is the case that used to be
    // clamped to 0, which is why dragging appeared to do nothing.
    expect(clampPan({ x: 0, y: -300 }, canvas, viewport, 1).y).toBe(-300)
  })

  it('centres an axis where the plan is smaller than the viewport', () => {
    // A tall plan zoomed to fit is narrower than the viewport; hugging the left
    // edge would leave dead space beside it.
    const canvas = { width: 1150, height: 2013 }
    const zoom = fitZoom(canvas, viewport)
    const panned = clampPan({ x: 0, y: 0 }, canvas, viewport, zoom)
    expect(panned.x).toBeCloseTo((viewport.width - canvas.width * zoom) / 2, 4)
    expect(panned.y).toBeCloseTo(0, 4)
  })

  it('keeps a zoomed-in plan pannable on both axes', () => {
    const canvas = { width: 1150, height: 2013 }
    const panned = clampPan({ x: -200, y: -400 }, canvas, viewport, 2)
    expect(panned).toEqual({ x: -200, y: -400 })
  })
})

describe('map labelling', () => {
  /**
   * The map labels a pitch by its AFFILIATE. A stall title is optional, so
   * labelling by title left untitled pitches blank - and the affiliate is who a
   * visitor is actually looking for.
   */
  it('labels by affiliate, falling back to the occupant own title', () => {
    expect(occupantMapLabel({ affiliate: { name: 'Flora Teahouse' } })).toBe('Flora Teahouse')
    expect(occupantMapLabel({ affiliate_name: 'Lunaria' })).toBe('Lunaria')
    // No affiliate: a record from before one was required. Naming it after the
    // VENUE was wrong - a pitch without an affiliate is not necessarily the
    // venue's own, and one such pitch titled "Atelier YAO" was drawn as "Senpan
    // Tea House". Its own title is the only thing that names it.
    expect(occupantMapLabel({ affiliate: null, title: 'Atelier YAO' })).toBe('Atelier YAO')
    expect(occupantMapLabel({ affiliate: { name: '   ' }, title: 'Atelier YAO' })).toBe(
      'Atelier YAO',
    )
    expect(occupantMapLabel({ affiliate: null })).toBe('')
  })

  it('does not repeat an affiliate whose stall is titled after it', () => {
    // Common in real data - a stall titled after the business running it - and
    // "Lunaria - Lunaria" reads as a mistake.
    expect(occupantListLabel({ affiliate_name: 'Lunaria', title: 'Lunaria' })).toBe('Lunaria')
    expect(occupantListLabel({ affiliate_name: 'Lunaria', title: 'lunaria' })).toBe('Lunaria')
  })

  it('does not repeat a title after itself in a list label', () => {
    // With no affiliate the title IS the name.
    expect(occupantListLabel({ affiliate: null, title: 'Atelier YAO' })).toBe('Atelier YAO')
    expect(occupantListLabel({ affiliate_name: 'Lunaria', title: 'Tea Bar' })).toBe(
      'Lunaria - Tea Bar',
    )
    expect(occupantListLabel({ affiliate_name: 'Lunaria' })).toBe('Lunaria')
  })

  /**
   * Admins write the day into the title - the form has always suggested exactly
   * "Flora Teahouse (Day 1)". This only moves where that parenthetical renders; it
   * is never invented for a title that has none.
   */
  it('splits a trailing parenthetical off a title', () => {
    expect(splitTitleNote('Flora Teahouse (Day 1)')).toEqual({
      title: 'Flora Teahouse',
      note: 'Day 1',
    })
    expect(splitTitleNote('  Tea Bar   (Day 2)  ')).toEqual({ title: 'Tea Bar', note: 'Day 2' })
  })

  it('leaves a title without a trailing parenthetical alone', () => {
    expect(splitTitleNote('Flora Teahouse')).toEqual({ title: 'Flora Teahouse', note: '' })
    expect(splitTitleNote('')).toEqual({ title: '', note: '' })
    // Mid-string: one name, not a name and a note.
    expect(splitTitleNote('Flora (Day 1) Annex')).toEqual({
      title: 'Flora (Day 1) Annex',
      note: '',
    })
    // Nothing before it: the parenthetical IS the title.
    expect(splitTitleNote('(Day 1)')).toEqual({ title: '(Day 1)', note: '' })
    // Empty parens are not a note.
    expect(splitTitleNote('Flora ()')).toEqual({ title: 'Flora ()', note: '' })
  })
})

describe('map frame shape', () => {
  /**
   * The frame used to be a fixed 16/10 whatever the plan was, so any other shape
   * had to be zoomed out to fit and sat letterboxed between empty margins. Taking
   * the shape from the plan means it fills the frame at the size it was drawn.
   */
  it('takes an ordinary plan of any shape at its own aspect', () => {
    expect(viewportAspect(1.5)).toBe(1.5) // 3:2
    expect(viewportAspect(1.6)).toBe(1.6) // 16:10
    expect(viewportAspect(1)).toBe(1) // square
  })

  /**
   * The shapes a real plan actually comes in are taken as they are. Every shape
   * the frame REFUSES is one the plan is then letterboxed inside, and those empty
   * bands either side are precisely the border a fitted map should not have - so
   * the bounds have to sit out past anything a person would draw.
   */
  it('takes a tall poster and a long banner at their own shape', () => {
    expect(viewportAspect(0.5625)).toBe(0.5625) // 9:16, a phone-shaped plan
    expect(viewportAspect(4)).toBe(4) // 4:1, a row of stalls along a street
  })

  it('clamps only a ribbon, which is not a map at any proportion', () => {
    // 1:20 capped to 80vh is a column ~40px wide; 20:1 a slot a few pixels tall.
    expect(viewportAspect(0.05)).toBe(0.4)
    expect(viewportAspect(20)).toBe(5)
  })

  it('reports 0 before the plan size is known, meaning "leave the default"', () => {
    expect(viewportAspect(0)).toBe(0)
    expect(viewportAspect(Number.NaN)).toBe(0)
    expect(viewportAspect(-2)).toBe(0)
  })

  /**
   * The pay-off: at its own aspect the plan fits at zoom 1 and covers the frame,
   * rather than being shrunk to sit inside a shape it never had.
   */
  it('lets a plan fill its frame exactly', () => {
    const frame = { width: 864, height: 864 / viewportAspect(1.5) }
    expect(fitZoom({ width: 864, height: 576 }, frame)).toBeCloseTo(1, 5)
  })
})

/**
 * FESTIVAL ONLY - the rest of the app keeps its own formats.
 *
 * Asserted by SHAPE, not against a spelled-out string: the output is deliberately
 * rendered in the viewer's own zone, so a literal expectation would encode whichever
 * machine ran the suite and fail on the next one. (Setting process.env.TZ does not
 * help - the zone is resolved before a test can change it.)
 */
describe('festival datetime formatting', () => {
  it('writes an evening that crosses midnight as one date and two times', () => {
    // The shape asked for: "Sun, Aug 30, 2026, 8:00 PM - 1:00 AM EST".
    const line = formatEventRange('2026-08-30T20:00:00Z', '2026-08-31T01:00:00Z')
    expect(line).toMatch(
      /^\w{3}, \w{3} \d{1,2}, \d{4}, \d{1,2}:\d{2} (AM|PM) - \d{1,2}:\d{2} (AM|PM)/,
    )
    // One date only - a five-hour sitting is not two days to a reader.
    expect(line.match(/\d{4}/g)).toHaveLength(1)
  })

  it('never repeats the date on the end, however long the range', () => {
    // A festival evening running past midnight is ONE sitting to the people at it;
    // a second date on the far side reads as two separate things. The date stated
    // once at the front is the day to turn up.
    const overnight = formatEventRange('2026-08-30T20:00:00Z', '2026-08-31T01:00:00Z')
    expect(overnight.match(/\d{4}/g)).toHaveLength(1)
    const long = formatEventRange('2026-08-28T14:00:00Z', '2026-08-30T22:00:00Z')
    expect(long.match(/\d{4}/g)).toHaveLength(1)
    expect(long).toMatch(/\d{1,2}:\d{2} (AM|PM) - \d{1,2}:\d{2} (AM|PM)/)
  })

  it('writes a start with no end as a single moment', () => {
    const line = formatEventRange('2026-08-30T20:00:00Z')
    expect(line).not.toContain(' - ')
    expect(line).toMatch(/\d{1,2}:\d{2} (AM|PM)/)
  })

  it('has no seconds and no slash-dates anywhere', () => {
    // What toLocaleString() used to produce: "8/30/2026, 8:00:00 PM".
    const line = formatEventRange('2026-08-30T20:00:00Z', '2026-08-31T01:00:00Z')
    expect(line).not.toMatch(/\d+\/\d+\/\d+/)
    expect(line).not.toMatch(/:\d{2}:\d{2}/)
  })

  it('returns nothing for an unparseable start', () => {
    expect(formatEventRange('')).toBe('')
    expect(formatEventRange('not a date')).toBe('')
  })

  it('keeps a label prefix, and drops the dash when there is none', () => {
    const labelled = formatEventTime({ label: 'Day 1', start: '2026-08-30T20:00:00Z', end: '' })
    expect(labelled.startsWith('Day 1 - ')).toBe(true)
    const bare = formatEventTime({ label: '  ', start: '2026-08-30T20:00:00Z', end: '' })
    expect(bare.startsWith('-')).toBe(false)
  })
})

describe('isDayRestricted', () => {
  const day = (label: string, start: string, end: string) => ({ label, start, end })
  const days = [
    day('Day 1', '2026-08-30T18:00:00Z', '2026-08-31T02:00:00Z'),
    day('Day 2', '2026-08-31T18:00:00Z', '2026-09-01T02:00:00Z'),
  ]

  /**
   * Whether a pitch's label names a day. Naming one on an occupant who is there
   * every day is worse than saying nothing - "(Day 1)" on a stall that is also
   * open on Day 2 reads as "come back tomorrow and it's gone".
   */
  it('is true for an occupant present on only some days', () => {
    expect(isDayRestricted({ times: [days[0]] }, days)).toBe(true)
    expect(isDayRestricted({ times: [days[1]] }, days)).toBe(true)
  })

  it('is false for an occupant present on every day', () => {
    // The case that prompted this: one affiliate covering both days was still
    // labelled "(DAY 1)" because the text came from the title rather than the
    // schedule.
    expect(isDayRestricted({ times: days }, days)).toBe(false)
  })

  it('is false for an occupant that keeps no hours of its own', () => {
    // No times means "follows the festival", which is every day of it.
    expect(isDayRestricted({ times: [] }, days)).toBe(false)
  })

  it('is false when the festival has no more than one day', () => {
    // Naming the only day there is tells a visitor nothing.
    expect(isDayRestricted({ times: [days[0]] }, [days[0]])).toBe(false)
    expect(isDayRestricted({ times: [days[0]] }, [])).toBe(false)
  })

  it('is judged on overlap, not on exact equality', () => {
    // An occupant keeping shorter hours inside a festival day is still there that
    // day; only a day it does not reach at all makes it restricted.
    const shortShift = day('', '2026-08-30T19:00:00Z', '2026-08-30T21:00:00Z')
    expect(isDayRestricted({ times: [shortShift] }, days)).toBe(true)
    expect(isDayRestricted({ times: [shortShift, days[1]] }, days)).toBe(false)
  })
})

describe('the day named under a pitch label', () => {
  const d = (label: string, start: string, end: string) => ({ label, start, end })
  const day1 = d('Day 1', '2026-08-30T18:00:00Z', '2026-08-31T02:00:00Z')
  const day2 = d('Day 2', '2026-08-31T18:00:00Z', '2026-09-01T02:00:00Z')

  /**
   * The reported bug: a pitch shared by two vendors, one of them there for a
   * single day, named no day at all. Two things were in the way - the festival
   * declared no days of its own, so nothing counted as restricted; and the note's
   * wording came from the title, which a second vendor rarely has it typed into.
   */
  it('names the day for each vendor of a two-vendor pitch, with no festival days set', () => {
    const saturday = { title: 'Tea Bar', times: [day1] }
    const sunday = { title: 'Craft Stall', times: [day2] }
    const reference = dayReference([], [saturday, sunday])
    expect(reference).toHaveLength(2)
    // Wording falls back to the labels the admin gave those hours.
    expect(occupantDayNote(saturday, reference)).toBe('Day 1')
    expect(occupantDayNote(sunday, reference)).toBe('Day 2')
  })

  /**
   * The live-site case. A festival names its days ("Day 1", "Day 2") and its
   * vendors just carry hours - so the vendor's own ranges are unlabelled, and
   * reading the name off the vendor found nothing to say. The day a vendor
   * OVERLAPS is what names it.
   */
  it('names the day from the festival when the vendor labels no hours of its own', () => {
    const unlabelled = (start: string, end: string) => ({ label: '', start, end })
    const saturday = {
      title: 'Tea Bar',
      times: [unlabelled('2026-08-30T19:00:00Z', '2026-08-31T01:00:00Z')],
    }
    const sunday = {
      title: 'Craft Table',
      times: [unlabelled('2026-08-31T19:00:00Z', '2026-09-01T01:00:00Z')],
    }
    expect(occupantDayNote(saturday, [day1, day2])).toBe('Day 1')
    expect(occupantDayNote(sunday, [day1, day2])).toBe('Day 2')
  })

  it('prefers the wording the admin typed into the title', () => {
    const vendor = { title: 'Tea Bar (Sat only)', times: [day1] }
    expect(occupantDayNote(vendor, dayReference([], [vendor, { title: '', times: [day2] }]))).toBe(
      'Sat only',
    )
  })

  it('still names nothing for a vendor there every day', () => {
    // The case that started this: one vendor across both days, titled "(Day 1)".
    const allFestival = { title: 'Tea Bar (Day 1)', times: [day1, day2] }
    expect(occupantDayNote(allFestival, [day1, day2])).toBe('')
    // ...and one keeping no hours of its own follows the festival, so likewise.
    expect(occupantDayNote({ title: 'Tea Bar (Day 1)', times: [] }, [day1, day2])).toBe('')
  })

  it('prefers the festivals own days when it declares them', () => {
    const declared = [day1, day2]
    expect(dayReference(declared, [{ times: [day1] }])).toEqual(declared)
  })

  it('falls back only when the festival declares fewer days than the pitch shows', () => {
    // A one-day festival stays one day; nothing about a pitch makes it two.
    expect(dayReference([day1], [{ times: [day1] }])).toEqual([day1])
  })
})

/**
 * The exact values from the production festival, which is what finally explained
 * this. Each "day" is entered as a broad ~24h span, so consecutive days RUN INTO
 * one another: Day 1 ends 2026-09-01T19:35 while Day 2 has already begun at
 * 19:39... and Lunaria, booked for Day 1, ends at 19:48 - nine minutes inside Day
 * 2's window.
 *
 * Matching on any overlap at all therefore put Lunaria on both days: it appeared
 * under the Day 2 filter, and named no day at all, because it looked like it was
 * there throughout. Each range now belongs to the ONE day it overlaps most.
 */
describe('production festival day assignment', () => {
  const day1 = {
    label: 'Day 1',
    start: '2026-08-31T19:35:00.000Z',
    end: '2026-09-01T19:35:00.000Z',
  }
  const day2 = {
    label: 'Day 2',
    start: '2026-09-01T19:39:00.000Z',
    end: '2026-09-02T19:39:00.000Z',
  }
  const days = [day1, day2]

  const lunaria = {
    title: 'Lunaria',
    times: [{ label: 'Day 1', start: '2026-08-31T19:48:00.000Z', end: '2026-09-01T19:48:00.000Z' }],
  }
  const moonlight = {
    title: 'Moonlight Tides',
    times: [{ label: 'Day 2', start: '2026-09-01T19:48:00.000Z', end: '2026-09-02T19:48:00.000Z' }],
  }

  it('puts each vendor on the one day it principally occupies', () => {
    expect(occupantDays(lunaria, days).map((d) => d.label)).toEqual(['Day 1'])
    expect(occupantDays(moonlight, days).map((d) => d.label)).toEqual(['Day 2'])
  })

  it('names the day under each vendor', () => {
    // The reported symptom: Lunaria showed no "(Day 1)".
    expect(occupantDayNote(lunaria, days)).toBe('Day 1')
    expect(occupantDayNote(moonlight, days)).toBe('Day 2')
  })

  it('shows each vendor only under its own day filter', () => {
    // The other symptom: Lunaria appeared as if open on both days.
    const both = [lunaria, moonlight]
    expect(occupantsOnDayFn(both, day1, days).map((o) => o.title)).toEqual(['Lunaria'])
    expect(occupantsOnDayFn(both, day2, days).map((o) => o.title)).toEqual(['Moonlight Tides'])
    // With no filter, the stall still shows its combined form.
    expect(occupantsOnDayFn(both, null, days)).toHaveLength(2)
  })
})

/**
 * The snippet an admin copies once and pastes onto a site we never see again. A
 * mistake in it is not caught by anything here - it is caught by a stranger
 * looking at a broken tag on someone's Carrd.
 */
describe('festival map embed snippet', () => {
  it('points at the map by whatever it is linked by', () => {
    expect(mapEmbedPath('summer-2026')).toBe('/embed/festival-maps/summer-2026')
    expect(mapEmbedPath('12')).toBe('/embed/festival-maps/12') // no shortcode set
  })

  it('builds a pasteable iframe for the map', () => {
    const html = mapEmbedSnippet('https://senpan.cafe/embed/festival-maps/summer', 'Summer Night')
    expect(html).toContain('src="https://senpan.cafe/embed/festival-maps/summer"')
    expect(html).toContain('width="100%"')
    expect(html).toContain(`height="${MAP_EMBED_HEIGHT}"`)
    expect(html).toContain('title="Summer Night - festival map"')
    // A closing tag, not a self-closed one: an <iframe> that never closes
    // swallows the rest of the host's page.
    expect(html.endsWith('></iframe>')).toBe(true)
  })

  /**
   * The one that matters. A festival called `Yoey's "Big" Night` would close the
   * title attribute early, and the paste would land on someone else's live page as
   * a broken tag with stray words after it - with nothing between the admin and
   * that but a copy button.
   */
  it('escapes a title that would otherwise break out of the attribute', () => {
    const html = mapEmbedSnippet(
      'https://senpan.cafe/embed/festival-maps/1',
      'Yoey\'s "Big" <Night>',
    )
    expect(html).toContain('title="Yoey\'s &quot;Big&quot; &lt;Night&gt; - festival map"')
    // Every attribute value still opens and closes cleanly, so nothing in the
    // title has started an attribute of its own - or a tag of its own.
    const attrs = [...html.matchAll(/([a-z]+)="([^"]*)"/g)].map((m) => m[1])
    expect(attrs).toEqual(['src', 'title', 'width', 'height', 'loading', 'style'])
    expect(html).not.toContain('<Night>')
  })

  it('escapes the URL too, and names an untitled map something', () => {
    expect(mapEmbedSnippet('https://x/y?a=1&b=2', '')).toContain('src="https://x/y?a=1&amp;b=2"')
    expect(mapEmbedSnippet('https://x/y', '   ')).toContain('title="Festival map"')
  })
})
