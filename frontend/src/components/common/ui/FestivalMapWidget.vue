<script setup lang="ts">
/**
 * The Festival Map as a self-contained widget: the plan, the controls that drive
 * it, and the detail panel a tapped stall opens - all inside ONE positioned box.
 *
 * Everything it needs is in here and nothing escapes it, which is the whole point:
 * the same widget is rendered on our own map page (views/FestivalMapView) and,
 * with `embedded` set, alone in an <iframe> on somebody else's site
 * (views/FestivalMapEmbedView). Anything it leaned on from the page around it -
 * a heading, a legend below the plan, the site's own nav - would simply not be
 * there in the second case.
 *
 * A stall on the plan is a PITCH holding one or more OCCUPANTS, so a booth that
 * changes hands between days is one shape listing both. The DAY switcher picks
 * which of them the plan draws; the "stamp rally stalls only" toggle pushes the
 * rest into the background.
 */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import FestivalMapCanvas from '@/components/common/ui/FestivalMapCanvas.vue'
import { assetUrl } from '@/lib/assets'
import { formatEventRange, occupantMapLabel, occupantsOnDay, stallCaption } from '@/lib/festivalmap'
import type {
  EventTime,
  PublicFestivalMap,
  PublicFestivalStall,
  PublicStallOccupant,
} from '@/types/api'

const props = withDefaults(
  defineProps<{
    map: PublicFestivalMap
    /**
     * Rendered inside an <iframe> on another site. Two things follow from it, and
     * both are consequences of the same fact rather than separate options: the
     * widget fills the box the embedder gave it instead of shaping itself to the
     * page, and every link leaves for a new tab - a visitor who taps through must
     * not have our site replace the page they were reading.
     */
    embedded?: boolean
  }>(),
  { embedded: false },
)

/** The pitch whose details are open (null = none). */
const selected = ref<PublicFestivalStall | null>(null)
const rallyOnly = ref(false)
/** Index into the map's `times` - which festival day is being browsed (-1 = all). */
const dayIndex = ref(-1)
const canvasRef = ref<{ zoomBy: (factor: number) => void; reset: () => void } | null>(null)

const stalls = computed(() => props.map.stalls)

/** The festival's own datetime rows - the days the switcher offers. */
const days = computed(() => props.map.times)
/** The day being browsed, or null for "all". */
const selectedDay = computed<EventTime | null>(() => days.value[dayIndex.value] ?? null)

// A different map in the same widget (the id is a route param, so an in-app link
// from one festival to another reuses this) starts fresh rather than keeping the
// previous festival's open stall and day filter.
watch(
  () => props.map.id,
  () => {
    selected.value = null
    rallyOnly.value = false
    dayIndex.value = -1
  },
)

/** The occupants of a pitch that are standing there on the browsed day. */
function occupantsShown(stall: PublicFestivalStall): PublicStallOccupant[] {
  return occupantsOnDay(stall.occupants, selectedDay.value, days.value)
}

/** Pitches with at least one occupant carrying a stamp on the open rally. */
const rallyIds = computed(
  () =>
    new Set(stalls.value.filter((s) => s.occupants.some((o) => o.in_stamp_rally)).map((s) => s.id)),
)

/**
 * Everything the filters currently push into the background: a pitch nobody
 * occupies on the browsed day, and - when the rally toggle is on - a pitch whose
 * occupants that day carry no stamp.
 */
const dimmedIds = computed(() => {
  const dimmed = new Set<number>()
  for (const stall of stalls.value) {
    const shown = occupantsShown(stall)
    if (shown.length === 0) dimmed.add(stall.id)
    else if (rallyOnly.value && !shown.some((o) => o.in_stamp_rally)) dimmed.add(stall.id)
  }
  return dimmed
})

/** The occupants listed in the open pitch's panel, filtered to the browsed day. */
const selectedOccupants = computed(() => (selected.value ? occupantsShown(selected.value) : []))

/** An occupant's own hours, or the festival's when it keeps none. */
function hoursFor(occupant: PublicStallOccupant): EventTime[] {
  return occupant.times.length ? occupant.times : days.value
}

/**
 * The shape the plan turned out to be, once its image loaded - 0 until then.
 *
 * The stylesheet needs it to size the WIDGET, not just the plan. The frame is
 * capped in height so a tall plan cannot push the page down, and it keeps the
 * plan's shape as it shrinks - but the toolbar above it and the stage around it
 * did not, so a square or portrait plan opened off to one side of a bar far wider
 * than itself, with dead page beside it. Handing the shape up lets the widget take
 * the same width, which is what makes a fitted map open with nothing around it.
 */
const frameAspect = ref(0)
const frameStyle = computed(() =>
  frameAspect.value > 0 ? { '--map-aspect': String(frameAspect.value) } : {},
)

