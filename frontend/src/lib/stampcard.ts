/**
 * Shared helpers for the Stamp Rally card canvas (admin editor + public view).
 *
 * A stamp/prize is positioned by a {@link Placement}: x/y/width/height as
 * percentages of the card image's box and rotation in degrees. The same CSS is used
 * to render an item in the read-only canvas, the interactive editor, and the public
 * card, so the style computation lives here in one place.
 */
import type { CSSProperties } from 'vue'
import type { Placement, StampType } from '@/types/api'

/** Absolute-position style for an item at the given placement (rotated about its centre). */
export function placementStyle(p: Placement): CSSProperties {
  return {
    left: `${p.x}%`,
    top: `${p.y}%`,
    width: `${p.width}%`,
    height: `${p.height}%`,
    transform: `rotate(${p.rotation}deg)`,
    transformOrigin: 'center center',
  }
}

/**
 * Display name for a stamp's stall, mirroring Go's StampRallyStamp.DisplayStall:
 * the first non-blank of the map occupant's stall name, then the affiliate, then
 * the venue's own default.
 *
 * The stall name has to come first. A rally linked to a festival map takes its
 * stamps from PITCH OCCUPANTS, and the server joins that occupant's name - so
 * naming a map-linked stamp by its affiliate showed the partner behind the booth
 * rather than the booth, and "Senpan Tea House" for any occupant with no affiliate
 * at all, which is exactly the case a festival map exists to describe.
 */
export function stallName(stallNameValue: string, affiliateName = ''): string {
  return stallNameValue.trim() || affiliateName.trim() || 'Senpan Tea House'
}

/**
 * The stamp types, in picker order. Anything unrecognized (including the empty
 * value every stamp written before types existed carries) reads as a food stamp,
 * matching the server's normalization.
 */
export const STAMP_TYPES: { value: StampType; label: string }[] = [
  { value: 'food', label: 'Food Stamp' },
  { value: 'game', label: 'Game Stamp' },
]

/** Label for a stamp type ("Food Stamp" / "Game Stamp"). */
export function stampTypeLabel(type: string): string {
  return type === 'game' ? 'Game Stamp' : 'Food Stamp'
}

/** Short label for a stamp type, for tables and badges ("Food" / "Game"). */
export function stampTypeShort(type: string): string {
  return type === 'game' ? 'Game' : 'Food'
}
