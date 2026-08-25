<script setup lang="ts">
/**
 * Public interactive Festival Map: one published map's floor plan, pannable and
 * zoomable, with every stall clickable for its details - what it offers, who runs
 * it (with a link to their site), when it's open, and whether it carries a stamp
 * on the rally that's currently running.
 *
 * A stall on the plan is a PITCH holding one or more OCCUPANTS, so a booth that
 * changes hands between days is one shape listing both. The DAY switcher picks
 * which of them the plan leads with and dims the pitches nobody occupies that day;
 * the "stamp rally stalls only" toggle dims the rest the same way. Both dim rather
 * than remove, so the plan keeps its shape and a visitor can still tell where
 * they are.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import FestivalMapCanvas from '@/components/common/ui/FestivalMapCanvas.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { assetUrl } from '@/lib/assets'
import { STALL_TYPES, formatEventTime, occupantsOnDay, stallCaption } from '@/lib/festivalmap'
import type { EventTime, PublicFestivalStall, PublicStallOccupant } from '@/types/api'

// The route param is the map's shortcode OR its numeric id - the server resolves
// either, so it is passed through as typed rather than parsed here.
const props = defineProps<{ id: string }>()

const router = useRouter()
const store = useFestivalMapsStore()

/** false once a load has come back empty - an unpublished or deleted map. */
const found = ref(true)
/** The pitch whose details are open (null = none). */
const selected = ref<PublicFestivalStall | null>(null)
const rallyOnly = ref(false)
/** Index into the map's `times` - which festival day is being browsed (-1 = all). */
const dayIndex = ref(-1)
const canvasRef = ref<{ zoomBy: (factor: number) => void; reset: () => void } | null>(null)

async function load(): Promise<void> {
  selected.value = null
  rallyOnly.value = false
  dayIndex.value = -1
  found.value = await store.loadPublicMap(props.id)
}

onMounted(load)
// The map id is a route param, so an in-app link from one map to another reuses
// this component - reload rather than leaving the previous festival on screen.
watch(() => props.id, load)

const stalls = computed(() => store.publicMap?.stalls ?? [])

/** The festival's own datetime rows - the days the switcher offers. */
const days = computed(() => store.publicMap?.times ?? [])
/** The day being browsed, or null for "all". */
const selectedDay = computed<EventTime | null>(() => days.value[dayIndex.value] ?? null)

