/**
 * App store: global app settings, Google Fonts list, and active theme CSS.
 *
 * Mirrors the original app.js settings logic (loadSettings/saveSettings,
 * loadGoogleFontsList, _applyHeaderFont, _loadActiveCSS/_applyCustomCSS).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import {
  applyCustomCSS,
  applyHeaderFont,
  applyUploadedFonts,
  applyNumberFlourish,
} from '@/lib/theme'
import { DEFAULT_APP_SETTINGS } from '@/lib/constants'
import type { AppSettings, UploadedFont } from '@/types/api'
import { useUiStore } from './ui'

export const useAppStore = defineStore('app', () => {
  const ui = useUiStore()

  const settings = ref<AppSettings>({ ...DEFAULT_APP_SETTINGS })
  const googleFontsList = ref<string[]>([])
  /** Fonts uploaded via System → Font Upload (name + serving token). */
  const uploadedFonts = ref<UploadedFont[]>([])
  let googleFontsCacheKey = ''
  /** True while settings are being saved (drives the Save button). */
  const savingSettings = ref(false)

  /**
   * Active theme decorative flourishes (root-relative paths into
   * images/flourishes, "" = built-in art). The number flourish is applied as a
   * CSS variable; the board flourish is read by CornerFlourish to render the
   * player board corners. Kept here (app-wide, loaded on every view) so both the
   * public player board and the admin views stay in sync.
   */
  const activeBoardFlourish = ref('')
  const activeNumberFlourish = ref('')

  /** Sets the active flourishes + applies the number-flourish CSS variable. */
  function applyFlourishes(board: string, number: string): void {
    activeBoardFlourish.value = board || ''
    activeNumberFlourish.value = number || ''
    applyNumberFlourish(activeNumberFlourish.value)
  }

  /** Loads app settings, applies title + header font, then fetches fonts. */
  async function loadSettings(): Promise<void> {
    try {
      const data = await endpoints.settings.get()
      // Register uploaded fonts first so applyHeaderFont knows to skip Google
      // for an uploaded family.
      uploadedFonts.value = data.uploaded_fonts || []
      applyUploadedFonts(uploadedFonts.value)
      settings.value = { ...settings.value, ...data.settings }
      document.title = settings.value.app_title || 'Senpan App Suite'
      applyHeaderFont(settings.value.header_font)
      void loadGoogleFontsList()
    } catch {
      /* silent */
    }
  }

  /** Saves settings to the server, applies them, and notifies. */
  async function saveSettings(): Promise<void> {
    savingSettings.value = true
    try {
      // The settings API is a string→string map, but a number <input> bound with
      // v-model yields a *number* for any field the admin edited — and a numeric
      // JSON value fails the backend's map[string]string decode ("Invalid JSON").
      // Coerce every value back to a string before sending. (The AppSettings type
      // says the values are already strings, hence the `unknown` view to allow it.)
      const payload = Object.fromEntries(
        Object.entries(settings.value as Record<string, string | number>).map(([k, v]) => [
          k,
          String(v),
        ]),
      ) as unknown as AppSettings
      await endpoints.settings.save(payload)
      document.title = settings.value.app_title || 'Senpan App Suite'
      applyHeaderFont(settings.value.header_font)
      void loadGoogleFontsList()
      ui.notify('Settings saved!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingSettings.value = false
    }
  }

  /** Fetches the Google Fonts list if an API key is configured (cached). */
  async function loadGoogleFontsList(): Promise<void> {
    const key = (settings.value.google_fonts_api_key || '').trim()
    if (!key) {
      googleFontsList.value = []
      googleFontsCacheKey = ''
      return
    }
    if (key === googleFontsCacheKey && googleFontsList.value.length > 0) return
    try {
      const resp = await fetch(
        `https://www.googleapis.com/webfonts/v1/webfonts?key=${encodeURIComponent(key)}&sort=popularity`,
      )
      if (!resp.ok) {
        googleFontsList.value = []
        return
      }
      const data = await resp.json()
      googleFontsList.value = (data.items || []).map((f: { family: string }) => f.family)
      googleFontsCacheKey = key
    } catch {
      googleFontsList.value = []
    }
  }

  /** Previews the header font without saving (live update in settings panel). */
  function previewHeaderFont(): void {
    const font = (settings.value.header_font || '').trim()
    if (font) applyHeaderFont(font)
  }

  /** Loads the active (admin-selected) theme CSS and injects it. This is the
   *  "Default" look — used when the player hasn't picked a specific public theme. */
  async function loadActiveCSS(): Promise<void> {
    try {
      const data = await endpoints.styles.activeCss()
      applyCustomCSS(data.css || '')
      applyFlourishes(data.board_flourish || '', data.number_flourish || '')
    } catch {
      /* silent — custom CSS is optional */
    }
  }

  // ── Per-browser theme preference (public theme picker) ──────────────────────
  // Players may pick any Public theme for themselves; the choice is persisted per
  // browser as 'default' | '<style id>'. 'default' follows whatever theme the admin
  // has activated (see loadActiveCSS) and is always labelled "Default" in the picker
  // — the admin theme's real name is never shown, since the active-CSS endpoint and
  // the style_update broadcast carry CSS only.
  const THEME_PREF_KEY = 'bingo_theme'
  const themePreference = ref(localStorage.getItem(THEME_PREF_KEY) || 'default')

  /** Fetches + injects a Public theme's CSS by id. Returns false if it isn't
   *  public/available (so the caller can fall back to Default). */
  async function applyPublicTheme(id: number): Promise<boolean> {
    try {
      const data = await endpoints.styles.publicCss(id)
      applyCustomCSS(data.css || '')
      applyFlourishes(data.board_flourish || '', data.number_flourish || '')
      return true
    } catch {
      return false
    }
  }

  /** Resolves and applies the saved theme preference. A specific theme that is no
   *  longer Public silently reverts to Default. */
  async function applyThemePreference(): Promise<void> {
    const pref = themePreference.value
    if (pref && pref !== 'default') {
      const id = Number(pref)
      if (Number.isFinite(id) && id > 0 && (await applyPublicTheme(id))) return
      themePreference.value = 'default'
      localStorage.setItem(THEME_PREF_KEY, 'default')
    }
    await loadActiveCSS()
  }

  /** Sets + persists the per-browser theme preference, then applies it. */
  async function setThemePreference(value: string): Promise<void> {
    themePreference.value = value || 'default'
    localStorage.setItem(THEME_PREF_KEY, themePreference.value)
    await applyThemePreference()
  }

  return {
    settings,
    googleFontsList,
    uploadedFonts,
    savingSettings,
    activeBoardFlourish,
    activeNumberFlourish,
    applyFlourishes,
    loadSettings,
    saveSettings,
    loadGoogleFontsList,
    previewHeaderFont,
    loadActiveCSS,
    themePreference,
    applyThemePreference,
    setThemePreference,
  }
})
