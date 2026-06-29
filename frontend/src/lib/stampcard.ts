/**
 * Shared helpers for the Stamp Rally card canvas (admin editor + public view).
 *
 * A stamp/prize is positioned by a {@link Placement}: x/y/width/height as
 * percentages of the card image's box and rotation in degrees. The same CSS is used
 * to render an item in the read-only canvas, the interactive editor, and the public
 * card, so the style computation lives here in one place.
 */
import type { CSSProperties } from 'vue'
import type { Placement } from '@/types/api'

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

/** Display name for a stamp's stall: its affiliate, or the Senpan Tea House default. */
export function stallName(affiliateName: string): string {
  return affiliateName.trim() || 'Senpan Tea House'
}
