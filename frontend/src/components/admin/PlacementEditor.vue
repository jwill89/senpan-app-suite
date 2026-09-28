<script setup lang="ts">
/**
 * Interactive placement editor for Stamp Rally stamps + prizes. Renders the card
 * background with each item positioned by its %-based {@link Placement}, and lets the
 * admin DRAG an item to move it, drag the bottom-right handle to RESIZE it, and drag
 * the top handle to ROTATE it. Positions are emitted as percentages of the card box
 * so they render identically at any display size (read-only view = StampCardCanvas).
 * The drag geometry and the `.placement-*` styles are shared with the Festival Map
 * stall editor (composables/usePlacementDrag.ts, assets/styles/mapeditor.css).
 *
 * The parent owns the stamp/prize arrays; this component is controlled - it emits
 * `select` and `update(key, placement)` rather than mutating props. Continuous drags
 * emit on each pointermove; the parent applies the new placement to the matching item.
 */
import { ref } from 'vue'
import { assetUrl } from '@/lib/assets'
import { placementStyle } from '@/lib/stampcard'
import { usePlacementDrag, type DragMode } from '@/composables/usePlacementDrag'
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

// Move/resize/rotate geometry is shared with the Festival Map stall editor.
const { beginDrag: startDrag } = usePlacementDrag<string>(canvasRef, (key, placement) =>
  emit('update', key, placement),
)

/** Selects the item, then starts dragging it (grabbing one always picks it). */
function beginDrag(mode: DragMode, item: PlaceItem, e: PointerEvent): void {
  emit('select', item.key)
  startDrag(mode, item.key, item.placement, e)
}

/**
 * Keyboard equivalent of grabbing an item. Dragging is inherently pointer-only,
 * but SELECTING is not: once selected, the form's numeric position/size/rotation
 * fields edit exactly what a drag would, so a keyboard user can place a stamp or
 * prize precisely. Without this the editor was unreachable without a mouse.
 */
function selectItem(item: PlaceItem): void {
  emit('select', item.key)
}

/** An item's accessible name: what it is, plus its place in the list. */
function itemLabel(item: PlaceItem, index: number): string {
  return `${item.kind === 'prize' ? 'Prize' : 'Stamp'} ${index + 1}: ${item.label} - select to edit its position and size`
}

/** Click on empty card area -> deselect. */
function onCanvasPointerDown(e: PointerEvent): void {
  if (
    e.target === canvasRef.value ||
    (e.target as HTMLElement).classList.contains('placement-bg')
  ) {
    emit('select', null)
  }
}
</script>

<template>
  <div>
    <div ref="canvasRef" class="placement-canvas" @pointerdown="onCanvasPointerDown">
      <img
        v-if="cardImage"
        :src="assetUrl(cardImage)"
        class="placement-bg"
        alt="Stamp card"
        draggable="false"
      />
      <div v-else class="placement-bg placement-empty">
        <font-awesome-icon :icon="['fad', 'image']" /> Pick a card image
      </div>

      <div
        v-for="(item, itemIndex) in items"
        :key="item.key"
        class="placement-item"
        :class="{
          'is-selected': item.key === selectedKey,
          'placement-item--prize': item.kind === 'prize',
        }"
        :style="placementStyle(item.placement)"
        role="button"
        tabindex="0"
        :aria-pressed="item.key === selectedKey"
        :aria-label="itemLabel(item, itemIndex)"
        @pointerdown="beginDrag('move', item, $event)"
        @keydown.enter.prevent="selectItem(item)"
        @keydown.space.prevent="selectItem(item)"
      >
        <img v-if="item.image" :src="assetUrl(item.image)" alt="" draggable="false" />
        <div v-else class="placement-item-empty">{{ item.label }}</div>

        <template v-if="item.key === selectedKey">
          <!-- Rotate handle (above, top-centre) -->
          <button
            class="placement-handle placement-rotate"
            type="button"
            aria-label="Rotate"
            title="Drag to rotate"
            @pointerdown.stop="beginDrag('rotate', item, $event)"
          >
            <font-awesome-icon :icon="['fas', 'rotate']" />
          </button>
          <!-- Resize handle (bottom-right) -->
          <button
            class="placement-handle placement-resize"
            type="button"
            aria-label="Resize"
            title="Drag to resize"
            @pointerdown.stop="beginDrag('resize', item, $event)"
          ></button>
        </template>
      </div>
    </div>
    <p class="placement-hint text-muted text-xs">
      Drag an item to move - drag the corner to resize - drag the top handle to rotate. Positions
      are saved as a share of the card, so they scale to any screen.
    </p>
  </div>
</template>
