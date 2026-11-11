import { describe, it, expect, beforeEach, beforeAll, afterAll, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

import { useAdminStore } from '@/stores/admin'
import { useGaraponsStore } from '@/stores/garapons'
import { useRafflesStore } from '@/stores/raffles'
import { usePresetsStore } from '@/stores/presets'
import { useUsersStore } from '@/stores/users'
import { useGameStore } from '@/stores/game'
import { useAnnouncementsStore } from '@/stores/announcements'
import { useFontsStore } from '@/stores/fonts'
import { useCarrdStore } from '@/stores/carrd'
import { useImagesStore } from '@/stores/images'
import { useStylesStore } from '@/stores/styles'
import { useAppStore } from '@/stores/app'
import { usePatternsStore } from '@/stores/patterns'
import { useBookclubStore } from '@/stores/bookclub'

// Replace every loader the admin store can trigger with a no-op spy, so neither
// setTabFromRoute nor refreshResource touches the network. Returns the stubbed
// stores for per-test assertions.
function stubLoaders() {
  const garapons = useGaraponsStore()
  garapons.loadGarapons = vi.fn()
  garapons.loadGaraponDetail = vi.fn(async () => undefined)
  const raffles = useRafflesStore()
  raffles.loadRaffles = vi.fn()
  raffles.loadRaffleDetail = vi.fn(async () => undefined)
  const presets = usePresetsStore()
  presets.loadPresets = vi.fn()
  const users = useUsersStore()
  users.loadUsers = vi.fn()
  const game = useGameStore()
  game.loadWinnersLog = vi.fn()
  const announcements = useAnnouncementsStore()
  announcements.load = vi.fn()
  const fonts = useFontsStore()
  fonts.loadFonts = vi.fn()
  const carrd = useCarrdStore()
  carrd.loadProjects = vi.fn()
  const images = useImagesStore()
  images.loadCategories = vi.fn()
  const styles = useStylesStore()
  styles.loadStyles = vi.fn()
  const app = useAppStore()
  app.loadSettings = vi.fn()
  const patterns = usePatternsStore()
  patterns.loadPatterns = vi.fn()
  const bookclub = useBookclubStore()
  bookclub.openClub = vi.fn()
  bookclub.applyExternalChange = vi.fn()
  return {
    garapons,
    raffles,
    presets,
    users,
    game,
    announcements,
    fonts,
    carrd,
    images,
    styles,
    app,
    patterns,
    bookclub,
  }
}

// The per-tab freshness gate (createFreshness) is module-scoped and Date.now()-
// based, so it survives Pinia resets between tests. Run under fake timers and
// step the clock forward past the 30s TTL before each test, so stamps written by
// a prior test read as stale here while within-test freshness still holds.
beforeAll(() => vi.useFakeTimers())
afterAll(() => vi.useRealTimers())

beforeEach(() => {
  vi.advanceTimersByTime(120_000)
  setActivePinia(createPinia())
})

describe('setTabFromRoute (section highlight + per-tab load)', () => {
  const cases: { tab: string; section: string }[] = [
    { tab: 'bingo-game', section: 'bingo' },
    { tab: 'teahouse-raffles', section: 'festival' }, // raffles sit under Festival
    { tab: 'festival-garapon', section: 'festival' },
    { tab: 'teahouse-announcements', section: 'teahouse' },
    { tab: 'bookclub-yaoi', section: 'teahouse' },
    { tab: 'atelier-fonts', section: 'atelier' },
    { tab: 'system-users', section: 'system' },
  ]
  for (const { tab, section } of cases) {
    it(`maps ${tab} -> ${section}`, () => {
      stubLoaders()
      const admin = useAdminStore()
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- the table holds plain strings; the store takes the AdminTab union
      admin.setTabFromRoute(tab as any)
      expect(admin.adminTab).toBe(tab)
      expect(admin.adminSection).toBe(section)
    })
  }

  it('loads garapons (once) when entering the garapon tab', () => {
    const { garapons } = stubLoaders()
    const admin = useAdminStore()
    admin.setTabFromRoute('festival-garapon')
    expect(garapons.loadGarapons).toHaveBeenCalledTimes(1)
    // Re-entering within the freshness TTL does not refetch.
    admin.setTabFromRoute('festival-garapon')
    expect(garapons.loadGarapons).toHaveBeenCalledTimes(1)
  })

  it('loads presets on both the game and presets tabs', () => {
    const { presets } = stubLoaders()
    const admin = useAdminStore()
    admin.setTabFromRoute('bingo-presets')
    expect(presets.loadPresets).toHaveBeenCalledTimes(1)
  })
})

describe('refreshResource (live invalidation)', () => {
  it('reloads garapons (list + open detail) when viewing the garapon tab', () => {
    const { garapons } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'festival-garapon'
    garapons.selectedGarapon = { id: 42 } as never
    admin.refreshResource('garapons')
    expect(garapons.loadGarapons).toHaveBeenCalled()
    expect(garapons.loadGaraponDetail).toHaveBeenCalledWith(42)
  })

  it('does not reload garapons when on a different tab', () => {
    const { garapons } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'system-users'
    admin.refreshResource('garapons')
    expect(garapons.loadGarapons).not.toHaveBeenCalled()
  })

  it('reloads raffles (list + open detail) when viewing the raffle tab', () => {
    const { raffles } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'teahouse-raffles'
    raffles.selectedRaffle = { id: 7 } as never
    admin.refreshResource('raffles')
    expect(raffles.loadRaffles).toHaveBeenCalled()
    expect(raffles.loadRaffleDetail).toHaveBeenCalledWith(7)
  })

  it('reloads raffles list but not detail when none is open', () => {
    const { raffles } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'teahouse-raffles'
    raffles.selectedRaffle = null
    admin.refreshResource('raffles')
    expect(raffles.loadRaffles).toHaveBeenCalled()
    expect(raffles.loadRaffleDetail).not.toHaveBeenCalled()
  })

  it('reloads presets when on the game tab (shared resource)', () => {
    const { presets } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'bingo-game'
    admin.refreshResource('presets')
    expect(presets.loadPresets).toHaveBeenCalled()
  })

  it('reloads users only when viewing the users tab', () => {
    const { users } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'system-users'
    admin.refreshResource('users')
    expect(users.loadUsers).toHaveBeenCalled()
  })

  it("routes a club's change through the bookclub store, viewing it or not", () => {
    const { bookclub } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'bookclub-yaoi'
    admin.refreshResource('bookclub-yaoi')
    expect(bookclub.applyExternalChange).toHaveBeenCalledWith('yaoi', true)
    admin.refreshResource('bookclub-yuri')
    expect(bookclub.applyExternalChange).toHaveBeenCalledWith('yuri', false)
  })

  it('reloads the themes list when viewing the themes tab', () => {
    const { styles } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'system-themes'
    admin.refreshResource('themes')
    expect(styles.loadStyles).toHaveBeenCalled()
  })

  it('ignores an unknown resource without throwing', () => {
    stubLoaders()
    const admin = useAdminStore()
    expect(() => admin.refreshResource('nonsense')).not.toThrow()
  })
})

describe('refreshOpenTab (the live stream opened)', () => {
  it('reads the open tab again', () => {
    const { raffles } = stubLoaders()
    const admin = useAdminStore()
    admin.adminTab = 'teahouse-raffles'
    admin.refreshOpenTab()
    expect(raffles.loadRaffles).toHaveBeenCalledTimes(1)
  })

  it("reads the settings, and an admin's integrations, on the settings tab", () => {
    const { app } = stubLoaders()
    app.loadIntegrations = vi.fn()
    const admin = useAdminStore()
    admin.adminTab = 'system-settings'
    admin.refreshOpenTab()
    expect(app.loadSettings).toHaveBeenCalled()
    expect(app.loadIntegrations).not.toHaveBeenCalled()
  })

  it('makes every other tab read afresh when next entered', () => {
    const { garapons } = stubLoaders()
    const admin = useAdminStore()
    admin.setTabFromRoute('festival-garapon')
    admin.setTabFromRoute('teahouse-raffles')
    admin.refreshOpenTab()
    admin.setTabFromRoute('festival-garapon')
    expect(garapons.loadGarapons).toHaveBeenCalledTimes(2)
  })
})
