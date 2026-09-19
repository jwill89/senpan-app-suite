<script setup lang="ts">
/**
 * Admin Festival Map manager (Festival -> Festival Map). Screens:
 *
 *   - list:   searchable grid of map cards (base image + title + stall count +
 *             publish status), with the closed maps in a table beneath.
 *   - detail: the selected map - a read-only preview of the plan with every
 *             stall in place, its dates, and the publish controls.
 *   - form:   the create/edit form (FestivalMapFormTab), a Back sub-page.
 *
 * Only a *published* map is visible to the public, so publishing is a deliberate
 * step on the detail screen rather than a field buried in the form.
 */
import { computed, ref } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import AdminPanel from '@/components/common/ui/AdminPanel.vue'
import ManagerView from '@/components/common/ui/ManagerView.vue'
import SubPageHeader from '@/components/common/ui/SubPageHeader.vue'
import SearchInput from '@/components/common/ui/SearchInput.vue'
import DataTable, {
  type DataColumn,
  type DataTableView,
} from '@/components/common/ui/DataTable.vue'
import DataTableToolbar from '@/components/common/ui/DataTableToolbar.vue'
import PaginationBar from '@/components/common/ui/PaginationBar.vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import FormField from '@/components/common/ui/FormField.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import FestivalMapFormTab from './FestivalMapFormTab.vue'
import FestivalMapCanvas from '@/components/common/ui/FestivalMapCanvas.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import {
  MAP_EMBED_HEIGHT,
  formatEventTime,
  mapEmbedPath,
  mapEmbedSnippet,
  occupantListLabel,
  stallCaption,
  toPublicStalls,
} from '@/lib/festivalmap'
import type { FestivalMap, FestivalMapStatus } from '@/types/api'

const store = useFestivalMapsStore()

type Screen = 'list' | 'detail' | 'form'
const screen = ref<Screen>('list')

/**
 * The badge's wording. `published` and `closed` already read as English; only
 * the stored `in_progress` needs its underscore turned back into a space.
 */
function statusLabel(status: string): string {
  return status === 'in_progress' ? 'in progress' : status
}

/** The badge modifier for a status (`in_progress` -> `status-badge-in-progress`). */
function statusClass(status: string): string {
  return `status-badge-${status.replace('_', '-')}`
}

// -- List: in-progress + published cards, closed table ------------------------
const search = ref('')
/** Everything still being worked on or live - the maps an admin acts on. */
const activeMaps = computed(() => store.maps.filter((m) => m.status !== 'closed'))
const filteredActive = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return activeMaps.value
  return activeMaps.value.filter((m) => m.title.toLowerCase().includes(q))
})

const closedColumns: DataColumn[] = [
  { key: 'title', label: 'Title', sortable: true },
  { key: 'stall_count', label: 'Stalls', align: 'right', sortable: true },
  { key: 'created_at', label: 'Created', sortable: true },
  { key: 'actions', label: '', align: 'right' },
]
const closedSearch = ref('')
const closedPage = ref(1)
const closedView = ref<DataTableView>({ total: 0, totalPages: 1, facets: {} })
const closedMatches = (m: FestivalMap, q: string): boolean => m.title.toLowerCase().includes(q)

// -- Detail -------------------------------------------------------------------
const selectedStalls = computed(() => store.selectedMap?.stalls ?? [])

/**
 * The public link for a published map, so staff can paste it into Discord. It
 * uses the map's shortcode when it has one (`store.mapPath`), which is the whole
 * point of setting one.
 */
const publicUrl = computed(() =>
  store.selectedMap
    ? `${window.location.origin}/festival-maps/${store.mapPath(store.selectedMap)}`
    : '',
)

/**
 * The map with none of our page around it, for an <iframe> somewhere else - and
 * the tag to paste to get it there.
 *
 * The same map, not a copy of it: the embed reads the same published map through
 * the same public endpoint, so a stall added here appears on every site the
 * snippet was pasted into without anyone repasting anything.
 */
const embedUrl = computed(() =>
  store.selectedMap
    ? `${window.location.origin}${mapEmbedPath(store.mapPath(store.selectedMap))}`
    : '',
)
const embedSnippet = computed(() =>
  store.selectedMap ? mapEmbedSnippet(embedUrl.value, store.selectedMap.title) : '',
)
/** Whether the embed panel is open. Closed by default - it is the rarer errand. */
const showEmbed = ref(false)

