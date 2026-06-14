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
import { useRouter } from 'vue-router'
import { adminTabRouteName } from '@/router'
import { useAdminStore, type AdminSection, type AdminTab } from '@/stores/admin'
import { useGameStore } from '@/stores/game'
import { useCardsStore } from '@/stores/cards'
import { useRafflesStore } from '@/stores/raffles'
import { BOOK_CLUBS } from '@/lib/constants'

const router = useRouter()
const admin = useAdminStore()
const game = useGameStore()
const cards = useCardsStore()
const raffles = useRafflesStore()

/** Navigate to an admin tab. */
function go(tab: AdminTab): void {
  router.push({ name: adminTabRouteName(tab) })
}

// Default tab opened when a section header is clicked (matches the old
// toggleSection behaviour). Clicking the already-open section is a no-op.
const sectionDefaultTab: Record<AdminSection, AdminTab> = {
  bingo: 'bingo-game',
  raffles: 'raffle-open',
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
    <div class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'bingo' }"
        @click="toggle('bingo')"
      >
        <span><i class="fa-duotone fa-circle-dot"></i> Bingo</span>
        <span class="nav-chevron">{{ admin.adminSection === 'bingo' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'bingo'" class="admin-nav-items">
        <button :class="{ active: admin.adminTab === 'bingo-game' }" @click="go('bingo-game')">
          <i class="fa-duotone fa-gamepad"></i> {{ game.adminGameLabel }}
          <span
            v-if="game.currentGame"
            class="live-dot nav-live-dot"
            role="status"
            aria-label="Game in progress"
          ></span>
        </button>
        <button :class="{ active: admin.adminTab === 'bingo-cards' }" @click="go('bingo-cards')">
          <i class="fa-duotone fa-id-card"></i> Manage Cards
          <span v-if="cards.cards.length" class="nav-count">({{ cards.cards.length }})</span>
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-winners-log' }"
          @click="go('bingo-winners-log')"
        >
          <i class="fa-duotone fa-trophy"></i> Winners Log
        </button>
        <span class="admin-nav-sub-header">Patterns</span>
        <button
          :class="{ active: admin.adminTab === 'bingo-categories' }"
          @click="go('bingo-categories')"
        >
          <i class="fa-duotone fa-folder-open"></i> Pattern Categories
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-new-pattern' }"
          @click="go('bingo-new-pattern')"
        >
          <i class="fa-duotone fa-plus"></i> New Pattern
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-patterns' }"
          @click="go('bingo-patterns')"
        >
          <i class="fa-duotone fa-pen-to-square"></i> Edit Patterns
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-presets' }"
          @click="go('bingo-presets')"
        >
          <i class="fa-duotone fa-layer-group"></i> Game Presets
        </button>
      </div>
    </div>

    <!-- Raffles section -->
    <div class="admin-nav-section">
      <div
        class="admin-nav-header"
        :class="{ open: admin.adminSection === 'raffles' }"
        @click="toggle('raffles')"
      >
        <span><i class="fa-duotone fa-ticket"></i> Raffles</span>
        <span class="nav-chevron">{{ admin.adminSection === 'raffles' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'raffles'" class="admin-nav-items">
        <button :class="{ active: admin.adminTab === 'raffle-new' }" @click="go('raffle-new')">
          <i class="fa-duotone fa-plus"></i> New Raffle
        </button>
        <button :class="{ active: admin.adminTab === 'raffle-open' }" @click="go('raffle-open')">
          <i class="fa-duotone fa-clipboard-list"></i> Open Raffles
          <span v-if="raffles.openRaffles.length" class="nav-count">
            ({{ raffles.openRaffles.length }})
          </span>
        </button>
        <button
          :class="{ active: admin.adminTab === 'raffle-closed' }"
          @click="go('raffle-closed')"
        >
          <i class="fa-duotone fa-lock"></i> Closed Raffles
          <span v-if="raffles.closedRaffles.length" class="nav-count">
            ({{ raffles.closedRaffles.length }})
          </span>
        </button>
      </div>
    </div>

    <!-- Senpan Tea House section (Announcement Management + the book clubs) -->
    <div class="admin-nav-section">
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
          :class="{ active: admin.adminTab === 'teahouse-announcements' }"
          @click="go('teahouse-announcements')"
        >
          <i class="fa-duotone fa-megaphone"></i> Announcements
        </button>
        <button
          v-for="club in BOOK_CLUBS"
          :key="club.slug"
          :class="{ active: admin.adminTab === `bookclub-${club.slug}` }"
          @click="go(`bookclub-${club.slug}` as AdminTab)"
        >
          <i class="fa-duotone" :class="club.icon"></i> {{ club.name }}
        </button>
      </div>
    </div>

    <!-- Atelier Yao section -->
    <div class="admin-nav-section">
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
          :class="{ active: admin.adminTab === 'atelier-fonts' }"
          @click="go('atelier-fonts')"
        >
          <i class="fa-duotone fa-font"></i> Font Upload
        </button>
        <button
          :class="{ active: admin.adminTab === 'atelier-carrd' }"
          @click="go('atelier-carrd')"
        >
          <i class="fa-duotone fa-images"></i> Carrd Upload
        </button>
      </div>
    </div>

    <!-- System section -->
    <div class="admin-nav-section">
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
          :class="{ active: admin.adminTab === 'system-settings' }"
          @click="go('system-settings')"
        >
          <i class="fa-duotone fa-gear"></i> App Settings
        </button>
        <button
          :class="{ active: admin.adminTab === 'system-themes' }"
          @click="go('system-themes')"
        >
          <i class="fa-duotone fa-palette"></i> Themes
        </button>
      </div>
    </div>
  </nav>
</template>
