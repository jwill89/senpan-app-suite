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
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { assetUrl } from '@/lib/assets'
import {
  clampPan as clampPanTo,
  fitZoom as fitZoomFor,
  dayReference,
  isCircle,
  occupantDayNote,
  occupantMapLabel,
  occupantsOnDay,
  stallCaption,
  stallColor,
  stallStyle,
  viewportAspect,
} from '@/lib/festivalmap'
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
    /**
     * Every day the festival runs. Needed to tell an occupant who is only there
     * for SOME of them from one who is there throughout - only the former has a
     * day worth naming on its label.
     */
    festivalDays?: EventTime[]
  }>(),
  {
    selectedId: null,
    dimmedIds: undefined,
    rallyIds: undefined,
    day: null,
    festivalDays: undefined,
  },
)

const emit = defineEmits<{
  select: [stall: PublicFestivalStall]
  /**
   * The shape the FRAME should take, once the plan's image has loaded - already
   * clamped by viewportAspect, so it is what the frame really is rather than what
   * the image is. The parent needs it to cap its own width to match: see
   * FestivalMapWidget's frameAspect.
   */
  'frame-aspect': [ratio: number]
}>()

const MAX_ZOOM = 6

const viewportRef = ref<HTMLElement | null>(null)
const canvasRef = ref<HTMLElement | null>(null)
const zoom = ref(1)
const panX = ref(0)
const panY = ref(0)
const dragging = ref(false)

/**
 * The canvas's UNZOOMED rendered size. Measured rather than assumed: the viewport
 * is a fixed shape but the canvas takes its height from the plan image, so the two
 * only match when the image happens to share that aspect.
 *
 * Assuming they matched is what broke this view. Pan was clamped against the
 * VIEWPORT, so at zoom 1 the allowed range was zero in both axes - a taller plan
 * was cut off at the bottom with no way to drag to it - and "Fit" reset to that
 * same cropped view because a floor of 1 made zooming out to the whole plan
 * impossible.
 */
const canvasSize = ref({ width: 0, height: 0 })

/**
 * The plan image's own aspect (width / height), once it has loaded. 0 until then.
 *
 * The viewport used to be a fixed 16/10 whatever the plan was, so anything taller
 * had to be zoomed out to fit and sat letterboxed between empty margins - readable
 * only by zooming back in. Taking the shape from the image means the plan fills the
 * frame it is given, at the size it was drawn.
 */
const imageAspect = ref(0)

/**
 * The plan's shape, handed to the stylesheet as a variable rather than set as an
 * `aspect-ratio` here.
 *
 * The stylesheet needs the NUMBER, not just the resulting shape, because it has to
 * cap the WIDTH as well: the frame's height is capped so a tall plan cannot push
 * the page down, and the width it may take at that height follows from this aspect.
 * As a variable the framed embed can also override the shape in plain CSS - which
 * an inline `aspect-ratio` would have outranked - and the widget can be centred at
 * the same width, which is what stops a narrowed plan opening beside a full-width
 * toolbar with dead page next to it.
 */
const viewportStyle = computed(() => {
  const aspect = viewportAspect(imageAspect.value)
  return aspect > 0 ? { '--map-aspect': String(aspect) } : {}
})

/** The zoom at which the whole plan fits inside the viewport. */
const fitZoom = computed(() => {
  const box = viewportRef.value?.getBoundingClientRect()
  if (!box) return 1
  return fitZoomFor(canvasSize.value, box)
})

/**
 * Never above 1 - a plan smaller than the viewport should sit at its natural size
 * rather than being blown up - and never above the fit, so the whole plan is always
 * reachable however tall it is.
 */
const minZoom = computed(() => Math.min(1, fitZoom.value))

const panStyle = computed(() => ({
  transform: `translate(${panX.value}px, ${panY.value}px) scale(${zoom.value})`,
}))

function clamp(value: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, value))
}

/**
 * Keeps the scaled canvas covering the viewport, so panning can never leave the
 * visitor looking at empty background.
 *
 * Measured against the canvas's real size, not the viewport's. An axis where the
 * scaled canvas is SMALLER than the viewport is centred instead of pinned to 0 -
 * otherwise a plan narrower than the viewport hugs the left edge with dead space
 * beside it.
 */
function clampPan(): void {
  const box = viewportRef.value?.getBoundingClientRect()
  if (!box) return
  const next = clampPanTo({ x: panX.value, y: panY.value }, canvasSize.value, box, zoom.value)
  panX.value = next.x
  panY.value = next.y
}

/**
 * Zooms by a factor, keeping the point under (clientX, clientY) fixed - so a
 * wheel over a stall zooms into that stall rather than the middle of the plan.
 * Without a point, zooms about the viewport's centre.
 */
