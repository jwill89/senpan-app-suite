<script setup lang="ts">
/**
 * Public Festival Map page: one published map's floor plan, with the festival's
 * own description and dates around it.
 *
 * The map itself is FestivalMapWidget, which is self-contained on purpose - the
 * same widget is what an <iframe> on someone else's site renders (see
 * views/FestivalMapEmbedView). This page is only the chrome around it: the back
 * link, the title, the description, and the days the festival runs.
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import MarkdownText from '@/components/common/MarkdownText.vue'
import FestivalMapWidget from '@/components/common/ui/FestivalMapWidget.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'
import { formatEventTime } from '@/lib/festivalmap'

// The route param is the map's shortcode OR its numeric id - the server resolves
// either, so it is passed through as typed rather than parsed here.
const props = defineProps<{ id: string }>()

const router = useRouter()
const store = useFestivalMapsStore()

/** false once a load has come back empty - an unpublished or deleted map. */
const found = ref(true)

async function load(): Promise<void> {
  found.value = await store.loadPublicMap(props.id)
}

onMounted(load)
// The map id is a route param, so an in-app link from one map to another reuses
// this component - reload rather than leaving the previous festival on screen.
watch(() => props.id, load)

/** The festival's own datetime rows. */
const days = computed(() => store.publicMap?.times ?? [])

function goBack(): void {
  void router.push({ name: 'festival-maps' })
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

        <FestivalMapWidget :map="store.publicMap" />
      </template>
    </div>
  </div>
</template>
