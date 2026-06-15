<script setup lang="ts">
/**
 * Admin sidebar navigation — collapsible Bingo / Raffles / System sections.
 * Navigates via the router (`/admin/...` routes); the active tab/section
 * highlight reads from the admin store, which the router guard keeps in sync
 * with the matched route.
 *
 * NOTE: the nav items are intentionally <button>s (with programmatic
 * router.push), not <RouterLink>/<a>. app.css (and user-authored custom themes)
 * style `.admin-nav-items button`, so switching to anchors would silently break
 * the sidebar's appearance under existing themes. The minor RouterLink perks
 * (middle-click / open-in-new-tab) aren't worth that theme-fidelity cost here.
 */
import { computed } from 'vue'
import { useRouter } from 'vue-router'
import { adminTabRouteName } from '@/router'
import { useAdminStore, type AdminSection, type AdminTab } from '@/stores/admin'
import { useAuthStore } from '@/stores/auth'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { useRafflesStore } from '@/stores/raffles'
import { BOOK_CLUBS } from '@/lib/constants'

const router = useRouter()
const admin = useAdminStore()
const auth = useAuthStore()
const game = useGameStore()
const cards = useCardsStore()
const raffles = useRafflesStore()

/** Whether the current account may access a page (admins → everything). */
function can(key: string): boolean {
  return auth.hasPermission(key)
}

// Section visibility: show a section only when the account can access at least
// one of its pages (admins see all). The System section also appears for admins
// because the Users page lives there.
const showBingo = computed(() =>
  ['bingo-game', 'bingo-cards', 'bingo-winners-log', 'bingo-patterns', 'bingo-presets'].some(can),
)
const showTeahouse = computed(() =>
  ['teahouse-announcements', 'teahouse-raffles', ...BOOK_CLUBS.map((c) => `bookclub-${c.slug}`)].some(
    can,
  ),
)
const showAtelier = computed(() => ['atelier-fonts', 'atelier-carrd'].some(can))
const showSystem = computed(() => auth.isAdmin || ['system-settings', 'system-themes'].some(can))

/** Navigate to an admin tab. */
function go(tab: AdminTab): void {
  router.push({ name: adminTabRouteName(tab) })
}

// Default tab opened when a section header is clicked (matches the old
// toggleSection behaviour). Clicking the already-open section is a no-op.
const sectionDefaultTab: Record<AdminSection, AdminTab> = {
  bingo: 'bingo-game',
  teahouse: 'teahouse-announcements',
  atelier: 'atelier-fonts',
  system: 'system-settings',
}
function toggle(section: AdminSection): void {
  if (admin.adminSection === section) return
  go(sectionDefaultTab[section])
}
</script>

<template>
  <nav class="admin-sidebar">
    <!-- Bingo section -->
    <div v-if="showBingo" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'bingo' }"
        @click="toggle('bingo')"
      >
        <span><i class="fa-duotone fa-circle-dot"></i> Bingo</span>
        <span class="nav-chevron">{{ admin.adminSection === 'bingo' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'bingo'" class="admin-nav-items">
        <button
          v-if="can('bingo-game')"
          :class="{ active: admin.adminTab === 'bingo-game' }"
          @click="go('bingo-game')"
        >
          <i class="fa-duotone fa-gamepad"></i> {{ game.adminGameLabel }}
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
          <i class="fa-duotone fa-id-card"></i> Manage Cards
          <span v-if="cards.cards.length" class="nav-count">({{ cards.cards.length }})</span>
        </button>
        <button
          v-if="can('bingo-winners-log')"
          :class="{ active: admin.adminTab === 'bingo-winners-log' }"
          @click="go('bingo-winners-log')"
        >
          <i class="fa-duotone fa-trophy"></i> Winners Log
        </button>
        <button
          v-if="can('bingo-patterns')"
          :class="{ active: admin.adminTab === 'bingo-patterns' }"
          @click="go('bingo-patterns')"
        >
          <i class="fa-duotone fa-grid"></i> Patterns
        </button>
        <button
          v-if="can('bingo-presets')"
          :class="{ active: admin.adminTab === 'bingo-presets' }"
          @click="go('bingo-presets')"
        >
          <i class="fa-duotone fa-ballot"></i> Game Presets
        </button>
      </div>
    </div>

    <!-- Senpan Tea House section (Announcements + Raffles + the book clubs) -->
    <div v-if="showTeahouse" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'teahouse' }"
        @click="toggle('teahouse')"
      >
        <span><i class="fa-duotone fa-torii-gate"></i> Senpan Tea House</span>
        <span class="nav-chevron">{{ admin.adminSection === 'teahouse' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'teahouse'" class="admin-nav-items">
        <button
          v-if="can('teahouse-announcements')"
          :class="{ active: admin.adminTab === 'teahouse-announcements' }"
          @click="go('teahouse-announcements')"
        >
          <i class="fa-duotone fa-megaphone"></i> Announcements
        </button>
        <button
          v-if="can('teahouse-raffles')"
          :class="{ active: admin.adminTab === 'teahouse-raffles' }"
          @click="go('teahouse-raffles')"
        >
          <i class="fa-duotone fa-ticket"></i> Raffles
          <span v-if="raffles.openRaffles.length" class="nav-count">
            ({{ raffles.openRaffles.length }})
          </span>
        </button>
        <button
          v-for="club in BOOK_CLUBS"
          v-show="can(`bookclub-${club.slug}`)"
          :key="club.slug"
          :class="{ active: admin.adminTab === `bookclub-${club.slug}` }"
          @click="go(`bookclub-${club.slug}` as AdminTab)"
        >
          <i class="fa-duotone" :class="club.icon"></i> {{ club.name }}
        </button>
      </div>
    </div>

    <!-- Atelier Yao section -->
    <div v-if="showAtelier" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'atelier' }"
        @click="toggle('atelier')"
      >
        <span><i class="fa-duotone fa-compass-drafting"></i> Atelier Yao</span>
        <span class="nav-chevron">{{ admin.adminSection === 'atelier' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'atelier'" class="admin-nav-items">
        <button
          v-if="can('atelier-fonts')"
          :class="{ active: admin.adminTab === 'atelier-fonts' }"
          @click="go('atelier-fonts')"
        >
          <i class="fa-duotone fa-font"></i> Font Upload
        </button>
        <button
          v-if="can('atelier-carrd')"
          :class="{ active: admin.adminTab === 'atelier-carrd' }"
          @click="go('atelier-carrd')"
        >
          <i class="fa-duotone fa-images"></i> Carrd Upload
        </button>
      </div>
    </div>

    <!-- System section -->
    <div v-if="showSystem" class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'system' }"
        @click="toggle('system')"
      >
        <span><i class="fa-duotone fa-gear"></i> System</span>
        <span class="nav-chevron">{{ admin.adminSection === 'system' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'system'" class="admin-nav-items">
        <button
          v-if="can('system-settings')"
          :class="{ active: admin.adminTab === 'system-settings' }"
          @click="go('system-settings')"
        >
          <i class="fa-duotone fa-gear"></i> App Settings
        </button>
        <button
          v-if="can('system-themes')"
          :class="{ active: admin.adminTab === 'system-themes' }"
          @click="go('system-themes')"
        >
          <i class="fa-duotone fa-palette"></i> Themes
        </button>
        <button
          v-if="auth.isAdmin"
          :class="{ active: admin.adminTab === 'system-users' }"
          @click="go('system-users')"
        >
          <i class="fa-duotone fa-users-gear"></i> Users
        </button>
      </div>
    </div>
  </nav>
</template>
