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
        <span><i class="fa-solid fa-circle-dot"></i> Bingo</span>
        <span class="nav-chevron">{{ admin.adminSection === 'bingo' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'bingo'" class="admin-nav-items">
        <button :class="{ active: admin.adminTab === 'bingo-game' }" @click="go('bingo-game')">
          <i class="fa-solid fa-gamepad"></i> {{ game.adminGameLabel }}
          <span
            v-if="game.currentGame"
            class="live-dot nav-live-dot"
            role="status"
            aria-label="Game in progress"
          ></span>
        </button>
        <button :class="{ active: admin.adminTab === 'bingo-cards' }" @click="go('bingo-cards')">
          <i class="fa-solid fa-id-card"></i> Manage Cards
          <span v-if="cards.cards.length" class="nav-count">({{ cards.cards.length }})</span>
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-winners-log' }"
          @click="go('bingo-winners-log')"
        >
          <i class="fa-solid fa-trophy"></i> Winners Log
        </button>
        <span class="admin-nav-sub-header">Patterns</span>
        <button
          :class="{ active: admin.adminTab === 'bingo-categories' }"
          @click="go('bingo-categories')"
        >
          <i class="fa-solid fa-folder-open"></i> Pattern Categories
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-new-pattern' }"
          @click="go('bingo-new-pattern')"
        >
          <i class="fa-solid fa-plus"></i> New Pattern
        </button>
        <button
          :class="{ active: admin.adminTab === 'bingo-patterns' }"
          @click="go('bingo-patterns')"
        >
          <i class="fa-solid fa-pen-to-square"></i> Edit Patterns
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
        <span><i class="fa-solid fa-ticket"></i> Raffles</span>
        <span class="nav-chevron">{{ admin.adminSection === 'raffles' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'raffles'" class="admin-nav-items">
        <button :class="{ active: admin.adminTab === 'raffle-new' }" @click="go('raffle-new')">
          <i class="fa-solid fa-plus"></i> New Raffle
        </button>
        <button :class="{ active: admin.adminTab === 'raffle-open' }" @click="go('raffle-open')">
          <i class="fa-solid fa-clipboard-list"></i> Open Raffles
          <span v-if="raffles.openRaffles.length" class="nav-count">
            ({{ raffles.openRaffles.length }})
          </span>
        </button>
        <button
          :class="{ active: admin.adminTab === 'raffle-closed' }"
          @click="go('raffle-closed')"
        >
          <i class="fa-solid fa-lock"></i> Closed Raffles
          <span v-if="raffles.closedRaffles.length" class="nav-count">
            ({{ raffles.closedRaffles.length }})
          </span>
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
        <span><i class="fa-solid fa-gear"></i> System</span>
        <span class="nav-chevron">{{ admin.adminSection === 'system' ? '▾' : '▸' }}</span>
      </div>
      <div v-show="admin.adminSection === 'system'" class="admin-nav-items">
        <button
          :class="{ active: admin.adminTab === 'system-settings' }"
          @click="go('system-settings')"
        >
          <i class="fa-solid fa-gear"></i> App Settings
        </button>
        <button
          :class="{ active: admin.adminTab === 'system-themes' }"
          @click="go('system-themes')"
        >
          <i class="fa-solid fa-palette"></i> Themes
        </button>
        <button
          :class="{ active: admin.adminTab === 'system-fonts' }"
          @click="go('system-fonts')"
        >
          <i class="fa-solid fa-font"></i> Font Upload
        </button>
      </div>
    </div>
  </nav>
</template>
