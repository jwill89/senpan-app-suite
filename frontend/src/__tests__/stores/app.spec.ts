import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Integrations, Settings, SettingsInput } from '@/api/client'
import { DEFAULT_SETTINGS } from '@/lib/constants'

// The store reads and saves through the API client; only its calls are faked
// (unwrap is the real one). Each fake answers a call's `{ data, response }`.
const ok = (data: unknown) => ({ data, response: new Response(null) })
const theme = (css: string) => ok({ css, boardFlourish: '', numberFlourish: '' })
const settings = (over: Partial<Settings> = {}): Settings => ({
  ...DEFAULT_SETTINGS,
  uploadedFonts: [],
  ...over,
})
const { activeCss, publicCss, settingsGet, settingsPut, integrationsPut } = vi.hoisted(() => ({
  settingsGet: vi.fn(),
  settingsPut: vi.fn(),
  integrationsPut: vi.fn(),
  activeCss: vi.fn(),
  publicCss: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api: {
    GET: (path: string, init?: { params: { path: { id: number } } }) =>
      path === '/settings'
        ? settingsGet()
        : path === '/styles/active'
          ? activeCss()
          : publicCss(init?.params.path.id),
    PUT: (path: string, init: { body: unknown }) =>
      path === '/settings' ? settingsPut(init.body) : integrationsPut(init.body),
  },
}))

import { useAppStore } from '@/stores/app'
import { useUiStore } from '@/stores/ui'

/** The CSS text currently injected by applyCustomCSS (theme.ts), or '' if none. */
function injectedThemeCss(): string {
  return document.getElementById('bingo-custom-theme')?.textContent ?? ''
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  for (const fn of [activeCss, publicCss, settingsGet, settingsPut, integrationsPut]) {
    fn.mockReset()
  }
  activeCss.mockResolvedValue(theme(':root{--t:active}'))
  publicCss.mockResolvedValue(theme(':root{--t:public}'))
  settingsGet.mockResolvedValue(ok(settings()))
})
afterEach(() => {
  document.documentElement.style.removeProperty('--number-flourish-url')
  document.getElementById('bingo-custom-theme')?.remove()
})

describe('app applyFlourishes', () => {
  it('stores both flourishes and applies the number-flourish CSS variable', () => {
    const app = useAppStore()
    app.applyFlourishes('images/flourishes/board.svg', 'images/flourishes/num.svg')
    expect(app.activeBoardFlourish).toBe('images/flourishes/board.svg')
    expect(app.activeNumberFlourish).toBe('images/flourishes/num.svg')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe(
      'url("/images/flourishes/num.svg")',
    )
  })

  it('clears the refs and the CSS variable when given empty values', () => {
    const app = useAppStore()
    app.applyFlourishes('images/flourishes/board.svg', 'images/flourishes/num.svg')
    app.applyFlourishes('', '')
    expect(app.activeBoardFlourish).toBe('')
    expect(app.activeNumberFlourish).toBe('')
    expect(document.documentElement.style.getPropertyValue('--number-flourish-url')).toBe('')
  })
})

describe('app theme preference', () => {
  it('defaults to "default" and applies the admin active theme', async () => {
    const app = useAppStore()
    expect(app.themePreference).toBe('default')
    await app.applyThemePreference()
    expect(activeCss).toHaveBeenCalled()
    expect(publicCss).not.toHaveBeenCalled()
    expect(injectedThemeCss()).toBe(':root{--t:active}')
  })

  it('initialises from a persisted preference', () => {
    localStorage.setItem('bingo_theme', '7')
    const app = useAppStore()
    expect(app.themePreference).toBe('7')
  })

  it('setThemePreference persists a public id and applies that theme', async () => {
    const app = useAppStore()
    await app.setThemePreference('5')
    expect(app.themePreference).toBe('5')
    expect(localStorage.getItem('bingo_theme')).toBe('5')
    expect(publicCss).toHaveBeenCalledWith(5)
    expect(injectedThemeCss()).toBe(':root{--t:public}')
  })

  it('setThemePreference("default") clears the choice and follows the active theme', async () => {
    const app = useAppStore()
    await app.setThemePreference('5')
    await app.setThemePreference('default')
    expect(localStorage.getItem('bingo_theme')).toBe('default')
    expect(activeCss).toHaveBeenCalled()
    expect(injectedThemeCss()).toBe(':root{--t:active}')
  })

  it('falls back to Default when the chosen theme is no longer public (404)', async () => {
    localStorage.setItem('bingo_theme', '9')
    const app = useAppStore()
    publicCss.mockResolvedValueOnce({
      error: { status: 404 },
      response: new Response(null, { status: 404 }),
    })
    await app.applyThemePreference()
    // Reverted to Default: preference reset + the active theme applied.
    expect(app.themePreference).toBe('default')
    expect(localStorage.getItem('bingo_theme')).toBe('default')
    expect(activeCss).toHaveBeenCalled()
    expect(injectedThemeCss()).toBe(':root{--t:active}')
  })
})

