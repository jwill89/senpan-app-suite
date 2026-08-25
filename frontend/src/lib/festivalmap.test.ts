import { describe, it, expect } from 'vitest'
import {
  STALL_TYPES,
  circleHeightPct,
  occupantsOnDay,
  rangesOverlap,
  formatEventTime,
  isCircle,
  isWithin,
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
    const line = formatEventTime(time({ start: '2026-08-01T18:00:00Z' }))
    expect(line.startsWith('-')).toBe(false)
    expect(line).not.toContain(' - ')
  })

  it('reads as a start time alone when there is no end', () => {
    const withEnd = formatEventTime(
      time({ start: '2026-08-01T18:00:00Z', end: '2026-08-01T22:00:00Z' }),
    )
    const openEnded = formatEventTime(time({ start: '2026-08-01T18:00:00Z' }))
    expect(withEnd).toContain(' to ')
    expect(openEnded).not.toContain(' to ')
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
