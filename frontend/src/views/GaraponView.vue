<script setup lang="ts">
/**
 * Public Garapon player view (reached via a per-player drawing link, /garapon/:token).
 *
 * The drum itself is GaraponPanel, which the festival dashboard renders too; this
 * page is the chrome around it, plus two ways out: the paired Stamp Rally card,
 * and - when that rally is filed under a published festival - the festival's
 * dashboard, which is handed this drawing link in the navigation's history state
 * (never the URL) so it opens able to draw.
 */
import { computed, onMounted, ref, watch } from 'vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import GaraponPanel from '@/components/player/GaraponPanel.vue'
import { useGaraponsStore } from '@/stores/garapons'
import { festivalConnectState } from '@/stores/festivalDashboard'

const props = defineProps<{ token: string }>()

const garapons = useGaraponsStore()

const notFound = ref(false)

async function load(token: string): Promise<void> {
  notFound.value = false
  const ok = await garapons.loadByToken(token)
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
  const link = garapons.publicFestivalLink
  return link
    ? {
        name: 'festival-dashboard',
        params: { id: link },
        state: festivalConnectState({ garaponToken: props.token }),
      }
    : null
})
</script>

<template>
  <div v-if="garapons.publicGarapon">
    <div class="topbar">
      <span></span>
      <h2>{{ garapons.publicGarapon.title }}</h2>
      <span></span>
    </div>

    <div class="tab-body content-container content-container--narrow">
      <RouterLink v-if="festivalTarget" :to="festivalTarget" class="link-card mb-16">
        <font-awesome-icon :icon="['fad', 'map-location-dot']" />
        <span>Open the festival dashboard</span>
      </RouterLink>
      <!-- Linked Stamp Rally card: shown only when this drawing link was issued
           alongside a paired rally card. -->
      <router-link
        v-else-if="garapons.publicStampCardToken"
        :to="{ name: 'stamp-card', params: { token: garapons.publicStampCardToken } }"
        class="link-card mb-16"
      >
        <font-awesome-icon :icon="['fad', 'stamp']" />
        <span>View your Stamp Rally card</span>
      </router-link>

      <GaraponPanel :token="token" />
    </div>
  </div>

  <!-- Not found -->
  <div v-else-if="notFound" class="tab-body">
    <p class="page-message text-muted">
      <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> This drawing link is invalid or has been
      removed.
    </p>
  </div>

  <!-- Loading -->
  <div v-else-if="garapons.publicLoading" class="tab-body">
    <LoadingSpinner block label="Loading garapon..." />
  </div>
</template>
