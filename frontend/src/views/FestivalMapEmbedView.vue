<script setup lang="ts">
/**
 * The Festival Map as an embed: the interactive widget, and nothing else, filling
 * the <iframe> it was dropped into.
 *
 * Reached at /embed/festival-maps/:id, the snippet the admin screen hands out. It
 * renders the SAME widget as the public page rather than a cut-down copy, so an
 * embed on a Carrd is the real map - pan, zoom, day filter, stall details and all
 * - not a picture of one. What it drops is everything that belongs to OUR page:
 * the header, the festival description, the site footer. The host page has its
 * own of each, and a second set inside a frame reads as a website in a box.
 *
 * It exposes nothing new. The map comes from the same public endpoint the public
 * page uses, which serves published maps only, so a map an admin has not
 * published has no embed either.
 *
 * `body.is-embedded` is what strips the page's own margins and scrollbars for the
 * duration; it is removed on the way out, since this is a normal route and a
 * visitor can navigate off it inside the frame.
 */
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import EmptyState from '@/components/common/ui/EmptyState.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import FestivalMapWidget from '@/components/common/ui/FestivalMapWidget.vue'
import { useFestivalMapsStore } from '@/stores/festivalMaps'

const props = defineProps<{ id: string }>()

const store = useFestivalMapsStore()

/** false once a load has come back empty - an unpublished or deleted map. */
const found = ref(true)

async function load(): Promise<void> {
  found.value = await store.loadPublicMap(props.id)
}

onMounted(() => {
  document.body.classList.add('is-embedded')
  void load()
})
onBeforeUnmount(() => document.body.classList.remove('is-embedded'))
watch(() => props.id, load)
</script>

<template>
  <div class="map-embed-page">
    <LoadingSpinner v-if="store.publicLoading" block label="Loading the festival map..." />

    <!-- Worth stating rather than showing an empty frame: an embed outlives the
         page it was pasted into, so the first person to see this is usually a
         visitor on someone else's site after the festival was closed. -->
    <EmptyState
      v-else-if="!found || !store.publicMap"
      :icon="['fad', 'map-location-dot']"
      text="That festival map isn't available."
      hint="It may not be published yet, or the festival is over."
    />

    <FestivalMapWidget v-else :map="store.publicMap" embedded />
  </div>
</template>
