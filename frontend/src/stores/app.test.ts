import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// The app store imports the endpoints layer at module load; stub it so the store
// can be instantiated without a real API. activeCss/publicCss are spies so the
// theme-preference tests can assert which one was fetched.
const { activeCss, publicCss, settingsGet } = vi.hoisted(() => ({
  settingsGet: vi.fn(async () => ({ settings: {}, uploaded_fonts: [] })),
  activeCss: vi.fn(async () => ({
    css: ':root{--t:active}',
    board_flourish: '',
    number_flourish: '',
  })),
  publicCss: vi.fn(async () => ({
    css: ':root{--t:public}',
    board_flourish: '',
    number_flourish: '',
  })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { styles: { activeCss, publicCss }, settings: { get: settingsGet } },
}))

import { useAppStore } from './app'

/** The CSS text currently injected by applyCustomCSS (theme.ts), or '' if none. */
function injectedThemeCss(): string {
  return document.getElementById('bingo-custom-theme')?.textContent ?? ''
}

beforeEach(() => {
  setActivePinia(createPinia())
  localStorage.clear()
  activeCss.mockClear()
  publicCss.mockClear()
  publicCss.mockResolvedValue({ css: ':root{--t:public}', board_flourish: '', number_flourish: '' })
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
    publicCss.mockRejectedValueOnce(new Error('404'))
    await app.applyThemePreference()
    // Reverted to Default: preference reset + the active theme applied.
    expect(app.themePreference).toBe('default')
    expect(localStorage.getItem('bingo_theme')).toBe('default')
    expect(activeCss).toHaveBeenCalled()
    expect(injectedThemeCss()).toBe(':root{--t:active}')
  })
})

describe('hideBingo', () => {
  beforeEach(() => {
    settingsGet.mockClear()
    settingsGet.mockResolvedValue({ settings: {}, uploaded_fonts: [] })
  })

  it('is off until the server says otherwise', () => {
    const app = useAppStore()
    expect(app.hideBingo).toBe(false)
    // Nothing has been read yet, so the home page must not act on the default.
    expect(app.settingsLoaded).toBe(false)
  })

  it("is on only for the exact flag value '1'", async () => {
    const app = useAppStore()
    settingsGet.mockResolvedValue({ settings: { hide_bingo: '1' }, uploaded_fonts: [] })
    await app.loadSettings()
    expect(app.hideBingo).toBe(true)
    expect(app.settingsLoaded).toBe(true)
  })

  it('treats any other stored value as off, never hiding the main feature by accident', async () => {
    const app = useAppStore()
    for (const val of ['0', 'true', 'yes', '', '2']) {
      settingsGet.mockResolvedValue({ settings: { hide_bingo: val }, uploaded_fonts: [] })
      await app.loadSettings()
      expect(app.hideBingo, `hide_bingo=${JSON.stringify(val)}`).toBe(false)
    }
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
