/**
 * Vue Router configuration.
 *
 * Replaces the previous store-driven view switching (`ui.view` + `admin.adminTab`)
 * with real, linkable URLs and HTML5 history mode. The route map mirrors the old
 * view/tab surface exactly:
 *
 *   /                         → Home
 *   /play/:cardId             → Player board (loads the board by id)
 *   /raffles                  → Public raffle list
 *   /raffles/:id              → Public raffle detail (loads raffle by id)
 *   /admin/login              → Admin login
 *   /admin                    → redirect → /admin/bingo/game
 *   /admin/bingo/game         → Admin: Current/New Game
 *   /admin/bingo/cards        → Admin: Manage Cards
 *   /admin/bingo/winners-log  → Admin: Winners Log
 *   /admin/bingo/categories   → Admin: Pattern Categories
 *   /admin/bingo/new-pattern  → Admin: New Pattern
 *   /admin/bingo/patterns     → Admin: Edit Patterns
 *   /admin/bingo/presets      → Admin: Game Presets
 *   /admin/raffles/new        → Admin: New/Edit Raffle
 *   /admin/raffles/open       → Admin: Open Raffles
 *   /admin/raffles/closed     → Admin: Closed Raffles
 *   /admin/atelier/fonts      → Admin: Font Upload
 *   /admin/atelier/carrd      → Admin: Carrd Upload
 *   /admin/system/settings    → Admin: App Settings
 *   /admin/system/themes      → Admin: Themes
 *
 * The admin tabs are child routes of the AdminView layout, so the sidebar/topbar
 * persist while the active tab is chosen by the matched child. A global guard
 * enforces admin auth, loads admin data on first entry, and drives the shared
 * WebSocket lifecycle (connect on player/admin, disconnect when leaving).
 */
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import type { AdminTab } from '@/stores/admin'
import { useAuthStore } from '@/stores/auth'
import { useAdminStore } from '@/stores/admin'
import { useUiStore } from '@/stores/ui'
import { BOOK_CLUBS } from '@/lib/constants'

// Views and admin tabs are lazy-loaded (dynamic import) so each route's code —
// and its heavy deps (CodeMirror, vuedraggable, markdown-it) — is split into a
// separate chunk fetched on demand. A player viewing a board never downloads
// the admin/themes editor code. Vite + the manualChunks config keep shared
// vendor code (vue, fontawesome) cached across these route chunks.

// Each admin child route carries the AdminTab id it represents in meta.tab so
// the guard can keep the admin store's adminTab/adminSection in sync (the
// sidebar highlights the active tab off that state).
const adminChildren: RouteRecordRaw[] = [
  { path: '', redirect: { name: 'admin-bingo-game' } },
  {
    path: 'bingo/game',
    name: 'admin-bingo-game',
    component: () => import('@/components/admin/GameTab.vue'),
    meta: { tab: 'bingo-game' },
  },
  {
    path: 'bingo/cards',
    name: 'admin-bingo-cards',
    component: () => import('@/components/admin/CardsTab.vue'),
    meta: { tab: 'bingo-cards' },
  },
  {
    path: 'bingo/winners-log',
    name: 'admin-bingo-winners-log',
    component: () => import('@/components/admin/WinnersLogTab.vue'),
    meta: { tab: 'bingo-winners-log' },
  },
  {
    path: 'bingo/categories',
    name: 'admin-bingo-categories',
    component: () => import('@/components/admin/CategoriesTab.vue'),
    meta: { tab: 'bingo-categories' },
  },
  {
    path: 'bingo/new-pattern',
    name: 'admin-bingo-new-pattern',
    component: () => import('@/components/admin/NewPatternTab.vue'),
    meta: { tab: 'bingo-new-pattern' },
  },
  {
    path: 'bingo/patterns',
    name: 'admin-bingo-patterns',
    component: () => import('@/components/admin/EditPatternsTab.vue'),
    meta: { tab: 'bingo-patterns' },
  },
  {
    path: 'bingo/presets',
    name: 'admin-bingo-presets',
    component: () => import('@/components/admin/PresetsTab.vue'),
    meta: { tab: 'bingo-presets' },
  },
  {
    path: 'raffles/new',
    name: 'admin-raffle-new',
    component: () => import('@/components/admin/RaffleFormTab.vue'),
    meta: { tab: 'raffle-new' },
  },
  {
    path: 'raffles/open',
    name: 'admin-raffle-open',
    component: () => import('@/components/admin/OpenRafflesTab.vue'),
    meta: { tab: 'raffle-open' },
  },
  {
    path: 'raffles/closed',
    name: 'admin-raffle-closed',
    component: () => import('@/components/admin/ClosedRafflesTab.vue'),
    meta: { tab: 'raffle-closed' },
  },
  // One route per registered book club, all served by the generic BookClubTab
  // (the active club drives its labels). Add a club in constants.ts to get its
  // route, sidebar button, and settings webhook field automatically.
  ...BOOK_CLUBS.map(
    (club): RouteRecordRaw => ({
      path: `bookclub/${club.slug}`,
      name: `admin-bookclub-${club.slug}`,
      component: () => import('@/components/admin/BookClubTab.vue'),
      meta: { tab: `bookclub-${club.slug}` as AdminTab },
    }),
  ),
  {
    path: 'system/settings',
    name: 'admin-system-settings',
    component: () => import('@/components/admin/SettingsTab.vue'),
    meta: { tab: 'system-settings' },
  },
  {
    path: 'system/themes',
    name: 'admin-system-themes',
    component: () => import('@/components/admin/ThemesTab.vue'),
    meta: { tab: 'system-themes' },
  },
  {
    path: 'atelier/fonts',
    name: 'admin-atelier-fonts',
    component: () => import('@/components/admin/FontsTab.vue'),
    meta: { tab: 'atelier-fonts' },
  },
  {
    path: 'atelier/carrd',
    name: 'admin-atelier-carrd',
    component: () => import('@/components/admin/CarrdUploadTab.vue'),
    meta: { tab: 'atelier-carrd' },
  },
]

