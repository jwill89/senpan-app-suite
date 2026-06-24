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
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import AdminSidebar from '@/components/admin/AdminSidebar.vue'
import WinnerVerifyModal from '@/components/admin/WinnerVerifyModal.vue'
import HalftimePromptModal from '@/components/admin/HalftimePromptModal.vue'
import EndGameModal from '@/components/admin/EndGameModal.vue'
import CardPreviewModal from '@/components/admin/CardPreviewModal.vue'
import ModalOverlay from '@/components/common/ModalOverlay.vue'
import FormField from '@/components/common/ui/FormField.vue'
import LoadingSpinner from '@/components/common/LoadingSpinner.vue'
import { useAuthStore } from '@/stores/auth'
import { useAppStore } from '@/stores/app'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { usePatternsStore } from '@/stores/patterns'
import { useRafflesStore } from '@/stores/raffles'
import { useUiStore } from '@/stores/ui'
import { endpoints } from '@/lib/endpoints'

const router = useRouter()
const auth = useAuthStore()
const app = useAppStore()
const game = useGameStore()
const cards = useCardsStore()
const patterns = usePatternsStore()
const raffles = useRafflesStore()
const ui = useUiStore()

onMounted(async () => {
  // Load the core admin data set, but only what this account may access (admins
  // can read everything). Settings are public, so they always load. allSettled
  // keeps one 403 from aborting the rest.
  try {
    const loads: Promise<unknown>[] = [app.loadSettings()]
    if (auth.hasPermission('bingo-cards')) loads.push(cards.loadCards())
    if (auth.hasPermission('bingo-patterns')) loads.push(patterns.loadPatterns())
    if (auth.hasPermission('bingo-game')) loads.push(game.loadGameState())
    if (auth.hasPermission('teahouse-raffles')) loads.push(raffles.loadRaffles())
    await Promise.allSettled(loads)
    game.drawDelay = parseInt(app.settings.default_draw_delay) || 0
  } catch {
    /* show the dashboard even if a data load failed */
  }
})

// ── Mobile nav drawer ────────────────────────────────────────────────────────
// On narrow viewports the sidebar collapses into a hamburger-triggered drawer
// (see app.css). Selecting a nav item navigates, so close the drawer on every
// route change; tapping the backdrop closes it too.
const navOpen = ref(false)
watch(
  () => router.currentRoute.value.fullPath,
  () => {
    navOpen.value = false
  },
)

// The hamburger toggle (and drawer backdrop) only exist on narrow viewports where
// the sidebar collapses into a drawer. We drive this from matchMedia and `v-if`
// rather than CSS `display`, so a custom theme's generic button styling can't
// accidentally re-show the button on desktop (where the sidebar is always shown).
const NARROW_QUERY = '(max-width: 768px)'
const isNarrow = ref(false)
let navMql: MediaQueryList | null = null
function syncIsNarrow(): void {
  isNarrow.value = !!navMql?.matches
  if (!isNarrow.value) navOpen.value = false // back to desktop → ensure the drawer is closed
}
onMounted(() => {
  navMql = window.matchMedia(NARROW_QUERY)
  syncIsNarrow()
  navMql.addEventListener('change', syncIsNarrow)
})
onBeforeUnmount(() => navMql?.removeEventListener('change', syncIsNarrow))

/** Log out, then return home (App's route watcher disconnects the WebSocket). */
async function logout(): Promise<void> {
  await auth.logout()
  router.push({ name: 'home' })
}

// ── Change-password modal (available to every logged-in account) ──────────────
const showChangePw = ref(false)
const currentPw = ref('')
const newPw = ref('')
const confirmPw = ref('')
const pwError = ref('')
const savingPw = ref(false)

function openChangePw(): void {
  currentPw.value = ''
  newPw.value = ''
  confirmPw.value = ''
  pwError.value = ''
  showChangePw.value = true
}