/**
 * Where the details panel has been dragged to, in pixels from the plan's top-left.
 * null means "wherever the stylesheet puts it" - a column beside the plan on a wide
 * screen, a band across the bottom on a phone.
 *
 * It is worth being movable: the panel necessarily covers part of the plan, and the
 * stall a visitor is reading about is as likely to be underneath it as not.
 */
const modalPos = ref<{ x: number; y: number } | null>(null)
/**
 * The STAGE, not the whole widget: the panel is positioned inside the stage, so a
 * drag measured against the widget would be offset by the toolbar sitting above it
 * - which showed up as the panel jumping down the moment it was grabbed.
 */
const stageRef = ref<HTMLElement | null>(null)
const modalRef = ref<HTMLElement | null>(null)

const modalStyle = computed(() =>
  modalPos.value
    ? { inset: 'auto auto auto 0', left: `${modalPos.value.x}px`, top: `${modalPos.value.y}px` }
    : {},
)

let dragFrom = { pointer: { x: 0, y: 0 }, panel: { x: 0, y: 0 } }

/** Keeps the panel within the map, so it can never be dragged out of reach. */
function clampToMap(x: number, y: number): { x: number; y: number } {
  const box = stageRef.value?.getBoundingClientRect()
  const panel = modalRef.value?.getBoundingClientRect()
  if (!box || !panel) return { x, y }
  return {
    x: Math.min(Math.max(0, x), Math.max(0, box.width - panel.width)),
    y: Math.min(Math.max(0, y), Math.max(0, box.height - panel.height)),
  }
}

function onModalDragMove(e: PointerEvent): void {
  modalPos.value = clampToMap(
    dragFrom.panel.x + (e.clientX - dragFrom.pointer.x),
    dragFrom.panel.y + (e.clientY - dragFrom.pointer.y),
  )
}

function onModalDragEnd(): void {
  window.removeEventListener('pointermove', onModalDragMove)
  window.removeEventListener('pointerup', onModalDragEnd)
  window.removeEventListener('pointercancel', onModalDragEnd)
}

/**
 * Starts a drag from the panel's grip. Listeners go on the WINDOW so a pointer that
 * outruns the panel - easy to do when flicking it aside - keeps dragging rather
 * than dropping it mid-move.
 */
function onModalDragStart(e: PointerEvent): void {
  // The grip holds the close button. Starting a drag from it would swallow the
  // button's click - preventDefault below suppresses it - so the panel could only
  // be nudged, never closed.
  if ((e.target as HTMLElement).closest('button')) return

  const box = stageRef.value?.getBoundingClientRect()
  const panel = modalRef.value?.getBoundingClientRect()
  if (!box || !panel) return
  e.preventDefault() // never let the map underneath start panning too
  dragFrom = {
    pointer: { x: e.clientX, y: e.clientY },
    panel: { x: panel.left - box.left, y: panel.top - box.top },
  }
  modalPos.value = { x: dragFrom.panel.x, y: dragFrom.panel.y }
  window.addEventListener('pointermove', onModalDragMove)
  window.addEventListener('pointerup', onModalDragEnd)
  window.addEventListener('pointercancel', onModalDragEnd)
}

/** A newly opened stall starts where the stylesheet puts it, not where the last drag left it. */
watch(selected, () => {
  modalPos.value = null
})

onBeforeUnmount(onModalDragEnd)

/**
 * The usage hint: a wash over the whole plan, the way a web map explains itself,
 * rather than a line in the corner - which is exactly the place an eye skips.
 *
 * It never intercepts anything (see .map-hint's pointer-events), so the first
 * drag both clears this AND moves the map instead of being spent on the overlay.
 * It goes on any real interaction - the visitor has evidently got it - or after
 * seven seconds if they never touch the map at all.
 */
const hintDismissed = ref(false)
let hintTimer: ReturnType<typeof setTimeout> | null = null

function dismissHint(): void {
  hintDismissed.value = true
  if (hintTimer !== null) {
    clearTimeout(hintTimer)
    hintTimer = null
  }
}

onMounted(() => {
  hintTimer = setTimeout(dismissHint, 7000)
})

onBeforeUnmount(() => {
  if (hintTimer !== null) clearTimeout(hintTimer)
})

/**
 * Where a link inside the panel opens. Embedded, everything leaves for a new tab:
 * the widget is a guest on someone else's page, and following a raffle link must
 * not replace the page a visitor was reading with ours inside a frame.
 */
const linkTarget = computed(() => (props.embedded ? '_blank' : undefined))
</script>

