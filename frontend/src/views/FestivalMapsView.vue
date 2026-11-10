<script setup lang="ts">
/**
 * Public Festival Map list: the maps an admin has published.
 *
 * The server decides what appears here - an in-progress or closed map is never
 * listed - so this view just renders what it is given. Picking one opens its
 * festival dashboard - the map with the festival's stamp rally, garapon and
 * raffles around it - which is directly linkable so staff can post it straight
 * into Discord.
 */
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { assetUrl } from '@/lib/assets'
import { formatEventTime } from '@/lib/festivalmap'
import type { PublicFestivalMapSummary } from '@/api/client'

const router = useRouter()
const store = useFestivalMapsStore()

onMounted(() => store.loadPublicMaps())

function openMap(m: PublicFestivalMapSummary): void {
  void router.push({ name: 'festival-dashboard', params: { id: store.mapPath(m) } })
}

function goHome(): void {
  void router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <button type="button" class="btn-neutral btn-sm" @click="goHome">
        <font-awesome-icon :icon="['fas', 'arrow-left']" /> Home
      </button>
      <h2><font-awesome-icon :icon="['fad', 'map-location-dot']" /> Festival Maps</h2>
      <span></span>
    </div>

    <div class="tab-body content-container">
      <LoadingSpinner v-if="store.publicLoading" block label="Loading festival maps..." />

      <div v-else-if="store.publicMaps.length" class="card-grid card-grid--center">
        <div
          v-for="m in store.publicMaps"
          :key="m.id"
          class="media-card"
          role="button"
          tabindex="0"
          @click="openMap(m)"
          @keydown.enter="openMap(m)"
          @keydown.space.prevent="openMap(m)"
        >
          <img
            v-if="m.mapImage"
            :src="assetUrl(m.mapImage)"
            class="media-card-image"
            alt="Festival map"
          />
          <div class="media-card-body">
            <h3>
              {{ m.title }}
              <span v-if="m.isActive" class="status-badge status-badge-open">happening now</span>
            </h3>
            <ul v-if="m.times.length" class="map-times">
              <li v-for="(t, i) in m.times" :key="i">
                <font-awesome-icon :icon="['fad', 'calendar-days']" /> {{ formatEventTime(t) }}
              </li>
            </ul>
            <p class="text-sm text-muted">{{ m.stallCount }} stall(s) to explore</p>
          </div>
        </div>
      </div>

      <EmptyState
        v-else
        :icon="['fad', 'map-location-dot']"
        text="No festival maps are published right now."
        hint="Check back when the next festival goes live."
      />
    </div>
  </div>
</template>
