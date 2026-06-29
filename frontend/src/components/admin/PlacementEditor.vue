<script setup lang="ts">
/**
 * Interactive placement editor for Stamp Rally stamps + prizes. Renders the card
 * background with each item positioned by its %-based {@link Placement}, and lets the
 * admin DRAG an item to move it, drag the bottom-right handle to RESIZE it, and drag
 * the top handle to ROTATE it. Positions are emitted as percentages of the card box
 * so they render identically at any display size (read-only view = StampCardCanvas).
 *
 * The parent owns the stamp/prize arrays; this component is controlled — it emits
 * `select` and `update(key, placement)` rather than mutating props. Continuous drags
 * emit on each pointermove; the parent applies the new placement to the matching item.
 */
import { ref } from 'vue'
import { assetUrl } from '@/lib/assets'
import { placementStyle } from '@/lib/stampcard'
import type { Placement } from '@/types/api'

export interface PlaceItem {
  key: string
  label: string
  image: string
  placement: Placement
  kind: 'stamp' | 'prize'
}

defineProps<{
  cardImage: string
  items: PlaceItem[]
  selectedKey: string | null
}>()

const emit = defineEmits<{
  select: [key: string | null]
  update: [key: string, placement: Placement]
}>()

const canvasRef = ref<HTMLElement | null>(null)

type Mode = 'move' | 'resize' | 'rotate'
interface DragState {
  mode: Mode
  key: string
  startX: number
  startY: number
  start: Placement
  rect: DOMRect
}
let drag: DragState | null = null

const MIN = 3 // minimum item size, % of card

function clamp(v: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, v))
}

function beginDrag(mode: Mode, item: PlaceItem, e: PointerEvent): void {
  const rect = canvasRef.value?.getBoundingClientRect()
  if (!rect) return
  emit('select', item.key)
  drag = {
    mode,
    key: item.key,
    startX: e.clientX,
    startY: e.clientY,
    start: { ...item.placement },
    rect,
  }
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', endDrag)
  e.preventDefault()
}

function onPointerMove(e: PointerEvent): void {
  if (!drag) return
  const { rect, start } = drag
  const dxPct = ((e.clientX - drag.startX) / rect.width) * 100
  const dyPct = ((e.clientY - drag.startY) / rect.height) * 100

  let next: Placement
  if (drag.mode === 'move') {
    next = {
      ...start,
      x: clamp(start.x + dxPct, 0, 100 - start.width),
      y: clamp(start.y + dyPct, 0, 100 - start.height),
    }
  } else if (drag.mode === 'resize') {
    // Project the screen delta onto the item's rotated local axes so resizing feels
    // natural even when the item is rotated (top-left stays anchored).
    const rad = (start.rotation * Math.PI) / 180
    const cos = Math.cos(rad)
    const sin = Math.sin(rad)
    const localW = dxPct * cos + dyPct * sin
    const localH = -dxPct * sin + dyPct * cos
    next = {
      ...start,
      width: clamp(start.width + localW, MIN, 100 - start.x),
      height: clamp(start.height + localH, MIN, 100 - start.y),
    }
  } else {
    // Rotate: angle from the item's centre to the pointer; the handle sits above the
    // item, so add 90° to map "pointer straight up" → 0°.
    const cx = rect.left + ((start.x + start.width / 2) / 100) * rect.width
    const cy = rect.top + ((start.y + start.height / 2) / 100) * rect.height
    const deg = (Math.atan2(e.clientY - cy, e.clientX - cx) * 180) / Math.PI + 90
    next = { ...start, rotation: Math.round(deg) }
  }
  emit('update', drag.key, next)
}

function endDrag(): void {
  drag = null
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', endDrag)
}

/** Click on empty card area → deselect. */
function onCanvasPointerDown(e: PointerEvent): void {
  if (e.target === canvasRef.value || (e.target as HTMLElement).classList.contains('pe-bg')) {
    emit('select', null)
  }
}
</script>

<template>
  <div class="pe-wrap">
    <div ref="canvasRef" class="pe-canvas" @pointerdown="onCanvasPointerDown">
      <img v-if="cardImage" :src="assetUrl(cardImage)" class="pe-bg" alt="Stamp card" draggable="false" />
      <div v-else class="pe-bg pe-empty"><font-awesome-icon :icon="['fad', 'image']" /> Pick a card image</div>

      <div
        v-for="item in items"
        :key="item.key"
        class="pe-item"
        :class="{ selected: item.key === selectedKey, prize: item.kind === 'prize' }"
        :style="placementStyle(item.placement)"
        @pointerdown="beginDrag('move', item, $event)"
      >
        <img v-if="item.image" :src="assetUrl(item.image)" alt="" draggable="false" />
        <div v-else class="pe-item-empty">{{ item.label }}</div>

        <template v-if="item.key === selectedKey">
          <!-- Rotate handle (above, top-centre) -->
          <button
            class="pe-handle pe-rotate"
            type="button"
            aria-label="Rotate"
            title="Drag to rotate"
            @pointerdown.stop="beginDrag('rotate', item, $event)"
          >
            <font-awesome-icon :icon="['fas', 'rotate']" />
          </button>
          <!-- Resize handle (bottom-right) -->
          <button
            class="pe-handle pe-resize"
            type="button"
            aria-label="Resize"
            title="Drag to resize"
            @pointerdown.stop="beginDrag('resize', item, $event)"
          ></button>
        </template>
      </div>
    </div>
    <p class="pe-hint text-dim text-xs">
      Drag an item to move · drag the corner to resize · drag the top handle to rotate. Positions
      are saved as a share of the card, so they scale to any screen.
    </p>
  </div>
</template>

<style scoped>
.pe-canvas {
  position: relative;
  width: 100%;
  user-select: none;
  touch-action: none;
}
.pe-bg {
  display: block;
  width: 100%;
  height: auto;
  border-radius: var(--radius);
}
.pe-empty {
  aspect-ratio: 16 / 10;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  color: var(--text-dim);
  background: var(--panel-raised-bg);
  border: 1px dashed var(--control-border);
}
.pe-item {
  position: absolute;
  cursor: move;
  box-sizing: border-box;
  border: 1px solid transparent;
}
.pe-item.selected {
  border-color: var(--highlight);
  box-shadow: 0 0 0 1px var(--highlight);
}
.pe-item.prize:not(.selected) {
  border-color: color-mix(in srgb, var(--highlight) 40%, transparent);
  border-style: dashed;
}
.pe-item img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}
.pe-item-empty {
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.7rem;
  text-align: center;
  color: var(--text-dim);
  background: color-mix(in srgb, var(--panel-raised-bg) 80%, transparent);
  border: 1px dashed var(--control-border);
  border-radius: 4px;
}
.pe-handle {
  position: absolute;
  padding: 0;
  border: 2px solid var(--highlight);
  background: var(--page-bg);
  border-radius: 50%;
  cursor: pointer;
}
.pe-rotate {
  width: 22px;
  height: 22px;
  top: -34px;
  left: 50%;
  transform: translateX(-50%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 0.7rem;
  color: var(--highlight);
  cursor: grab;
}
.pe-resize {
  width: 16px;
  height: 16px;
  right: -8px;
  bottom: -8px;
  border-radius: 3px;
  cursor: nwse-resize;
}
.pe-hint {
  margin-top: 8px;
}
</style>