async function submitChangePw(): Promise<void> {
  pwError.value = ''
  if (newPw.value.length < 8) {
    pwError.value = 'New password must be at least 8 characters.'
    return
  }
  if (newPw.value !== confirmPw.value) {
    pwError.value = 'New passwords do not match.'
    return
  }
  savingPw.value = true
  try {
    await endpoints.account.changePassword(currentPw.value, newPw.value)
    ui.notify('Password changed', 'success')
    showChangePw.value = false
  } catch (e) {
    pwError.value = (e as Error).message
  } finally {
    savingPw.value = false
  }
}
</script>

<template>
  <div>
    <div class="topbar">
      <div class="flex gap-sm">
        <button
          v-if="isNarrow"
          class="btn-neutral btn-sm admin-nav-toggle"
          :aria-expanded="navOpen"
          aria-label="Toggle navigation menu"
          @click="navOpen = !navOpen"
        >
          <font-awesome-icon :icon="['fad', 'bars']" />
        </button>
        <h2><font-awesome-icon :icon="['fad', 'gear']" /> Admin Dashboard</h2>
      </div>
      <div class="topbar-actions">
        <span v-if="auth.user" class="topbar-user text-dim">
          <font-awesome-icon :icon="['fad', 'user']" /> {{ auth.user.username }}
        </span>
        <button class="btn-neutral btn-sm" @click="openChangePw">
          <font-awesome-icon :icon="['fas', 'lock']" /> Change Password
        </button>
        <button class="btn-neutral btn-sm" @click="logout">
          <font-awesome-icon :icon="['fas', 'arrow-right-from-bracket']" /> Logout
        </button>
      </div>
    </div>

    <div class="admin-layout">
      <!-- Backdrop behind the mobile drawer; only exists in the narrow drawer mode. -->
      <div v-if="isNarrow && navOpen" class="admin-nav-backdrop" @click="navOpen = false"></div>

      <AdminSidebar :class="{ 'is-open': navOpen }" />

      <div class="admin-content">
        <div class="admin-content-inner content-container">
          <router-view />
        </div>
      </div>
    </div>

    <!-- Modals (overlay any tab) -->
    <WinnerVerifyModal />
    <HalftimePromptModal />
    <EndGameModal />
    <CardPreviewModal />

    <ModalOverlay
      v-if="showChangePw"
      aria-label="Change password"
      box-style="max-width: 420px"
      @close="showChangePw = false"
    >
      <h3 class="mt-0"><font-awesome-icon :icon="['fad', 'lock']" /> Change Password</h3>
      <form autocomplete="off" @submit.prevent="submitChangePw">
        <FormField label="Current password" html-for="cur-pw">
          <input
            id="cur-pw"
            v-model="currentPw"
            type="password"
            autocomplete="current-password"
          />
        </FormField>
        <FormField label="New password" html-for="new-pw">
          <input
            id="new-pw"
            v-model="newPw"
            type="password"
            autocomplete="new-password"
            placeholder="At least 8 characters"
          />
        </FormField>
        <FormField label="Confirm new password" html-for="confirm-pw">
          <input
            id="confirm-pw"
            v-model="confirmPw"
            type="password"
            autocomplete="new-password"
          />
        </FormField>
        <p v-if="pwError" class="error-msg">{{ pwError }}</p>
        <div class="modal-actions">
          <button
            type="button"
            class="btn-neutral"
            :disabled="savingPw"
            @click="showChangePw = false"
          >
            Cancel
          </button>
          <button type="submit" class="btn-action" :disabled="savingPw">
            <LoadingSpinner v-if="savingPw" label="Saving…" />
            <template v-else>Change Password</template>
          </button>
        </div>
      </form>
    </ModalOverlay>
  </div>
</template>

<style scoped>
.topbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}
.topbar-user {
  font-size: 0.85rem;
}
.modal-actions {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 16px;
}
</style>
