<script setup lang="ts">
/**
 * Read-only Stamp Rally card: the fully-designed card background image with the
 * earned stamp/prize art overlaid at each slot's %-based {@link Placement} (rotated
 * about its centre). Used by the public participant view and the admin detail preview.
 *
 * The card image is the complete design — decorative frame, the "?"/empty slot
 * placeholders, the stall labels, and any prize panel are all part of it. This
 * component therefore only draws an item when it has an image to show (a collected
 * stamp, or a revealed prize): an item with no image renders nothing, letting the
 * card's own slot art show through. That turns the "empty" card into the "full" one
 * slot by slot. (An optional not-stamped overlay can still be supplied per item for
 * cards whose slots aren't pre-marked.)
 */
import { assetUrl } from '@/lib/assets'
import { placementStyle } from '@/lib/stampcard'
import type { Placement } from '@/types/api'

export interface CanvasItem {
  key: string | number
  /** Image to overlay; '' draws nothing (the card's own slot art shows through). */
  image: string
  placement: Placement
}

defineProps<{
  /** Card background image path/URL ('' shows a neutral placeholder box). */
  cardImage: string
  items: CanvasItem[]
}>()
</script>

<template>
  <div class="stamp-canvas">
    <img v-if="cardImage" :src="assetUrl(cardImage)" class="stamp-canvas-bg" alt="Stamp card" />
    <div v-else class="stamp-canvas-bg stamp-canvas-empty">
      <font-awesome-icon :icon="['fad', 'image']" />
    </div>
    <!-- Only items with art are drawn; empty slots fall through to the card design. -->
    <div
      v-for="item in items"
      v-show="item.image"
      :key="item.key"
      class="stamp-canvas-item"
      :style="placementStyle(item.placement)"
    >
      <img v-if="item.image" :src="assetUrl(item.image)" alt="" />
    </div>
  </div>
</template>

<style scoped>
.stamp-canvas {
  position: relative;
  width: 100%;
  /* Items are absolutely positioned in % of this box; the bg image sizes it. */
}
.stamp-canvas-bg {
  display: block;
  width: 100%;
  height: auto;
  border-radius: var(--radius);
}
.stamp-canvas-empty {
  aspect-ratio: 16 / 10;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 2rem;
  color: var(--text-dim);
  background: var(--panel-raised-bg);
  border: 1px dashed var(--control-border);
}
.stamp-canvas-item {
  position: absolute;
  display: flex;
  align-items: center;
  justify-content: center;
}
.stamp-canvas-item img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  pointer-events: none;
}
</style>
