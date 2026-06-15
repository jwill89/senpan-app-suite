<script setup lang="ts">
/**
 * Admin dashboard shell: a topbar, the collapsible sidebar, and a content area
 * that renders the active admin tab via <router-view> (the tab is selected by
 * the matched `/admin/...` child route). The four admin modals (winner
 * verification, halftime prompt, end-game, card preview) live here so they
 * overlay any tab.
 *
 * Loads the initial admin data set once when the dashboard mounts (this used to
 * happen in the App shell after a successful login; now it runs whenever the
 * admin area is entered, including via a direct link / refresh). Logout
 * disconnects the WebSocket implicitly (App's route watcher) and returns home.
 */
import { onMounted } from 'vue'
import { useRouter } from 'vue-router'
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import WinnerVerifyModal from '@/components/admin/WinnerVerifyModal.vue'
import HalftimePromptModal from '@/components/admin/HalftimePromptModal.vue'
import EndGameModal from '@/components/admin/EndGameModal.vue'
import CardPreviewModal from '@/components/admin/CardPreviewModal.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { useRafflesStore } from '@/stores/raffles'

const router = useRouter()
const auth = useAuthStore()
const app = useAppStore()
const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const raffles = useRafflesStore()

onMounted(async () => {
  // Load the core admin data set (mirrors the old App.onAdminLoginSuccess).
  try {
    await Promise.all([
      cards.loadCards(),
      patterns.loadPatterns(),
      game.loadGameState(),
      raffles.loadRaffles(),
      app.loadSettings(),
    ])
    game.drawDelay = parseInt(app.settings.default_draw_delay) || 0
  } catch {
    /* show the dashboard even if a data load failed */
  }
})

/** Log out, then return home (App's route watcher disconnects the WebSocket). */
async function logout(): Promise<void> {
  await auth.logout()
  router.push({ name: 'home' })
}
</script>

<template>
  <div>
    <div class="topbar">
      <h2><i class="fa-duotone fa-gear"></i> Admin Dashboard</h2>
      <button class="btn-neutral btn-sm" @click="logout">
        <i class="fa-solid fa-arrow-right-from-bracket"></i> Logout
      </button>
    </div>

    <div class="admin-layout">
      <AdminSidebar />

      <div class="admin-content">
        <router-view />
      </div>
    </div>

    <!-- Modals (overlay any tab) -->
    <WinnerVerifyModal />
    <HalftimePromptModal />
    <EndGameModal />
    <CardPreviewModal />
  </div>
</template>
