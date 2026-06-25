/**
 * Admin navigation store: tracks the active tab + open section (for sidebar
 * highlighting) and triggers the relevant per-tab data loads.
 *
 * Navigation itself is now handled by Vue Router (`/admin/...` routes); the
 * router guard calls `setTabFromRoute()` whenever an admin child route is
 * matched, which both updates the highlight state and performs the data load
 * that the old `adminNav()` used to do as a side effect.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { useGameStore } from './game'
import { useRafflesStore } from './raffles'
import { useGaraponsStore } from './garapons'
import { useBookclubStore } from './bookclub'
import { useStylesStore } from './styles'
import { useAppStore } from './app'
import { useFontsStore } from './fonts'
import { useCarrdStore } from './carrd'
import { usePresetsStore } from './presets'
import { usePatternsStore } from './patterns'
import { useAnnouncementsStore } from './announcements'
import { useImagesStore } from './images'
import { useUsersStore } from './users'
import { BOOK_CLUBS } from '@/lib/constants'
import { createFreshness } from '@/lib/freshness'

export type AdminSection = 'bingo' | 'teahouse' | 'festival' | 'atelier' | 'system'

/** One tab per registered book club, e.g. 'bookclub-yaoi' | 'bookclub-yuri'. */
export type BookClubTab = `bookclub-${(typeof BOOK_CLUBS)[number]['slug']}`

export type AdminTab =
  | 'bingo-game'
  | 'bingo-cards'
  | 'bingo-winners-log'
  | 'bingo-patterns'
  | 'bingo-presets'
  | 'teahouse-announcements'
  | 'teahouse-raffles'
  | BookClubTab
  | 'festival-garapon'
  | 'atelier-fonts'
  | 'atelier-carrd'
  | 'system-settings'
  | 'system-themes'
  | 'system-images'
  | 'system-users'

// Per-dataset freshness gate so re-entering a tab doesn't re-spin and re-fetch
// data we just loaded. Keyed by data domain (several tabs can share one key, e.g.
// the open/closed raffle tabs, or the game/presets tabs). Mutations refresh by
// calling the store loaders directly (bypassing this), so edits still show at
// once; the live game/cards/patterns also stay current over WebSocket.
const tabData = createFreshness(30_000)

export const useAdminStore = defineStore('admin', () => {
  const adminTab = ref<AdminTab>('bingo-game')
  const adminSection = ref<AdminSection>('bingo')

  /** Runs `load` unless `key`'s data was already loaded within the freshness TTL. */
  function loadFresh(key: string, load: () => void): void {
    if (!tabData.isStale(key)) return
    tabData.touch(key)
    load()
  }

  /**
   * Called by the router guard when an admin child route is matched. Updates
   * the active tab + section (for sidebar highlight) and runs the per-tab data
   * load that the old `adminNav()` performed.
   */
  function setTabFromRoute(tab: AdminTab): void {
    if (tab.startsWith('bingo-')) adminSection.value = 'bingo'
    // The Senpan Tea House section holds Announcement Management + Raffles
    // (teahouse-*) and the book clubs (bookclub-*).
    else if (tab.startsWith('teahouse-') || tab.startsWith('bookclub-'))
      adminSection.value = 'teahouse'
    else if (tab.startsWith('festival-')) adminSection.value = 'festival'
    else if (tab.startsWith('atelier-')) adminSection.value = 'atelier'
    else if (tab.startsWith('system-')) adminSection.value = 'system'
    adminTab.value = tab

    const game = useGameStore()
    const raffles = useRafflesStore()
    const garapons = useGaraponsStore()
    const bookclub = useBookclubStore()
    const styles = useStylesStore()
    const app = useAppStore()
    const fonts = useFontsStore()
    const carrd = useCarrdStore()
    const presets = usePresetsStore()
    const patterns = usePatternsStore()
    const announcements = useAnnouncementsStore()
    const images = useImagesStore()
    const users = useUsersStore()

    if (tab === 'teahouse-raffles') {
      raffles.selectedRaffle = null
      loadFresh('raffles', () => raffles.loadRaffles())
    }
    if (tab === 'festival-garapon') {
      garapons.selectedGarapon = null
      garapons.garaponForm = null
      loadFresh('garapons', () => garapons.loadGarapons())
    }
    // openClub manages its own per-club freshness (and preserves the open list /
    // events sub-view when the same club tab is re-entered).
    if (tab.startsWith('bookclub-')) {
      bookclub.openClub(tab.slice('bookclub-'.length))
    }
    if (tab === 'teahouse-announcements') loadFresh('announcements', () => announcements.load())
    if (tab === 'system-themes') loadFresh('styles', () => styles.loadStyles())
    if (tab === 'system-settings') loadFresh('settings', () => app.loadSettings())
    if (tab === 'system-images') loadFresh('images', () => images.loadCategories())
    if (tab === 'system-users') loadFresh('users', () => users.loadUsers())
    if (tab === 'atelier-fonts') loadFresh('fonts', () => fonts.loadFonts())
    if (tab === 'atelier-carrd') loadFresh('carrd', () => carrd.loadProjects())
    if (tab === 'bingo-winners-log') loadFresh('winners-log', () => game.loadWinnersLog())
    if (tab === 'bingo-patterns') loadFresh('patterns', () => patterns.loadPatterns())
    // Presets are needed on the Game tab (to start from one) and the Presets tab.
    if (tab === 'bingo-game' || tab === 'bingo-presets')
      loadFresh('presets', () => presets.loadPresets())
  }

  return { adminTab, adminSection, setTabFromRoute }
})