describe('hideBingo', () => {
  it('is off until the server says otherwise', () => {
    const app = useAppStore()
    expect(app.hideBingo).toBe(false)
    // Nothing has been read yet, so the home page must not act on the default.
    expect(app.settingsLoaded).toBe(false)
  })

  it('follows the server', async () => {
    const app = useAppStore()
    settingsGet.mockResolvedValue(ok(settings({ hideBingo: true })))
    await app.loadSettings()
    expect(app.hideBingo).toBe(true)
    expect(app.settingsLoaded).toBe(true)
  })

  it('reports settings as loaded even when the read fails', async () => {
    const app = useAppStore()
    settingsGet.mockRejectedValueOnce(new Error('offline'))
    await app.loadSettings()
    // Otherwise the home page would wait forever and show nothing at all.
    expect(app.settingsLoaded).toBe(true)
    expect(app.hideBingo).toBe(false)
  })
})

describe('hideCustomCards', () => {
  it('is off until the server says otherwise', () => {
    const app = useAppStore()
    expect(app.hideCustomCards).toBe(false)
    expect(app.settingsLoaded).toBe(false)
  })

  it('is independent of hideBingo, which is the wider switch', async () => {
    const app = useAppStore()
    // Custom cards off on their own: bingo stays up, so the home page keeps Join
    // Bingo and drops only the request row.
    settingsGet.mockResolvedValue(ok(settings({ hideCustomCards: true })))
    await app.loadSettings()
    expect(app.hideCustomCards).toBe(true)
    expect(app.hideBingo).toBe(false)

    // Hiding bingo does not set this flag - the home page is what combines them.
    settingsGet.mockResolvedValue(ok(settings({ hideBingo: true })))
    await app.loadSettings()
    expect(app.hideCustomCards).toBe(false)
    expect(app.hideBingo).toBe(true)
  })
})

describe('loadSettings', () => {
  it('keeps the settings and the uploaded fonts apart', async () => {
    const app = useAppStore()
    const font = { name: 'Fancy', family: 'Fancy', token: 't1' }
    settingsGet.mockResolvedValue(
      ok(
        settings({
          appTitle: 'Tea Time',
          updatedAt: '2026-11-09T10:00:00Z',
          uploadedFonts: [font],
        }),
      ),
    )
    await app.loadSettings()
    expect(app.settings.appTitle).toBe('Tea Time')
    expect(app.settings.updatedAt).toBe('2026-11-09T10:00:00Z')
    expect(app.settings).not.toHaveProperty('uploadedFonts')
    expect(app.uploadedFonts).toEqual([font])
    expect(document.title).toBe('Tea Time')
  })
})

describe('saveSettings', () => {
  const keys: Integrations = { googleFontsApiKey: 'key' }

  it('saves the settings alone and keeps what the server saved', async () => {
    const app = useAppStore()
    app.settings.appTitle = 'Renamed'
    settingsPut.mockImplementation(async (body: SettingsInput) =>
      ok(settings({ ...body, updatedAt: '2026-11-09T11:00:00Z' })),
    )
    await app.saveSettings()
    expect(settingsPut).toHaveBeenCalledWith(expect.objectContaining({ appTitle: 'Renamed' }))
    expect(integrationsPut).not.toHaveBeenCalled()
    // The new updatedAt is what the next save is checked against.
    expect(app.settings.updatedAt).toBe('2026-11-09T11:00:00Z')
  })

  it("saves an admin's integrations too", async () => {
    const app = useAppStore()
    app.integrations = keys
    settingsPut.mockResolvedValue(ok(settings()))
    integrationsPut.mockResolvedValue(ok(keys))
    await app.saveSettings(true)
    expect(integrationsPut).toHaveBeenCalledWith(keys)
  })

  it("shows the server's reason when a save is refused", async () => {
    const app = useAppStore()
    const ui = useUiStore()
    const notify = vi.spyOn(ui, 'notify')
    settingsPut.mockResolvedValue({
      error: {
        status: 409,
        title: 'Conflict',
        detail: 'The settings changed since this page read them.',
      },
      response: new Response(null, { status: 409 }),
    })
    await app.saveSettings(true)
    expect(notify).toHaveBeenCalledWith('The settings changed since this page read them.', 'error')
    expect(integrationsPut).not.toHaveBeenCalled()
    expect(app.savingSettings).toBe(false)
  })
})
