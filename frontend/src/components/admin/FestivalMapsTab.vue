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
import MarkdownText from '@/components/common/MarkdownText.vue'
import FestivalMapFormTab from './FestivalMapFormTab.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { assetUrl } from '@/lib/assets'
import { formatServerTimestamp } from '@/lib/datetime'
import {
  formatEventTime,
  isCircle,
  stallCaption,
  stallColor,
  stallStyle,
  stallOperator,
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

async function copyPublicUrl(): Promise<void> {
  try {
    await navigator.clipboard.writeText(publicUrl.value)
  } catch {
    /* clipboard blocked - the link is shown beside the button either way */
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
function onFormDone(): void {
  screen.value = 'list'
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
        <p v-if="store.selectedMap.status === 'published'" class="flex-toolbar mb-16">
          <button class="btn-neutral btn-sm" @click="copyPublicUrl">
            <font-awesome-icon :icon="['fad', 'link']" /> Copy public link
          </button>
          <code class="text-xs text-muted">{{ publicUrl }}</code>
        </p>

        <MarkdownText
          v-if="store.selectedMap.description"
          :source="store.selectedMap.description"
        />

        <ul v-if="store.selectedMap.times.length" class="map-times mb-16">
          <li v-for="(t, i) in store.selectedMap.times" :key="i">
            <font-awesome-icon :icon="['fad', 'calendar-days']" /> {{ formatEventTime(t) }}
          </li>
        </ul>

        <!-- Read-only preview: the plan exactly as the public sees it. -->
        <div class="map-canvas">
          <img
            v-if="store.selectedMap.map_image"
            :src="assetUrl(store.selectedMap.map_image)"
            class="map-canvas-bg"
            alt="Festival map"
          />
          <div v-else class="map-canvas-bg map-canvas-empty">
            <font-awesome-icon :icon="['fad', 'image']" /> No base map image yet
          </div>
          <div
            v-for="stall in selectedStalls"
            :key="stall.id"
            class="map-stall"
            :class="{ 'map-stall--round': isCircle(stall.shape) }"
            :style="stallStyle(stall.placement, stall.shape, stallColor(stall))"
          >
            <span class="map-stall-label">
              <span
                v-for="occupant in stall.occupants"
                :key="occupant.id"
                class="map-stall-occupant"
              >
                <span>{{ occupant.title }}</span>
                <span v-if="stallCaption(occupant)" class="map-stall-caption">{{
                  stallCaption(occupant)
                }}</span>
              </span>
            </span>
          </div>
        </div>

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
              <strong>{{ occupant.title || 'Untitled stall' }}</strong>
              <template v-if="stallCaption(occupant)"> ({{ stallCaption(occupant) }})</template>
              - {{ stallOperator(occupant.affiliate_name) }}
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
