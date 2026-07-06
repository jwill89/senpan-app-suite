<script setup lang="ts">
/**
 * Admin sidebar navigation — accordion sections (Bingo / Senpan Tea House /
 * Atelier Yao / System). Section headers are pure accordion toggles: clicking
 * one shows/hides the items it contains and never navigates, and any number of
 * sections can be open at once (independent toggles). Only the items navigate
 * (via the router). A section is hidden entirely when the account can't access
 * any of its pages. The active item's highlight reads from the admin store,
 * which the router guard keeps in sync with the matched route.
 *
 * NOTE: the nav items are intentionally <button>s (with programmatic
 * router.push), not <RouterLink>/<a>. app.css (and user-authored custom themes)
 * style `.admin-nav-items button`, so switching to anchors would silently break
 * the sidebar's appearance under existing themes. The minor RouterLink perks
 * (middle-click / open-in-new-tab) aren't worth that theme-fidelity cost here.
 */
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { adminTabRouteName } from '@/router'
import { useAdminStore, type AdminSection, type AdminTab } from '@/stores/admin'
import { useAuthStore } from '@/stores/auth'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { useRafflesStore } from '@/stores/raffles'
import { BOOK_CLUBS } from '@/lib/constants'
import AppVersions from '@/components/admin/AppVersions.vue'

const router = useRouter()
const admin = useAdminStore()
const auth = useAuthStore()
const game = useGameStore()
const cards = useCardsStore()
const raffles = useRafflesStore()

// Change Password / Logout are actions, not navigation: the sidebar emits them so
// the admin shell (which owns the change-password modal + session) handles them.
const emit = defineEmits<{
  'access-token': []
  'change-password': []
  'manage-passkeys': []
  logout: []
}>()
// "User Options" isn't tied to a route/tab, so it tracks its own accordion state.
const userOptionsOpen = ref(false)

/** Whether the current account may access a page (admins → everything). */
function can(key: string): boolean {
  return auth.hasPermission(key)
}

/** Book clubs the current account may access (filtered so the template can
 * `v-for` without a per-row `v-show`). */
const visibleClubs = computed(() => BOOK_CLUBS.filter((c) => can(`bookclub-${c.slug}`)))

// Section visibility: show a section only when the account can access at least
// one of its pages (admins see all). The System section also appears for admins
// because the Users page lives there.
const showBingo = computed(() =>
  ['bingo-game', 'bingo-cards', 'bingo-winners-log', 'bingo-patterns', 'bingo-presets'].some(can),
)
const showTeahouse = computed(() =>
  [
    'teahouse-announcements',
    'teahouse-affiliates',
    ...BOOK_CLUBS.map((c) => `bookclub-${c.slug}`),
  ].some(can),
)
// Festival now also hosts Raffles (moved out of Senpan Tea House). The Raffles
// page keeps its `teahouse-raffles` permission/route id; only its placement moved.
const showFestival = computed(() =>
  ['festival-garapon', 'festival-stamp-rally', 'teahouse-raffles'].some(can),
)
const showAtelier = computed(() => ['atelier-fonts', 'atelier-carrd'].some(can))
const showSystem = computed(
  () => auth.isAdmin || ['system-settings', 'system-themes', 'system-images'].some(can),
)

/** Navigate to an admin tab (items navigate; headers don't — see toggleSection). */
function go(tab: AdminTab): void {
  void router.push({ name: adminTabRouteName(tab) })
}

// Which sections are expanded. Headers are independent accordion toggles: each
// shows/hides its own items and never navigates, and any number can be open at
// once. A section auto-opens when navigation makes it active (so the highlighted
// item is visible); manually opened/closed sections are otherwise left alone.
const openSections = reactive(new Set<AdminSection>([admin.adminSection]))
watch(
  () => admin.adminTab,
  () => {
    openSections.add(admin.adminSection)
  },
)

/** Whether a section's items are expanded. */
function isOpen(section: AdminSection): boolean {
  return openSections.has(section)
}

/** Accordion toggle for a section header (no navigation; independent per section). */
function toggleSection(section: AdminSection): void {
  if (openSections.has(section)) openSections.delete(section)
  else openSections.add(section)
}
</script>

