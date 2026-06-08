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
import { useStylesStore } from './styles'
import { useAppStore } from './app'
import { useFontsStore } from './fonts'

export type AdminSection = 'bingo' | 'raffles' | 'system'
export type AdminTab =
  | 'bingo-game'
  | 'bingo-cards'
  | 'bingo-winners-log'
  | 'bingo-categories'
  | 'bingo-new-pattern'
  | 'bingo-patterns'
  | 'raffle-new'
  | 'raffle-open'
  | 'raffle-closed'
  | 'system-settings'
  | 'system-themes'
  | 'system-fonts'

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
    else if (tab.startsWith('system-')) adminSection.value = 'system'
    adminTab.value = tab

    const game = useGameStore()
    const raffles = useRafflesStore()
    const styles = useStylesStore()
    const app = useAppStore()
    const fonts = useFontsStore()

    if (tab === 'raffle-open' || tab === 'raffle-closed') {
      raffles.selectedRaffle = null
      raffles.loadRaffles()
    }
    if (tab === 'system-themes') styles.loadStyles()
    if (tab === 'system-settings') app.loadSettings()
    if (tab === 'system-fonts') fonts.loadFonts()
    if (tab === 'bingo-winners-log') game.loadWinnersLog()
    if (tab === 'raffle-new' && !raffles.raffleForm) raffles.newRaffleForm()
  }

  return { adminTab, adminSection, setTabFromRoute }
})