const routes: RouteRecordRaw[] = [
  { path: '/', name: 'home', component: () => import('@/views/HomeView.vue') },
  {
    path: '/play/:cardId',
    name: 'player',
    component: () => import('@/views/PlayerView.vue'),
    props: true,
  },
  { path: '/raffles', name: 'raffles', component: () => import('@/views/RafflesView.vue') },
  {
    path: '/raffles/:id',
    name: 'raffle-detail',
    component: () => import('@/views/RaffleDetailView.vue'),
    props: true,
  },
  {
    path: '/admin/login',
    name: 'admin-login',
    component: () => import('@/views/AdminLoginView.vue'),
  },
  {
    path: '/admin',
    component: () => import('@/views/AdminView.vue'),
    meta: { requiresAdmin: true },
    children: adminChildren,
  },
  // Unknown paths fall back to home.
  { path: '/:pathMatch(.*)*', redirect: { name: 'home' } },
]

export const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

/** Maps an AdminTab id to its route name (for programmatic navigation). */
export function adminTabRouteName(tab: AdminTab): string {
  return 'admin-' + tab
}

// ── Navigation guard ─────────────────────────────────────────────────────────
//
// Enforces admin auth for /admin routes and keeps the admin store's active
// tab/section in sync with the matched route so the sidebar highlights it. Data
// loading for individual admin tabs (and the shared WebSocket lifecycle) is
// handled by the views/composable, not here, to keep the guard lightweight.
// Drive the top progress bar: a navigation may await the async guard below and
// then fetch the matched route's lazy chunk, so show the bar for the whole
// navigation and clear it once it settles (success, redirect, or error).
router.beforeEach(() => {
  useUiStore().setRouteLoading(true)
})
router.afterEach(() => {
  useUiStore().setRouteLoading(false)
})
router.onError(() => {
  useUiStore().setRouteLoading(false)
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()

  if (to.meta.requiresAdmin) {
    // Verify admin auth once per session (server is the source of truth).
    if (!auth.authChecked) {
      await auth.checkAuth()
    }
    if (!auth.isAdmin) {
      return { name: 'admin-login', query: { redirect: to.fullPath } }
    }
  }

  // If already authenticated, skip the login page straight to the dashboard.
  if (to.name === 'admin-login' && auth.authChecked && auth.isAdmin) {
    return { name: 'admin-bingo-game' }
  }

  // Sync the admin store's tab/section from the matched child route's meta.
  const tab = to.meta.tab as AdminTab | undefined
  if (tab) {
    const admin = useAdminStore()
    admin.setTabFromRoute(tab)
  }

  return true
})
