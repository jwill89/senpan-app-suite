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
import { useBookclubStore } from './bookclub'
import { useStylesStore } from './styles'
import { useAppStore } from './app'
import { useFontsStore } from './fonts'
import { useCarrdStore } from './carrd'
import { usePresetsStore } from './presets'
import { useAnnouncementsStore } from './announcements'
import { BOOK_CLUBS } from '@/lib/constants'

export type AdminSection = 'bingo' | 'raffles' | 'teahouse' | 'atelier' | 'system'

/** One tab per registered book club, e.g. 'bookclub-yaoi' | 'bookclub-yuri'. */
export type BookClubTab = `bookclub-${(typeof BOOK_CLUBS)[number]['slug']}`

export type AdminTab =
  | 'bingo-game'
  | 'bingo-cards'
  | 'bingo-winners-log'
  | 'bingo-categories'
  | 'bingo-new-pattern'
  | 'bingo-patterns'
  | 'bingo-presets'
  | 'raffle-new'
  | 'raffle-open'
  | 'raffle-closed'
  | 'teahouse-announcements'
  | BookClubTab
  | 'atelier-fonts'
  | 'atelier-carrd'
  | 'system-settings'
  | 'system-themes'

export const useAdminStore = defineStore('admin', () => {
  const adminTab = ref<AdminTab>('bingo-game')
  const adminSection = ref<AdminSection>('bingo')

  /**
   * Called by the router guard when an admin child route is matched. Updates
   * the active tab + section (for sidebar highlight) and runs the per-tab data
   * load that the old `adminNav()` performed.
   */
  function setTabFromRoute(tab: AdminTab): void {
    if (tab.startsWith('bingo-')) adminSection.value = 'bingo'
    else if (tab.startsWith('raffle-')) adminSection.value = 'raffles'
    // The Senpan Tea House section holds both Announcement Management
    // (teahouse-*) and the book clubs (bookclub-*).
    else if (tab.startsWith('teahouse-') || tab.startsWith('bookclub-'))
      adminSection.value = 'teahouse'
    else if (tab.startsWith('atelier-')) adminSection.value = 'atelier'
    else if (tab.startsWith('system-')) adminSection.value = 'system'
    adminTab.value = tab

    const game = useGameStore()
    const raffles = useRafflesStore()
    const bookclub = useBookclubStore()
    const styles = useStylesStore()
    const app = useAppStore()
    const fonts = useFontsStore()
    const carrd = useCarrdStore()
    const presets = usePresetsStore()
    const announcements = useAnnouncementsStore()

    if (tab === 'raffle-open' || tab === 'raffle-closed') {
      raffles.selectedRaffle = null
      raffles.loadRaffles()
    }
    if (tab.startsWith('bookclub-')) {
      bookclub.openClub(tab.slice('bookclub-'.length))
    }
    if (tab === 'teahouse-announcements') announcements.load()
    if (tab === 'system-themes') styles.loadStyles()
    if (tab === 'system-settings') app.loadSettings()
    if (tab === 'atelier-fonts') fonts.loadFonts()
    if (tab === 'atelier-carrd') carrd.loadProjects()
    if (tab === 'bingo-winners-log') game.loadWinnersLog()
    // Presets are needed on the Game tab (to start from one) and the Presets tab.
    if (tab === 'bingo-game' || tab === 'bingo-presets') presets.loadPresets()
    if (tab === 'raffle-new' && !raffles.raffleForm) raffles.newRaffleForm()
  }

  return { adminTab, adminSection, setTabFromRoute }
})