<template>
  <div class="map-embed" :class="{ 'is-framed': embedded }" :style="frameStyle">
    <!-- Any interaction at all clears the hint, the toolbar's own buttons
         included: zooming from the bar is using the map. Capture, so it counts
         even though the controls below stop the event for their own purposes. -->
    <div
      class="map-toolbar"
      @pointerdown.capture="dismissHint"
      @wheel.capture="dismissHint"
      @keydown.capture="dismissHint"
    >
      <button class="btn-neutral btn-sm" aria-label="Zoom in" @click="canvasRef?.zoomBy(1.4)">
        <font-awesome-icon :icon="['fas', 'magnifying-glass-plus']" />
      </button>
      <button class="btn-neutral btn-sm" aria-label="Zoom out" @click="canvasRef?.zoomBy(1 / 1.4)">
        <font-awesome-icon :icon="['fas', 'magnifying-glass-minus']" />
      </button>
      <button class="btn-neutral btn-sm" @click="canvasRef?.reset()">
        <font-awesome-icon :icon="['fas', 'expand']" /> Fit
      </button>
      <!-- Day switcher: only worth offering when the festival runs across more
           than one, since some stalls change hands from one day to the next. -->
      <span v-if="days.length > 1" class="map-day-picker">
        <button
          class="btn-neutral btn-sm"
          :class="{ 'is-active': dayIndex === -1 }"
          @click="dayIndex = -1"
        >
          All days
        </button>
        <button
          v-for="(t, i) in days"
          :key="i"
          class="btn-neutral btn-sm"
          :class="{ 'is-active': dayIndex === i }"
          @click="dayIndex = i"
        >
          {{ t.label || `Day ${i + 1}` }}
        </button>
      </span>
      <label v-if="rallyIds.size" class="checkbox-inline">
        <input v-model="rallyOnly" type="checkbox" />
        Highlight {{ map.stamp_rally_title || 'stamp rally' }} stalls only
      </label>
    </div>

    <!-- The stage: the plan and everything positioned against it. The toolbar is
         a sibling ABOVE this, so anchoring the panel and the hint to the widget
         would measure from the top of the bar instead of the top of the plan. -->
    <div
      ref="stageRef"
      class="map-embed-stage"
      @pointerdown.capture="dismissHint"
      @wheel.capture="dismissHint"
      @keydown.capture="dismissHint"
    >
      <FestivalMapCanvas
        ref="canvasRef"
        :map-image="map.map_image"
        :stalls="stalls"
        :selected-id="selected?.id ?? null"
        :dimmed-ids="dimmedIds"
        :rally-ids="rallyIds"
        :day="selectedDay"
        :festival-days="days"
        :fill="embedded"
        @select="selected = $event"
        @frame-aspect="frameAspect = $event"
      />

      <!-- Covers the plan and dims it for a moment, then clears itself. Inert:
           it is told about, never touched. -->
      <div class="map-hint" :class="{ 'is-dismissed': hintDismissed }" aria-hidden="true">
        <p class="map-hint-text">
          <font-awesome-icon :icon="['fad', 'hand-pointer']" class="map-hint-icon" />
          <span>
            Drag to move around the map - scroll or pinch to zoom - tap a stall for its details.
          </span>
        </p>
      </div>

      <!-- The tapped pitch, over the map rather than below it: the details belong
           to the thing that was tapped, and there may be no page under this to
           put them on. -->
      <div
        v-if="selected"
        ref="modalRef"
        class="map-stall-modal"
        :style="modalStyle"
        role="dialog"
        aria-modal="false"
        aria-label="Details for the selected stall"
      >
        <!-- The grip. Dragging anywhere on the panel would fight text selection
             and the links inside it, so only this bar moves it. -->
        <div class="map-stall-modal-grip" @pointerdown="onModalDragStart">
          <font-awesome-icon :icon="['fad', 'bars']" class="map-stall-modal-grip-icon" />
          <button class="map-stall-modal-close" aria-label="Close details" @click="selected = null">
            <font-awesome-icon :icon="['fas', 'circle-xmark']" />
          </button>
        </div>
        <div class="map-stall-modal-body">
          <p v-if="!selectedOccupants.length" class="text-muted text-sm">
            Nobody is at this stall on the day you're viewing.
          </p>
          <div
            v-for="occupant in selectedOccupants"
            :key="occupant.id"
            class="map-stall-occupant-panel"
          >
            <div class="map-stall-panel-head">
              <div>
                <!-- The affiliate leads, because that is who the visitor is
                     looking for; a stall title is optional and only ADDS to it. -->
                <h3>
                  {{ occupantMapLabel(occupant) }}
                  <template v-if="occupant.title.trim()"> - {{ occupant.title.trim() }} </template>
                </h3>
                <!-- On its own row rather than trailing the name: whether the
                     stall is open right now is the first thing a visitor wants,
                     and inline it was easy to miss at the end of a long name that
                     had already wrapped. -->
                <p class="map-stall-status">
                  <span
                    class="status-badge"
                    :class="occupant.is_open ? 'status-badge-open' : 'status-badge-closed'"
                  >
                    {{ occupant.is_open ? 'open now' : 'closed' }}
                  </span>
                </p>
                <!-- The type alone. The affiliate used to be repeated here, one
                     line under the name it now heads. -->
                <p v-if="stallCaption(occupant)" class="map-stall-type">
                  <font-awesome-icon :icon="['fad', 'store']" />
                  <span>{{ stallCaption(occupant) }}</span>
                </p>
              </div>
              <img
                v-if="occupant.affiliate?.logo"
                :src="assetUrl(occupant.affiliate.logo)"
                class="map-stall-logo"
                :alt="occupant.affiliate.name"
              />
            </div>

            <!-- Order matters here: what a visitor standing in front of the stall
                 needs first is whether it is open and when - so the hours come
                 straight after the type, ahead of the prose. -->
            <ul v-if="hoursFor(occupant).length" class="map-stall-hours">
              <li v-for="(t, i) in hoursFor(occupant)" :key="i">
                <font-awesome-icon :icon="['fad', 'clock']" />
                <!-- The label is the part a visitor scans for ("Day 1"), so it
                     carries the weight; the range beside it stays plain. -->
                <span>
                  <strong v-if="t.label.trim()">{{ t.label.trim() }}</strong
                  ><template v-if="t.label.trim()"> - </template
                  >{{ formatEventRange(t.start, t.end) }}
                </span>
              </li>
            </ul>

            <MarkdownText v-if="occupant.description" :source="occupant.description" />

            <div
              v-if="
                occupant.event_carrd ||
                occupant.affiliate?.carrd_link ||
                occupant.affiliate?.discord_link
              "
              class="map-stall-links"
            >
              <!-- All three sit together at the left, under the text they belong
                   to. The booth page leads them: it is about THIS stall on THIS
                   day, where the two beside it belong to the business behind it. -->
              <a
                v-if="occupant.event_carrd"
                class="btn-view btn-sm"
                :href="occupant.event_carrd"
                target="_blank"
                rel="noopener noreferrer"
              >
                <font-awesome-icon :icon="['fad', 'store']" /> Booth Page
              </a>
              <a
                v-if="occupant.affiliate?.carrd_link"
                class="btn-view btn-sm"
                :href="occupant.affiliate.carrd_link"
                target="_blank"
                rel="noopener noreferrer"
              >
                <font-awesome-icon :icon="['fas', 'link']" /> Website
              </a>
              <a
                v-if="occupant.affiliate?.discord_link"
                class="btn-neutral btn-sm"
                :href="occupant.affiliate.discord_link"
                target="_blank"
                rel="noopener noreferrer"
              >
                <font-awesome-icon :icon="['fab', 'discord']" /> Discord
              </a>
            </div>

            <!-- Raffle running at this stall right now (the server leaves off any
                 that is closed or outside its dates). -->
            <div v-if="occupant.raffle" class="map-stall-rally">
              <img
                v-if="occupant.raffle.prize_image"
                :src="assetUrl(occupant.raffle.prize_image)"
                alt=""
              />
              <div>
                <strong>{{ occupant.raffle.title }}</strong>
                <p class="text-sm text-muted">
                  A raffle is running at this stall.
                  <!-- A RouterLink rather than a button, so it carries a real href:
                       embedded, `target` makes it open outside the frame, and
                       vue-router leaves a _blank link to the browser. On our own
                       page it navigates in place as before. -->
                  <RouterLink
                    class="link-btn"
                    :to="{ name: 'raffle-detail', params: { id: String(occupant.raffle.id) } }"
                    :target="linkTarget"
                  >
                    View the raffle
                  </RouterLink>
                </p>
              </div>
            </div>

            <!-- Stamp rally badge: this occupant carries a stamp on the open rally. -->
            <div v-if="occupant.in_stamp_rally" class="map-stall-rally">
              <img v-if="occupant.stamp_image" :src="assetUrl(occupant.stamp_image)" alt="" />
              <div>
                <strong>Part of {{ occupant.rally_title }}</strong>
                <p class="text-sm text-muted">
                  Visit this stall to collect its stamp.
                  <RouterLink
                    v-if="map.stamp_rally_signup && map.stamp_rally_id"
                    class="link-btn"
                    :to="{
                      name: 'stamp-rally-signup',
                      params: { id: String(map.stamp_rally_id) },
                    }"
                    :target="linkTarget"
                  >
                    Get a stamp card
                  </RouterLink>
                </p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