async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    /* clipboard blocked - both values are shown beside their buttons either way */
  }
}

// -- Navigation ---------------------------------------------------------------
function openNew(): void {
  store.newMapForm()
  screen.value = 'form'
}
function openMap(m: FestivalMap): void {
  store.viewMap(m)
  screen.value = 'detail'
}
function backToList(): void {
  store.selectedMap = null
  screen.value = 'list'
}
/**
 * Whether the detail fetch has actually landed. Both entry points below seed a
 * form that saves as a FULL REPLACE, so acting on a list row (which carries no
 * stalls) would delete every pitch on the map. See hasMapDetail.
 */
/** The selected map's stalls in the shape the public canvas draws. */
const previewStalls = computed(() => toPublicStalls(selectedStalls.value))

const detailReady = computed(() => !store.detailLoading && store.hasMapDetail(store.selectedMap))

function editSelected(): void {
  if (!store.selectedMap) return
  if (!store.editMapForm(store.selectedMap)) return
  screen.value = 'form'
}
/** Opens the create form pre-filled from this map (see copyMapForm). */
function duplicateSelected(): void {
  if (!store.selectedMap) return
  if (!store.copyMapForm(store.selectedMap)) return
  screen.value = 'form'
}
/**
 * Row action on the closed table: lay out next year's festival from this one
 * without opening it first - the same shortcut the closed raffle table offers.
 *
 * The row comes from the LIST, which carries no stalls (they load with the
 * detail), so a copy seeded straight off it would have an empty plan - which is
 * exactly what a duplicate exists to avoid. Fetch the detail first and copy that.
 */
async function copyMap(m: FestivalMap): Promise<void> {
  await store.loadMapDetail(m.id)
  if (!store.selectedMap) return
  // The fetch can still have failed, which leaves the stall-less list row here.
  if (!store.copyMapForm(store.selectedMap)) return
  screen.value = 'form'
}
/**
 * Where the form leaves you.
 *
 * Saving used to drop the admin back at the map picker, several clicks from the
 * thing they had just been working on - and saving a map is rarely "done with this
 * map": publishing it, or reading how it turned out, is the usual next step. A save
 * leaves the map selected (see saveMap), so it lands on that map's own page.
 *
 * Cancelling goes wherever it came FROM: back to the map when editing one, back to
 * the list when the form was a brand-new map with nothing behind it.
 */
function onFormDone(): void {
  screen.value = store.selectedMap ? 'detail' : 'list'
}
async function deleteSelected(): Promise<void> {
  const m = store.selectedMap
  if (!m) return
  await store.deleteMap(m.id)
  if (!store.selectedMap) screen.value = 'list'
}
function setStatus(status: FestivalMapStatus): void {
  if (store.selectedMap) void store.setStatus(store.selectedMap.id, status)
}
</script>

