/**
 * Pointer-drag math for the %-based {@link Placement} editors.
 *
 * Both visual editors - the Stamp Rally card (PlacementEditor) and the Festival
 * Map floor plan (MapStallEditor) - place items by dragging them around a
 * background image, resizing from a corner handle and rotating from a handle
 * above. The geometry is identical and fiddly (a resize has to be projected onto
 * the item's ROTATED local axes, or dragging a tilted item feels wrong), so it
 * lives here once instead of in each editor.
 *
 * The composable owns no item state: it reports the new placement through
 * `onUpdate` and lets the caller apply it to whatever it is editing.
 */
import { onBeforeUnmount, type Ref } from 'vue'
import { circleHeightPct } from '@/lib/festivalmap'
import type { Placement } from '@/types/api'

/** What a drag is doing to the item under the pointer. */
export type DragMode = 'move' | 'resize' | 'rotate'

/** Minimum item size, as a percentage of the background box. */
const MIN_SIZE = 3

function clamp(value: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, value))
}

/** Per-drag options the caller passes to {@link usePlacementDrag}'s beginDrag. */
export interface DragOptions {
  /**
   * Keep the item SQUARE on screen: its height follows its rendered width, so it
   * draws as a true circle whatever the background image's aspect ratio (see
   * `stallStyle`). Resizing then moves the width alone, and the stored height is
   * left untouched - it still describes the item's rectangle form.
   */
  square?: boolean
}

interface DragState<K> {
  mode: DragMode
  key: K
  startX: number
  startY: number
  start: Placement
  rect: DOMRect
  square: boolean
}

/**
 * The item's on-screen height as a percentage of the box: derived from the width
 * for a square item, else simply its stored height. This is what bounds a drag
 * against the bottom edge, so a circle can't be pushed off the plan by a height
 * the stylesheet never uses.
 */
function renderedHeightPct(start: Placement, square: boolean, rect: DOMRect): number {
  return square ? circleHeightPct(start.width, rect.width, rect.height) : start.height
}

/**
 * Wires up placement dragging over the element in `canvasRef`.
 *
 * `onUpdate(key, placement)` fires on every pointermove for the item identified
 * by `key` (whatever the caller uses to identify items - an index string, a uid).
 */
export function usePlacementDrag<K>(
  canvasRef: Ref<HTMLElement | null>,
  onUpdate: (key: K, placement: Placement) => void,
) {
  let drag: DragState<K> | null = null

  function onPointerMove(e: PointerEvent): void {
    if (!drag) return
    const { rect, start } = drag
    const dxPct = ((e.clientX - drag.startX) / rect.width) * 100
    const dyPct = ((e.clientY - drag.startY) / rect.height) * 100

    const heightPct = renderedHeightPct(start, drag.square, rect)

    let next: Placement
    if (drag.mode === 'move') {
      next = {
        ...start,
        x: clamp(start.x + dxPct, 0, 100 - start.width),
        y: clamp(start.y + dyPct, 0, 100 - heightPct),
      }
    } else if (drag.mode === 'resize') {
      // Project the screen delta onto the item's rotated local axes so resizing
      // feels natural even when the item is rotated (top-left stays anchored).
      const rad = (start.rotation * Math.PI) / 180
      const cos = Math.cos(rad)
      const sin = Math.sin(rad)
      const localW = dxPct * cos + dyPct * sin
      const localH = -dxPct * sin + dyPct * cos
      if (drag.square) {
        // Height follows width, so only the width is dragged - capped so the
        // height it implies still fits above the bottom edge.
        const widthFromBottom = start.width * ((100 - start.y) / Math.max(heightPct, 0.001))
        next = {
          ...start,
          width: clamp(start.width + localW, MIN_SIZE, Math.min(100 - start.x, widthFromBottom)),
        }
      } else {
        next = {
          ...start,
          width: clamp(start.width + localW, MIN_SIZE, 100 - start.x),
          height: clamp(start.height + localH, MIN_SIZE, 100 - start.y),
        }
      }
    } else {
      // Rotate: angle from the item's centre to the pointer; the handle sits
      // above the item, so add 90 degrees to map "pointer straight up" -> 0.
      // The centre uses the RENDERED height, or a circle would pivot about a
      // point that isn't its middle.
      const cx = rect.left + ((start.x + start.width / 2) / 100) * rect.width
      const cy = rect.top + ((start.y + heightPct / 2) / 100) * rect.height
      const deg = (Math.atan2(e.clientY - cy, e.clientX - cx) * 180) / Math.PI + 90
      next = { ...start, rotation: Math.round(deg) }
    }
    onUpdate(drag.key, next)
  }

  function endDrag(): void {
    drag = null
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', endDrag)
  }

  /** Starts a drag on `key`'s item, whose current placement is `placement`. */
  function beginDrag(
    mode: DragMode,
    key: K,
    placement: Placement,
    e: PointerEvent,
    opts: DragOptions = {},
  ): void {
    const rect = canvasRef.value?.getBoundingClientRect()
    if (!rect) return
    drag = {
      mode,
      key,
      startX: e.clientX,
      startY: e.clientY,
      start: { ...placement },
      rect,
      square: opts.square ?? false,
    }
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', endDrag)
    e.preventDefault()
  }

  // If the editor unmounts mid-drag the window listeners would otherwise leak
  // (endDrag never fires). Tear them down defensively on unmount.
  onBeforeUnmount(endDrag)

  return { beginDrag, endDrag }
}