/** The occupants of a pitch that are standing there on the browsed day. */
function occupantsShown(stall: PublicFestivalStall): PublicStallOccupant[] {
  return occupantsOnDay(stall.occupants, selectedDay.value)
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

/** The stall types actually on this map, so the legend lists only what's there. */
const legendTypes = computed(() => {
  const present = new Set(stalls.value.flatMap((s) => s.occupants.map((o) => o.stall_type)))
  return STALL_TYPES.filter((t) => present.has(t.value))
})

/** The occupants listed in the open pitch's panel, filtered to the browsed day. */
const selectedOccupants = computed(() => (selected.value ? occupantsShown(selected.value) : []))

/** An occupant's own hours, or the festival's when it keeps none. */
function hoursFor(occupant: PublicStallOccupant): EventTime[] {
  return occupant.times.length ? occupant.times : days.value
}

function goBack(): void {
  void router.push({ name: 'festival-maps' })
}

/** Open a raffle running at one of the map's stalls. */
function goRaffle(id: number): void {
  void router.push({ name: 'raffle-detail', params: { id: String(id) } })
}

function goSignup(): void {
  const id = store.publicMap?.stamp_rally_id
  if (id) void router.push({ name: 'stamp-rally-signup', params: { id: String(id) } })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button class="btn-neutral btn-sm" @click="goBack">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Festival Maps
      </button>
      <h2>
        <font-awesome-icon :icon="['fad', 'map-location-dot']" />
        {{ store.publicMap?.title ?? 'Festival Map' }}
      </h2>
      <span></span>
    </div>

    <div class="tab-body content-container">
      <LoadingSpinner v-if="store.publicLoading" block label="Loading the festival map..." />

      <EmptyState
        v-else-if="!found || !store.publicMap"
        :icon="['fad', 'map-location-dot']"
        text="That festival map isn't available."
        hint="It may not be published yet, or the festival is over."
      />

      <template v-else>
        <MarkdownText v-if="store.publicMap.description" :source="store.publicMap.description" />

        <ul v-if="days.length" class="map-times mb-16">
          <li v-for="(t, i) in days" :key="i">
            <font-awesome-icon :icon="['fad', 'calendar-days']" /> {{ formatEventTime(t) }}
          </li>
        </ul>

        <div class="map-toolbar">
          <button class="btn-neutral btn-sm" aria-label="Zoom in" @click="canvasRef?.zoomBy(1.4)">
            <font-awesome-icon :icon="['fas', 'magnifying-glass-plus']" />
          </button>
          <button
            class="btn-neutral btn-sm"
            aria-label="Zoom out"
            @click="canvasRef?.zoomBy(1 / 1.4)"
          >
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
            Highlight
            {{ store.publicMap.stamp_rally_title || 'stamp rally' }} stalls only
          </label>
        </div>

        <FestivalMapCanvas
          ref="canvasRef"
          :map-image="store.publicMap.map_image"
          :stalls="stalls"
          :selected-id="selected?.id ?? null"
          :dimmed-ids="dimmedIds"
          :rally-ids="rallyIds"
          :day="selectedDay"
          @select="selected = $event"
        />

        <p class="placement-hint text-muted text-xs">
          Drag to move around the map - scroll or pinch to zoom - tap a stall for its details.
        </p>

        <ul v-if="legendTypes.length" class="map-legend">
          <li v-for="t in legendTypes" :key="t.value" class="map-legend-item">
            <span
              class="map-legend-swatch"
              :class="{ 'map-legend-swatch--round': t.shape === 'circle' }"
              :style="{ background: t.color }"
            ></span>
            {{ t.label }}
          </li>
          <li v-if="rallyIds.size" class="map-legend-item">
            <span class="map-legend-swatch map-legend-swatch--round is-rally"></span>
            Part of {{ store.publicMap.stamp_rally_title }}
          </li>
        </ul>

        <!-- Selected pitch: everyone standing in it on the browsed day -->
        <div v-if="selected" class="map-stall-panel">
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
                <h3>
                  {{ occupant.title }}
                  <span
                    class="status-badge"
                    :class="occupant.is_open ? 'status-badge-open' : 'status-badge-closed'"
                  >
                    {{ occupant.is_open ? 'open now' : 'closed' }}
                  </span>
                </h3>
                <p class="text-sm text-muted">
                  <template v-if="stallCaption(occupant)">
                    {{ stallCaption(occupant) }} -
                  </template>
                  {{ occupant.affiliate?.name || 'Senpan Tea House' }}
                  <template v-if="occupant.affiliate?.subtitle">
                    ({{ occupant.affiliate.subtitle }})
                  </template>
                </p>
              </div>
              <img
                v-if="occupant.affiliate?.logo"
                :src="assetUrl(occupant.affiliate.logo)"
                class="map-stall-logo"
                :alt="occupant.affiliate.name"
              />
            </div>

            <MarkdownText v-if="occupant.description" :source="occupant.description" />

            <p v-if="occupant.affiliate?.owners?.length" class="text-sm text-muted">
              <font-awesome-icon :icon="['fad', 'user']" />
              {{ occupant.affiliate.owners.join(', ') }}
            </p>

            <ul v-if="hoursFor(occupant).length" class="map-stall-hours">
              <li v-for="(t, i) in hoursFor(occupant)" :key="i">
                <font-awesome-icon :icon="['fad', 'clock']" /> {{ formatEventTime(t) }}
              </li>
            </ul>

            <div
              v-if="occupant.affiliate?.carrd_link || occupant.affiliate?.discord_link"
              class="map-stall-links"
            >
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
                  <button class="link-btn" @click="goRaffle(occupant.raffle.id)">
                    View the raffle
                  </button>
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
                  <button
                    v-if="store.publicMap.stamp_rally_signup"
                    class="link-btn"
                    @click="goSignup"
                  >
                    Get a stamp card
                  </button>
                </p>
              </div>
            </div>
          </div>
        </div>
        <p v-else class="text-muted text-sm mt-10">Tap a stall on the map to see what it offers.</p>
      </template>
    </div>
  </div>
</template>