<template>
  <div class="tab-body">
    <!-- -- Form ---------------------------------------------------------------- -->
    <FestivalMapFormTab v-if="screen === 'form'" @saved="onFormDone" @cancel="onFormDone" />

    <!-- -- Detail -------------------------------------------------------------- -->
    <AdminPanel v-else-if="screen === 'detail' && store.selectedMap">
      <SubPageHeader @back="backToList">
        {{ store.selectedMap.title }}
        <span :class="['status-badge', statusClass(store.selectedMap.status)]">
          {{ statusLabel(store.selectedMap.status) }}
        </span>
      </SubPageHeader>

      <div class="flex-toolbar flex-end mb-16">
        <button
          class="btn-confirm btn-sm"
          :disabled="!detailReady"
          :title="detailReady ? undefined : 'Still loading this map'"
          @click="editSelected"
        >
          <font-awesome-icon :icon="['fas', 'pen-to-square']" /> Edit
        </button>
        <button
          class="btn-neutral btn-sm"
          :disabled="!detailReady"
          :title="detailReady ? undefined : 'Still loading this map'"
          @click="duplicateSelected"
        >
          <font-awesome-icon :icon="['fas', 'copy']" /> Duplicate
        </button>
        <button
          v-if="store.selectedMap.status !== 'published'"
          class="btn-view btn-sm"
          @click="setStatus('published')"
        >
          <font-awesome-icon :icon="['fad', 'eye']" /> Publish
        </button>
        <button
          v-if="store.selectedMap.status === 'published'"
          class="btn-neutral btn-sm"
          @click="setStatus('in_progress')"
        >
          <font-awesome-icon :icon="['fad', 'eye-slash']" /> Unpublish
        </button>
        <button
          v-if="store.selectedMap.status !== 'closed'"
          class="btn-neutral btn-sm"
          @click="setStatus('closed')"
        >
          <font-awesome-icon :icon="['fad', 'box-archive']" /> Close
        </button>
        <button class="btn-danger btn-sm" @click="deleteSelected">
          <font-awesome-icon :icon="['fas', 'trash']" /> Delete
        </button>
      </div>

      <LoadingSpinner v-if="store.detailLoading" block label="Loading map..." />
      <template v-else>
        <!-- Published only: both of these are the PUBLIC map, and a map nobody
             can see yet has no link worth handing out and no embed worth
             pasting. -->
        <div v-if="store.selectedMap.status === 'published'" class="mb-16">
          <p class="flex-toolbar">
            <button class="btn-neutral btn-sm" @click="copyText(publicUrl)">
              <font-awesome-icon :icon="['fad', 'link']" /> Copy public link
            </button>
            <code class="text-xs text-muted">{{ publicUrl }}</code>
          </p>

          <p class="flex-toolbar mt-8">
            <button
              class="btn-neutral btn-sm"
              :aria-expanded="showEmbed"
              aria-controls="map-embed-panel"
              @click="showEmbed = !showEmbed"
            >
              <font-awesome-icon :icon="['fas', 'code']" /> Embed code
            </button>
            <span class="text-xs text-muted">
              Puts the interactive map on a Carrd, or any page that takes HTML.
            </span>
          </p>

          <div v-if="showEmbed" id="map-embed-panel" class="mt-8">
            <FormField label="Embed on an external site (e.g. Carrd)" html-for="map-embed-snippet">
              <div class="map-embed-code-row">
                <input
                  id="map-embed-snippet"
                  readonly
                  :value="embedSnippet"
                  @focus="($event.target as HTMLInputElement).select()"
                />
                <button
                  class="btn-view btn-sm"
                  title="Copy embed code"
                  @click="copyText(embedSnippet)"
                >
                  <font-awesome-icon :icon="['fas', 'copy']" /> Copy
                </button>
                <a class="btn-neutral btn-sm" :href="embedUrl" target="_blank" rel="noopener">
                  <font-awesome-icon :icon="['fas', 'arrow-up-right-from-square']" /> Preview
                </a>
              </div>
              <template #help>
                Paste it into the other site's HTML. The map stays live - stalls, hours and links
                follow this map, so changes here reach every site it was pasted into without
                repasting. It is {{ MAP_EMBED_HEIGHT }}px tall; edit that number in the snippet to
                give the map more or less room.
              </template>
            </FormField>
          </div>
        </div>

        <MarkdownText
          v-if="store.selectedMap.description"
          :source="store.selectedMap.description"
        />

        <ul v-if="store.selectedMap.times.length" class="map-times mb-16">
          <li v-for="(t, i) in store.selectedMap.times" :key="i">
            <font-awesome-icon :icon="['fad', 'calendar-days']" /> {{ formatEventTime(t) }}
          </li>
        </ul>

        <!--
          The plan drawn by the SAME component the public map uses, against adapted
          data. This screen used to re-implement it in its own markup, which is how
          it drifted: it still labelled stalls by title long after the map had moved
          to labelling by affiliate, so every untitled stall showed up blank here.
          One drawing of a festival map, not two kept in step by hand.
        -->
        <FestivalMapCanvas
          :map-image="store.selectedMap.map_image"
          :stalls="previewStalls"
          :festival-days="store.selectedMap.times"
        />

        <h3 class="section-heading mt-16">
          <font-awesome-icon :icon="['fad', 'shop']" /> Stalls ({{ selectedStalls.length }})
        </h3>
        <EmptyState
          v-if="!selectedStalls.length"
          :icon="['fad', 'shop']"
          text="No stalls placed yet."
          hint="Edit the map to drop stalls onto the plan."
        />
        <ul v-else class="map-times">
          <li v-for="stall in selectedStalls" :key="stall.id">
            <template v-for="(occupant, oi) in stall.occupants" :key="occupant.id">
              <template v-if="oi">, then </template>
              <strong>{{ occupantListLabel(occupant) }}</strong>
              <template v-if="stallCaption(occupant)"> ({{ stallCaption(occupant) }})</template>
              <template v-if="occupant.times.length">
                - {{ occupant.times.map(formatEventTime).join(' / ') }}
              </template>
            </template>
          </li>
        </ul>
      </template>
    </AdminPanel>

    <!-- -- List ---------------------------------------------------------------- -->
    <template v-else>
      <ManagerView title="Festival Maps" :icon="['fad', 'map-location-dot']">
        <template #actions>
          <button class="btn-confirm btn-sm" @click="openNew">
            <font-awesome-icon :icon="['fas', 'plus']" /> New Map
          </button>
        </template>
        <template #toolbar>
          <SearchInput v-model="search" placeholder="Search maps..." />
        </template>

        <LoadingSpinner v-if="store.mapsLoading" block label="Loading festival maps..." />
        <div v-else-if="filteredActive.length" class="card-grid">
          <div
            v-for="m in filteredActive"
            :key="m.id"
            class="media-card"
            role="button"
            tabindex="0"
            @click="openMap(m)"
            @keydown.enter="openMap(m)"
            @keydown.space.prevent="openMap(m)"
          >
            <img
              v-if="m.map_image"
              :src="assetUrl(m.map_image)"
              class="media-card-image"
              alt="Festival map"
            />
            <div class="media-card-body">
              <h3>{{ m.title }}</h3>
              <p class="text-sm text-muted">
                <span :class="['status-badge', statusClass(m.status)]">
                  {{ statusLabel(m.status) }}
                </span>
                {{ m.stall_count || 0 }} stall(s)
              </p>
            </div>
          </div>
        </div>
        <EmptyState
          v-else
          :icon="['fad', 'map-location-dot']"
          text="No festival maps yet."
          hint="Create one, pick its floor plan, then drop the stalls onto it."
        />
      </ManagerView>

      <ManagerView
        v-if="store.closedMaps.length"
        title="Closed Maps"
        :icon="['fad', 'box-archive']"
        class="mt-16"
      >
        <template #toolbar>
          <DataTableToolbar :count="closedView.total" :total="store.closedMaps.length" noun="map">
            <template #search>
              <SearchInput
                v-model="closedSearch"
                placeholder="Search closed maps..."
                aria-label="Search closed maps"
              />
            </template>
          </DataTableToolbar>
        </template>
        <DataTable
          :columns="closedColumns"
          :rows="store.closedMaps"
          row-key="id"
          :filter="closedSearch"
          :filter-fn="closedMatches"
          @update:view="closedView = $event"
        >
          <template #cell-created_at="{ row }">
            <span class="text-sm text-muted">{{
              formatServerTimestamp((row as FestivalMap).created_at)
            }}</span>
          </template>
          <template #cell-actions="{ row }">
            <div class="row-actions">
              <button
                class="btn-view btn-sm"
                aria-label="View"
                title="View"
                @click="openMap(row as FestivalMap)"
              >
                <font-awesome-icon :icon="['fas', 'eye']" />
              </button>
              <button
                class="btn-view btn-sm"
                aria-label="Copy to new festival map"
                title="Copy to new festival map"
                @click="copyMap(row as FestivalMap)"
              >
                <font-awesome-icon :icon="['fas', 'copy']" />
              </button>
              <button
                class="btn-danger btn-sm"
                aria-label="Delete"
                title="Delete"
                @click="store.deleteMap((row as FestivalMap).id)"
              >
                <font-awesome-icon :icon="['fas', 'trash']" />
              </button>
            </div>
          </template>
          <template #empty><EmptyState text="No closed maps match your search." /></template>
        </DataTable>
        <PaginationBar
          v-if="closedView.totalPages > 1"
          class="mt-12"
          :page="closedPage"
          :total-pages="closedView.totalPages"
          @go="(p: number) => (closedPage = p)"
        />
      </ManagerView>
    </template>
  </div>
</template>
