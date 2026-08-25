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
import type { EventTime, Placement, StallShape, StallType, StampType } from '@/types/api'
import { formatServerTimestamp, parseServerTimestamp } from '@/lib/datetime'

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
export function stallStyle(placement: Placement, shape: string, color: string): CSSProperties {
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
 * One datetime range as a single readable line: "Day 1 - Aug 1, 6:00 PM to
 * 10:00 PM". A range with no end reads as a start time alone, and an unlabeled
 * one drops the prefix rather than showing a stray dash.
 */
export function formatEventTime(time: EventTime): string {
  const start = formatServerTimestamp(time.start)
  const span = time.end ? `${start} to ${formatServerTimestamp(time.end)}` : start
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
): T[] {
  if (!day) return occupants
  return occupants.filter((o) => o.times.length === 0 || o.times.some((t) => rangesOverlap(t, day)))
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
