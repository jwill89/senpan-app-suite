<script setup lang="ts">
/**
 * Public Stamp Rally card (reached via a per-participant card link, /stamp-card/:token).
 *
 * The card itself is StampCardPanel, which the festival dashboard renders too;
 * this page is the chrome around it. When the rally is filed under a published
 * festival, it also offers that festival's dashboard, handing the card over in
 * the navigation's history state (never the URL), so the dashboard opens with
 * the card already in it and keeps it on this device.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import StampCardPanel from '@/components/player/StampCardPanel.vue'
import { useStampRalliesStore } from '@/stores/stampRallies'
import { festivalConnectState } from '@/stores/festivalDashboard'

const props = defineProps<{ token: string }>()

const store = useStampRalliesStore()

const notFound = ref(false)

async function load(token: string): Promise<void> {
  notFound.value = false
  const ok = await store.loadByToken(token)
  if (!ok) notFound.value = true
}

onMounted(() => load(props.token))
watch(
  () => props.token,
  (t) => load(t),
)

/**
 * Where the festival dashboard link goes: the festival, with this link handed
 * over in history state rather than the URL. null when the rally belongs to no
 * published festival.
 */
const festivalTarget = computed(() => {
  const link = store.publicCard?.rally.festival_link
  return link
    ? {
        name: 'festival-dashboard',
        params: { id: link },
        state: festivalConnectState({ cardToken: props.token }),
      }
    : null
})
</script>

<template>
  <div v-if="store.publicCard">
    <div class="topbar">
      <span></span>
      <h2>{{ store.publicCard.rally.title }}</h2>
      <span></span>
    </div>

    <div class="tab-body content-container content-container--narrow">
      <RouterLink v-if="festivalTarget" :to="festivalTarget" class="link-card mb-16">
        <font-awesome-icon :icon="['fad', 'map-location-dot']" />
        <span>Open the festival dashboard</span>
      </RouterLink>
      <StampCardPanel :token="token" />
    </div>
  </div>

  <!-- Not found -->
  <div v-else-if="notFound" class="tab-body">
    <p class="page-message text-muted">
      <font-awesome-icon :icon="['fad', 'stamp']" /> This stamp card link is invalid or has been
      removed.
    </p>
  </div>

  <!-- Loading -->
  <div v-else-if="store.publicLoading" class="tab-body">
    <LoadingSpinner block label="Loading stamp card..." />
  </div>
</template>
