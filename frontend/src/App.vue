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
import YoeverOverlay from '@/components/common/YoeverOverlay.vue'
import { useWebSocket } from '@/composables/useWebSocket'
import { useAppStore } from '@/stores/app'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'

const route = useRoute()
const app = useAppStore()
const player = usePlayerStore()
const raffles = useRafflesStore()

const { client: ws } = useWebSocket()

/**
 * Footer shows only on the public (non-admin) pages, not the admin
 * dashboard/login - and not on an embed, which is our document inside somebody
 * else's page: our footer there would be a second one stacked inside theirs.
 */
const showFooter = computed(
  () => !String(route.name ?? '').startsWith('admin') && !route.meta.embed,
)

// Drive the shared WebSocket from the active route via a *stable connection key*
// rather than the raw route name, so the socket is (re)connected only when the
// intended target actually changes - not on every admin sub-route navigation
// (which previously tore the socket down and reconnected without the reconnect
// catch-up, dropping missed draws/winners/logs). Keys:
//   `player:<cardId>` - a loaded player board
//   `admin`           - any live admin page
//   null              - no socket (home, auth pages, and the hidden
//                       /admin/register page, which has no session yet)
// Watching the player's card id too ensures that on a direct link / refresh to
// /play/:cardId we connect once the board has finished loading (the card id is
// null at the moment the route first changes).
const wsKey = computed<string | null>(() => {
  const name = route.name
  if (name === 'player') {
    return player.playerCard?.id ? `player:${player.playerCard.id}` : null
  }
  if (
    typeof name === 'string' &&
    name.startsWith('admin') &&
    name !== 'admin-login' &&
    name !== 'admin-register'
  ) {
    return 'admin'
  }
  return null
})

// A Vue watch on a primitive computed fires only when the value changes, so
// navigating between admin sub-routes (key stays `admin`) is a no-op and the
// live socket - plus its internal reconnect/catch-up - is left untouched.
watch(
  wsKey,
  (key) => {
    if (key === null) {
      ws.disconnect()
    } else if (key.startsWith('player:')) {
      ws.connect(key.slice('player:'.length))
    } else {
      ws.connect(null)
    }
  },
  { immediate: true },
)

onMounted(() => {
  // Apply the per-browser theme preference (Default follows the admin's active
  // theme; a picked public theme overrides it), load app settings, and preload
  // open raffles so the home page knows whether to show the Raffles card.
  void app.applyThemePreference()
  void app.loadSettings()
  void raffles.loadHomeRaffles()
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
    <!-- The page's own content is the main landmark; the footer stays outside it.
         The site's name heads every page for screen readers, above each page's
         visible title (its h2); on home the logo, which carries the wordmark, is
         that heading instead. -->
    <main>
      <h1 v-if="route.name !== 'home'" class="sr-only">
        {{ app.settings.app_title || 'Senpan App Suite' }}
      </h1>
      <router-view />
    </main>
    <AppFooter v-if="showFooter" />
    <!-- "It's Yoever" reaction - global so it shows on both player and admin views -->
    <YoeverOverlay />
  </div>
</template>
