<script setup lang="ts">
/**
 * Read-only Festival Map: the base floor-plan image with every stall drawn on top
 * at its %-based {@link Placement} - a circle or rectangle in the stall's color
 * with its title inside - inside a pan/zoom viewport.
 *
 * Panning is a pointer drag on the viewport; zoom is the wheel (cursor-anchored),
 * pinch on touch, or the +/-/reset buttons the parent renders against the exposed
 * `zoomBy`/`reset` methods. Clicking a stall emits `select` so the parent can open
 * its details; `dimmedIds` pushes the stalls a filter excluded into the background
 * without removing them, so the plan keeps its shape.
 *
 * The interactive editor counterpart is admin/MapStallEditor.vue; both draw the
 * same shapes through lib/festivalmap.ts, so a stall looks identical in each.
 */
import { computed, onBeforeUnmount, ref } from 'vue'
import { assetUrl } from '@/lib/assets'
import { isCircle, occupantsOnDay, stallCaption, stallColor, stallStyle } from '@/lib/festivalmap'
import type { EventTime, PublicFestivalStall, PublicStallOccupant } from '@/types/api'

const props = withDefaults(
  defineProps<{
    mapImage: string
    stalls: PublicFestivalStall[]
    /** Pitch id currently open in the parent's detail panel. */
    selectedId?: number | null
    /** Pitch ids to push into the background (a filter excluded them). */
    dimmedIds?: Set<number>
    /** Pitch ids that carry a stamp on the open rally (drawn with a ring). */
    rallyIds?: Set<number>
    /**
     * The festival day being browsed, or null for "all". A pitch that changes
     * hands between days still draws every occupant - the plan keeps its shape -
     * but the ones not on that day are pushed back behind the one who is.
     */
    day?: EventTime | null
  }>(),
  { selectedId: null, dimmedIds: undefined, rallyIds: undefined, day: null },
)

const emit = defineEmits<{ select: [stall: PublicFestivalStall] }>()

const MIN_ZOOM = 1
const MAX_ZOOM = 6

const viewportRef = ref<HTMLElement | null>(null)
const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const dragging = ref(false)

const panStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`,
}))

function clamp(value: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, value))
}

/**
 * Keeps the scaled canvas covering the viewport, so panning can never drag the
 * map off screen and leave the visitor looking at empty background. At zoom 1
 * the canvas exactly fills the viewport and both offsets settle at 0.
 */
function clampPan(): void {
  const box = viewportRef.value?.getBoundingClientRect()
  if (!box) return
  const overflowX = box.width * (zoom.value - 1)
  const overflowY = box.height * (zoom.value - 1)
  panX.value = clamp(panX.value, -overflowX, 0)
  panY.value = clamp(panY.value, -overflowY, 0)
}

/**
 * Zooms by a factor, keeping the point under (clientX, clientY) fixed - so a
 * wheel over a stall zooms into that stall rather than the middle of the plan.
 * Without a point, zooms about the viewport's centre.
 */
function zoomBy(factor: number, clientX?: number, clientY?: number): void {
  const box = viewportRef.value?.getBoundingClientRect()
  if (!box) return
  const next = clamp(zoom.value * factor, MIN_ZOOM, MAX_ZOOM)
  if (next === zoom.value) return
  const anchorX = (clientX ?? box.left + box.width / 2) - box.left
  const anchorY = (clientY ?? box.top + box.height / 2) - box.top
  // Solve for the pan that leaves the anchor over the same canvas point.
  const ratio = next / zoom.value
  panX.value = anchorX - (anchorX - panX.value) * ratio
  panY.value = anchorY - (anchorY - panY.value) * ratio
  zoom.value = next
  clampPan()
}

/** Back to the whole plan, centred. */
function reset(): void {
  zoom.value = 1
  panX.value = 0
  panY.value = 0
}

function onWheel(e: WheelEvent): void {
  e.preventDefault()
  zoomBy(e.deltaY < 0 ? 1.15 : 1 / 1.15, e.clientX, e.clientY)
}

// -- Pan (pointer drag) + pinch (two pointers) -------------------------------
// Pointer ids are tracked so a second finger turns the drag into a pinch instead
// of fighting it, and so a lifted-off-screen pointer can't leave a drag running.
const pointers = new Map<number, { x: number; y: number }>()
let panStart = { x: 0, y: 0, panX: 0, panY: 0 }
let pinchStart = 0

/** Distance between the two active pointers (0 when there aren't two). */
function pinchDistance(): number {
  if (pointers.size < 2) return 0
  const [a, b] = [...pointers.values()]
  return Math.hypot(a.x - b.x, a.y - b.y)
}

function onPointerDown(e: PointerEvent): void {
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })
  if (pointers.size === 2) {
    pinchStart = pinchDistance()
    dragging.value = false
    return
  }
  dragging.value = true
  panStart = { x: e.clientX, y: e.clientY, panX: panX.value, panY: panY.value }
  window.addEventListener('pointermove', onPointerMove)
  window.addEventListener('pointerup', onPointerUp)
  window.addEventListener('pointercancel', onPointerUp)
}

function onPointerMove(e: PointerEvent): void {
  if (!pointers.has(e.pointerId)) return
  pointers.set(e.pointerId, { x: e.clientX, y: e.clientY })

  if (pointers.size === 2) {
    const distance = pinchDistance()
    if (pinchStart > 0 && distance > 0) {
      const [a, b] = [...pointers.values()]
      zoomBy(distance / pinchStart, (a.x + b.x) / 2, (a.y + b.y) / 2)
      pinchStart = distance
    }
    return
  }
  if (!dragging.value) return
  panX.value = panStart.panX + (e.clientX - panStart.x)
  panY.value = panStart.panY + (e.clientY - panStart.y)
  clampPan()
}

function onPointerUp(e: PointerEvent): void {
  pointers.delete(e.pointerId)
  if (pointers.size < 2) pinchStart = 0
  if (pointers.size > 0) return
  dragging.value = false
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
}

// A viewport unmounted mid-drag would otherwise leak the window listeners.
onBeforeUnmount(() => {
  pointers.clear()
  window.removeEventListener('pointermove', onPointerMove)
  window.removeEventListener('pointerup', onPointerUp)
  window.removeEventListener('pointercancel', onPointerUp)
})

/**
 * A click that ended a pan must not also open a stall. The drag flag is already
 * cleared by pointerup, so compare against where the press started instead: a
 * movement under a few pixels is a click, anything more was a drag.
 */
function onStallPointerUp(stall: PublicFestivalStall, e: PointerEvent): void {
  const moved = Math.hypot(e.clientX - panStart.x, e.clientY - panStart.y)
  if (moved > 4) return
  emit('select', stall)
}

const isDimmed = (id: number): boolean => props.dimmedIds?.has(id) ?? false
const isRally = (id: number): boolean => props.rallyIds?.has(id) ?? false

/**
 * The stamp art to corner-badge a pitch with: the first occupant on the browsed
 * day that carries one, so a pitch shows the stamp belonging to whoever is
 * actually there rather than whichever tenant happens to be listed first.
 */
function stampArt(stall: PublicFestivalStall): string {
  const onDay = occupantsOnDay(stall.occupants, props.day)
  return onDay.find((o) => o.stamp_image)?.stamp_image ?? ''
}

/** Whether an occupant is one of those standing here on the day being browsed. */
function onSelectedDay(stall: PublicFestivalStall, occupant: PublicStallOccupant): boolean {
  if (!props.day) return true
  return occupantsOnDay(stall.occupants, props.day).includes(occupant)
}

defineExpose({ zoomBy, reset, zoom })
</script>

<template>
  <div
    ref="viewportRef"
    class="map-viewport"
    :class="{ 'is-dragging': dragging }"
    @wheel="onWheel"
    @pointerdown="onPointerDown"
  >
    <div class="map-pan" :style="panStyle">
      <div class="map-canvas">
        <img
          v-if="mapImage"
          :src="assetUrl(mapImage)"
          class="map-canvas-bg"
          alt="Festival map"
          draggable="false"
        />
        <div v-else class="map-canvas-bg map-canvas-empty">
          <font-awesome-icon :icon="['fad', 'image']" />
        </div>

        <div
          v-for="stall in stalls"
          :key="stall.id"
          class="map-stall is-interactive"
          :class="{
            'is-selected': stall.id === selectedId,
            'is-dimmed': isDimmed(stall.id),
            'is-rally': isRally(stall.id),
            'map-stall--round': isCircle(stall.shape),
          }"
          :style="stallStyle(stall.placement, stall.shape, stallColor(stall))"
          role="button"
          tabindex="0"
          :aria-label="`${stall.occupants.map((o) => o.title).join(', ')} - open details`"
          @pointerup="onStallPointerUp(stall, $event)"
          @keydown.enter.prevent="emit('select', stall)"
          @keydown.space.prevent="emit('select', stall)"
        >
          <span class="map-stall-label">
            <span
              v-for="occupant in stall.occupants"
              :key="occupant.id"
              class="map-stall-occupant"
              :class="{ 'is-dimmed': !onSelectedDay(stall, occupant) }"
            >
              <span>{{ occupant.title }}</span>
              <span v-if="stallCaption(occupant)" class="map-stall-caption">{{
                stallCaption(occupant)
              }}</span>
            </span>
          </span>
          <img
            v-if="stampArt(stall)"
            :src="assetUrl(stampArt(stall))"
            class="map-stall-stamp"
            alt=""
            draggable="false"
          />
        </div>
      </div>
    </div>
  </div>
</template>
