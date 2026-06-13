<script setup lang="ts">
/**
 * Root application shell.
 *
 * With Vue Router in place, this is a thin shell: it renders the matched route
 * via <router-view>, shows the global toast, owns the shared WebSocket lifecycle
 * (driven by the current route), and performs the one-time global loads
 * (active theme CSS, app settings, open-raffle preload) on mount.
 *
 * Per-view orchestration (joining a board, loading admin data, etc.) lives in
 * the individual views and the router guard, not here.
 */
import { onBeforeUnmount, onMounted, computed, watch } from 'vue'
import { useRoute } from 'vue-router'
import ToastNotification from '@/components/common/ToastNotification.vue'
import ConfirmModal from '@/components/common/ConfirmModal.vue'
import RouteProgressBar from '@/components/common/RouteProgressBar.vue'
import AppFooter from '@/components/common/AppFooter.vue'
import { useWebSocket } from '@/composables/useWebSocket'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'

const route = useRoute()
const app = useAppStore()
const player = usePlayerStore()
const raffles = useRafflesStore()

const { client: ws } = useWebSocket()

/** Footer shows only on the public (non-admin) pages, not the admin dashboard/login. */
const showFooter = computed(() => !String(route.name ?? '').startsWith('admin'))

// Drive the shared WebSocket from the active route: connect (with the player's
// card id) on the player view, connect without an id on admin views, and
// disconnect everywhere else. Watching the player's card id too ensures that on
// a direct link / refresh to /play/:cardId we (re)connect once the board has
// finished loading (the card id is null at the moment the route first changes).
watch(
  () => [route.name, player.playerCard?.id] as const,
  ([name, cardId]) => {
    if (name === 'player') {
      if (cardId) ws.connect(cardId)
    } else if (typeof name === 'string' && name.startsWith('admin') && name !== 'admin-login') {
      ws.connect(null)
    } else {
      ws.disconnect()
    }
  },
  { immediate: true },
)

onMounted(() => {
  // Load the active theme CSS + app settings, and preload open raffles so the
  // home page knows whether to show the Raffles card (mirrors app.js mounted()).
  app.loadActiveCSS()
  app.loadSettings()
  raffles.loadHomeRaffles()
})

onBeforeUnmount(() => {
  ws.disconnect()
})
</script>

<template>
  <div>
    <RouteProgressBar />
    <ToastNotification />
    <ConfirmModal />
    <router-view />
    <AppFooter v-if="showFooter" />
  </div>
</template>