<template>
  <nav class="admin-sidebar">
    <!-- Bingo section -->
    <div v-if="showBingo" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: isOpen('bingo') }"
        @click="toggleSection('bingo')"
      >
        <span><font-awesome-icon :icon="['fad', 'circle-dot']" /> Bingo</span>
        <span class="nav-chevron">{{ isOpen('bingo') ? '▾' : '▸' }}</span>
      </div>
      <div v-show="isOpen('bingo')" class="admin-nav-items">
        <button
          v-if="can('bingo-game')"
          :class="{ active: admin.adminTab === 'bingo-game' }"
          @click="go('bingo-game')"
        >
          <font-awesome-icon :icon="['fad', 'gamepad']" /> {{ game.adminGameLabel }}
          <span
            v-if="game.currentGame"
            class="live-dot nav-live-dot"
            role="status"
            aria-label="Game in progress"
          ></span>
        </button>
        <button
          v-if="can('bingo-cards')"
          :class="{ active: admin.adminTab === 'bingo-cards' }"
          @click="go('bingo-cards')"
        >
          <font-awesome-icon :icon="['fad', 'id-card']" /> Manage Cards
          <span v-if="cards.cards.length" class="nav-count">({{ cards.cards.length }})</span>
        </button>
        <button
          v-if="can('bingo-patterns')"
          :class="{ active: admin.adminTab === 'bingo-patterns' }"
          @click="go('bingo-patterns')"
        >
          <font-awesome-icon :icon="['fad', 'grid']" /> Patterns
        </button>
        <button
          v-if="can('bingo-presets')"
          :class="{ active: admin.adminTab === 'bingo-presets' }"
          @click="go('bingo-presets')"
        >
          <font-awesome-icon :icon="['fad', 'ballot']" /> Game Presets
        </button>
        <button
          v-if="can('bingo-winners-log')"
          :class="{ active: admin.adminTab === 'bingo-winners-log' }"
          @click="go('bingo-winners-log')"
        >
          <font-awesome-icon :icon="['fad', 'trophy']" /> Winners Log
        </button>
      </div>
    </div>

    <!-- Senpan Tea House section (Announcements + Raffles + the book clubs) -->
    <div v-if="showTeahouse" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: isOpen('teahouse') }"
        @click="toggleSection('teahouse')"
      >
        <span><font-awesome-icon :icon="['fad', 'torii-gate']" /> Senpan Tea House</span>
        <span class="nav-chevron">{{ isOpen('teahouse') ? '▾' : '▸' }}</span>
      </div>
      <div v-show="isOpen('teahouse')" class="admin-nav-items">
        <button
          v-if="can('teahouse-affiliates')"
          :class="{ active: admin.adminTab === 'teahouse-affiliates' }"
          @click="go('teahouse-affiliates')"
        >
          <font-awesome-icon :icon="['fad', 'handshake']" /> Affiliates
        </button>
        <button
          v-if="can('teahouse-announcements')"
          :class="{ active: admin.adminTab === 'teahouse-announcements' }"
          @click="go('teahouse-announcements')"
        >
          <font-awesome-icon :icon="['fad', 'megaphone']" /> Announcements
        </button>
        <button
          v-for="club in visibleClubs"
          :key="club.slug"
          :class="{ active: admin.adminTab === `bookclub-${club.slug}` }"
          @click="go(`bookclub-${club.slug}` as AdminTab)"
        >
          <font-awesome-icon :icon="['fad', club.icon]" /> {{ club.name }}
        </button>
      </div>
    </div>

    <!-- Festival section (Garapon) -->
    <div v-if="showFestival" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: isOpen('festival') }"
        @click="toggleSection('festival')"
      >
        <span><font-awesome-icon :icon="['fad', 'pot-food']" /> Festival</span>
        <span class="nav-chevron">{{ isOpen('festival') ? '▾' : '▸' }}</span>
      </div>
      <div v-show="isOpen('festival')" class="admin-nav-items">
        <button
          v-if="can('festival-garapon')"
          :class="{ active: admin.adminTab === 'festival-garapon' }"
          @click="go('festival-garapon')"
        >
          <font-awesome-icon :icon="['fad', 'ferris-wheel']" /> Garapon
        </button>
        <button
          v-if="can('teahouse-raffles')"
          :class="{ active: admin.adminTab === 'teahouse-raffles' }"
          @click="go('teahouse-raffles')"
        >
          <font-awesome-icon :icon="['fad', 'ticket']" /> Raffles
          <span v-if="raffles.openRaffles.length" class="nav-count">
            ({{ raffles.openRaffles.length }})
          </span>
        </button>
        <button
          v-if="can('festival-stamp-rally')"
          :class="{ active: admin.adminTab === 'festival-stamp-rally' }"
          @click="go('festival-stamp-rally')"
        >
          <font-awesome-icon :icon="['fad', 'stamp']" /> Stamp Rally
        </button>
      </div>
    </div>

    <!-- Atelier Yao section -->
    <div v-if="showAtelier" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: isOpen('atelier') }"
        @click="toggleSection('atelier')"
      >
        <span><font-awesome-icon :icon="['fad', 'compass-drafting']" /> Atelier Yao</span>
        <span class="nav-chevron">{{ isOpen('atelier') ? '▾' : '▸' }}</span>
      </div>
      <div v-show="isOpen('atelier')" class="admin-nav-items">
        <button
          v-if="can('atelier-carrd')"
          :class="{ active: admin.adminTab === 'atelier-carrd' }"
          @click="go('atelier-carrd')"
        >
          <font-awesome-icon :icon="['fad', 'images']" /> Carrd Upload
        </button>
        <button
          v-if="can('atelier-fonts')"
          :class="{ active: admin.adminTab === 'atelier-fonts' }"
          @click="go('atelier-fonts')"
        >
          <font-awesome-icon :icon="['fad', 'font']" /> Font Upload
        </button>
      </div>
    </div>

    <!-- System section -->
    <div v-if="showSystem" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: isOpen('system') }"
        @click="toggleSection('system')"
      >
        <span><font-awesome-icon :icon="['fad', 'gears']" /> System</span>
        <span class="nav-chevron">{{ isOpen('system') ? '▾' : '▸' }}</span>
      </div>
      <div v-show="isOpen('system')" class="admin-nav-items">
        <button
          v-if="can('system-images')"
          :class="{ active: admin.adminTab === 'system-images' }"
          @click="go('system-images')"
        >
          <font-awesome-icon :icon="['fad', 'images']" /> Images
        </button>
        <button
          v-if="can('system-themes')"
          :class="{ active: admin.adminTab === 'system-themes' }"
          @click="go('system-themes')"
        >
          <font-awesome-icon :icon="['fad', 'palette']" /> Themes
        </button>
        <button
          v-if="auth.isAdmin"
          :class="{ active: admin.adminTab === 'system-users' }"
          @click="go('system-users')"
        >
          <font-awesome-icon :icon="['fad', 'users-gear']" /> Users
        </button>
        <button
          v-if="can('system-settings')"
          :class="{ active: admin.adminTab === 'system-settings' }"
          @click="go('system-settings')"
        >
          <font-awesome-icon :icon="['fad', 'gear']" /> Settings
        </button>
        <button
          v-if="auth.isAdmin"
          :class="{ active: admin.adminTab === 'system-logs' }"
          @click="go('system-logs')"
        >
          <font-awesome-icon :icon="['fad', 'clipboard-clock']" /> Logs
        </button>
      </div>
    </div>
    <!-- User Options (Change Password / Logout) — actions, not navigation. -->
    <div class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: userOptionsOpen }"
        @click="userOptionsOpen = !userOptionsOpen"
      >
        <span><font-awesome-icon :icon="['fad', 'user']" /> User Options</span>
        <span class="nav-chevron">{{ userOptionsOpen ? '▾' : '▸' }}</span>
      </div>
      <div v-show="userOptionsOpen" class="admin-nav-items">
        <button @click="emit('access-token')">
          <font-awesome-icon :icon="['fad', 'key']" /> Access Token
        </button>
        <button @click="emit('change-password')">
          <font-awesome-icon :icon="['fad', 'lock']" /> Change Password
        </button>
        <button @click="emit('manage-passkeys')">
          <font-awesome-icon :icon="['fad', 'user-key']" /> Add Passkey
        </button>
        <button @click="emit('logout')">
          <font-awesome-icon :icon="['fas', 'arrow-right-from-bracket']" /> Logout
        </button>
      </div>
    </div>

    <!-- Frontend/backend version readout (compatibility check). -->
    <AppVersions />
  </nav>
</template>