function zoomBy(factor: number, clientX?: number, clientY?: number): void {
  const box = viewportRef.value?.getBoundingClientRect()
  if (!box) return
  const next = clamp(zoom.value * factor, minZoom.value, MAX_ZOOM)
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

/**
 * Back to the whole plan, centred - which is what "Fit" has to mean for a plan
 * taller than the viewport. Setting zoom to 1 would simply restore the crop.
 */
function reset(): void {
  zoom.value = fitZoom.value
  panX.value = 0
  panY.value = 0
  clampPan()
}

/**
 * Tracks the canvas's rendered size. A ResizeObserver rather than a one-off
 * measurement because the size arrives late and changes afterwards: the plan image
 * has no height until it loads, and the whole canvas is fluid, so the viewport
 * reflows on rotation and on any panel resize around it.
 *
 * The first non-zero measurement fits the plan, so the view opens showing all of
 * it. Later measurements only re-clamp, which keeps the visitor's own zoom and
 * position rather than yanking them back to the top on a resize.
 *
 * The observer is a follow-up, NOT the primary trigger: it delivers during the
 * rendering steps, which a browser skips entirely while the page is hidden, so a
 * map opened in a background tab would never fit. The plan image's own `load`
 * event is what the first fit hangs off - the canvas has no height until then, and
 * that height is the whole input to the calculation.
 */
let resizeObserver: ResizeObserver | null = null
let hasFitted = false

/** Records the plan's own proportions, which the viewport then takes its shape from. */
async function onImageLoad(e: Event): Promise<void> {
  const img = e.target as HTMLImageElement
  if (img.naturalWidth > 0 && img.naturalHeight > 0) {
    imageAspect.value = img.naturalWidth / img.naturalHeight
    emit('frame-aspect', viewportAspect(imageAspect.value))
  }
  // Wait for that new shape to reach the DOM before measuring - here AND in the
  // widget above, which caps its own width to match. The fit is computed FROM the
  // frame, so measuring in the same tick computes it against the old shape and
  // leaves the plan inset inside a frame that was about to fit it exactly.
  await nextTick()
  measureCanvas()
}

/**
 * Whether the frame has taken the PLAN's shape yet.
 *
 * It has not until the image has loaded: the frame's aspect comes from the image,
 * so before that it is still the stylesheet's fallback, and a fit measured against
 * the fallback is a fit against a box the plan is not the shape of. It comes out at
 * exactly fallbackAspect / planAspect - for a 3:2 plan in a 16:10 frame, 0.9375 -
 * so the map opened a few percent small, inset inside its own frame, and pressing
 * "Fit" was the only thing that put it right.
 *
 * The measurement it used to be spent on is real and arrives first whenever the
 * image is already in the browser's cache: the canvas has its full size at mount,
 * before the load event that tells the frame what shape to be. That is the ordinary
 * case for a second visit, which is why this looked intermittent.
 *
 * A map with no image at all has nothing to wait for - its empty state carries its
 * own shape - so it fits immediately.
 */
function frameHasPlanShape(): boolean {
  return imageAspect.value > 0 || !props.mapImage
}

function measureCanvas(): void {
  const el = canvasRef.value
  if (!el) return
  const box = el.getBoundingClientRect()
  if (box.width <= 0 || box.height <= 0) return
  // getBoundingClientRect reports the SCALED box; divide the zoom back out to
  // recover the natural size every calculation here is written against.
  canvasSize.value = { width: box.width / zoom.value, height: box.height / zoom.value }
  if (!hasFitted && frameHasPlanShape()) {
    hasFitted = true
    reset()
    return
  }
  clampPan()
}

onMounted(() => {
  measureCanvas()
  if (typeof ResizeObserver === 'undefined') return
  resizeObserver = new ResizeObserver(() => measureCanvas())
  if (canvasRef.value) resizeObserver.observe(canvasRef.value)
  if (viewportRef.value) resizeObserver.observe(viewportRef.value)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})

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
 * Whether to corner-badge a pitch as part of the open stamp rally, judged over the
 * occupants on the browsed day rather than all of them - a pitch shows the badge
 * for whoever is actually standing there.
 *
 * A BADGE, not the stamp's own artwork. A rally stamp is often a picture of food,
 * and printed on the stall it read as a menu: a visitor took it for what the stall
 * serves rather than for something to collect. The artwork still belongs to the
 * stamp, so it stays where it is being described - in the panel a tapped stall
 * opens - and the plan carries one consistent mark instead.
 *
 * Keyed on in_stamp_rally rather than on having artwork, so a stall whose stamp
 * image has not been uploaded yet is still marked as part of the rally.
 */
/**
 * Day-dependent derivation for every pitch, computed ONCE per (stalls, day) rather
 * than per render.
 *
 * These two answers used to be derived inside the template - stampArt up to twice
 * per stall and onSelectedDay once per occupant - so every pointermove during a pan
 * or pinch re-walked and re-filtered every pitch's occupant list, on a map that is
 * being dragged at frame rate. A computed keyed on the props they actually depend
 * on collapses that to one pass whenever the day or the map really changes.
 */
const perStall = computed(() => {
  const byStall = new Map<
    PublicFestivalStall,
    { shown: Set<PublicStallOccupant>; list: PublicStallOccupant[]; stamped: boolean }
  >()
  for (const stall of props.stalls) {
    const onDay = occupantsOnDay(stall.occupants, props.day, props.festivalDays ?? [])
    byStall.set(stall, {
      shown: new Set(onDay),
      list: onDay,
      stamped: onDay.some((o) => o.in_stamp_rally),
    })
  }
  return byStall
})

/**
 * Who to DRAW in a pitch: only those standing there on the day being browsed.
 *
 * Under a day filter the others used to be drawn faded, which meant a pitch that
 * changes hands still showed two names when only one of them was there - the
 * visitor reads the pitch, not the opacity. A pitch shows its combined form only
 * when two tenants really are there on the same day.
 */
function drawnOccupants(stall: PublicFestivalStall): PublicStallOccupant[] {
  return perStall.value.get(stall)?.list ?? stall.occupants
}

function isStamped(stall: PublicFestivalStall): boolean {
  return perStall.value.get(stall)?.stamped ?? false
}

defineExpose({ zoomBy, reset, zoom })
</script>

<template>
  <div
    ref="viewportRef"
    class="map-viewport"
    :class="{ 'is-dragging': dragging }"
    :style="viewportStyle"
    @wheel="onWheel"
    @pointerdown="onPointerDown"
  >
    <div class="map-pan" :style="panStyle">
      <div ref="canvasRef" class="map-canvas">
        <!-- @load is what triggers the first fit: the canvas has no height until
             the plan has loaded, and that height is the whole input to it. -->
        <img
          v-if="mapImage"
          :src="assetUrl(mapImage)"
          class="map-canvas-bg"
          alt="Festival map"
          draggable="false"
          @load="onImageLoad"
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
          :style="
            stallStyle(
              stall.placement,
              stall.shape,
              stallColor(stall),
              stall.selection_color,
              stall.text_color,
              drawnOccupants(stall).length,
            )
          "
          role="button"
          tabindex="0"
          :aria-label="`${stall.occupants.map((o) => o.title).join(', ')} - open details`"
          @pointerup="onStallPointerUp(stall, $event)"
          @keydown.enter.prevent="emit('select', stall)"
          @keydown.space.prevent="emit('select', stall)"
        >
          <span class="map-stall-label">
            <span
              v-for="occupant in drawnOccupants(stall)"
              :key="occupant.id"
              class="map-stall-occupant"
            >
              <!-- Labelled by AFFILIATE, always: that is who a visitor is looking
                   for, and a stall title is optional so labelling by it left
                   untitled pitches blank. A title's trailing "(Day 1)" - which is
                   how admins have always written it - drops to its own line
                   underneath rather than disappearing. -->
              <span>{{ occupantMapLabel(occupant) }}</span>
              <!-- Only for an occupant who is NOT there every day: naming a day on
                   one present throughout reads as "gone tomorrow". Judged against
                   the festival's days, or - when it declares none - the days this
                   pitch's own occupants describe between them. -->
              <span
                v-if="occupantDayNote(occupant, dayReference(festivalDays ?? [], stall.occupants))"
                class="map-stall-caption"
              >
                ({{ occupantDayNote(occupant, dayReference(festivalDays ?? [], stall.occupants)) }})
              </span>
              <!-- The type is its OWN line, not an alternative to the day note: a
                   pitch has a type whether or not its title carries a day, and
                   pairing them as if/else silently dropped it from every titled
                   pitch. Lighter than the name above it, so the two read in
                   order. -->
              <span
                v-if="stallCaption(occupant)"
                class="map-stall-caption map-stall-caption--type"
                >{{ stallCaption(occupant) }}</span
              >
            </span>
          </span>
          <!-- The rally mark. Solid rather than duotone so it is one flat colour -
               the stall's own label colour - instead of two tones of it. -->
          <span v-if="isStamped(stall)" class="map-stall-stamp" aria-hidden="true">
            <font-awesome-icon :icon="['fas', 'stamp']" />
          </span>
        </div>
      </div>
    </div>
  </div>
</template>
