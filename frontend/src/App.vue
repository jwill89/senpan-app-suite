<script setup lang="ts">
/**
 * Root application shell.
 *
 * With Vue Router in place, this is a thin shell: it renders the matched route
 * via <router-view>, shows the global toast, keeps the tab's one live stream
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
import { useLiveEvents, type LiveTarget } from '@/composables/useLiveEvents'
import { useAppStore } from '@/stores/app'
import { useAuthStore } from '@/stores/auth'
import { usePlayerStore } from '@/stores/player'
import { useRafflesStore } from '@/stores/raffles'

const route = useRoute()
const app = useAppStore()
const auth = useAuthStore()
const player = usePlayerStore()
const raffles = useRafflesStore()

const live = useLiveEvents()

/**
 * Footer shows only on the public (non-admin) pages, not the admin
 * dashboard/login - and not on an embed, which is our document inside somebody
 * else's page: our footer there would be a second one stacked inside theirs.
 */
const showFooter = computed(
  () => !String(route.name ?? '').startsWith('admin') && !route.meta.embed,
)

// The tab's live stream follows the route through a *stable key* rather than the
// raw route name, so the stream is opened again only when what it follows changes,
// not on every admin sub-route navigation:
//   `board:<cardId>` - a loaded player board: that board's stream
//   `staff`          - any live admin page, for an account with a page to follow
//   null             - no stream (home, the sign-in pages, an account with no
//                      page granted yet)
// Watching the player's card id too ensures that on a direct link / refresh to
// /play/:cardId the stream opens once the board has finished loading (the card id
// is null at the moment the route first changes).
const liveKey = computed<string | null>(() => {
  const name = route.name
  if (name === 'player') {
    return player.playerCard?.id ? `board:${player.playerCard.id}` : null
  }
  if (
    typeof name === 'string' &&
    name.startsWith('admin') &&
    name !== 'admin-login' &&
    name !== 'admin-register' &&
    auth.user &&
    (auth.user.isAdmin || auth.user.permissions.length > 0)
  ) {
    return 'staff'
  }
  return null
})

// A Vue watch on a primitive computed fires only when the value changes, so
// navigating between admin sub-routes (key stays `staff`) leaves the stream, and
// its own reopening and catching up, untouched.
watch(
  liveKey,
  (key) => {
    let target: LiveTarget = null
    if (key === 'staff') {
      target = { kind: 'staff' }
    } else if (key?.startsWith('board:')) {
      target = { kind: 'board', cardId: key.slice('board:'.length) }
    }
    live.follow(target)
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
  live.stop()
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
        {{ app.settings.appTitle || 'Senpan App Suite' }}
      </h1>
      <router-view />
    </main>
    <AppFooter v-if="showFooter" />
    <!-- "It's Yoever" reaction - global so it shows on both player and admin views -->
    <YoeverOverlay />
  </div>
</template>
