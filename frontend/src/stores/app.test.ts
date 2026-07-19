import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'

// The app store imports the endpoints layer at module load; stub it so the store
// can be instantiated without a real API. activeCss/publicCss are spies so the
// theme-preference tests can assert which one was fetched.
const { activeCss, publicCss } = vi.hoisted(() => ({
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
  endpoints: { styles: { activeCss, publicCss }, settings: {} },
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
