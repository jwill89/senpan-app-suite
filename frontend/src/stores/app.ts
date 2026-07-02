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
import type { AppSettings } from '@/types/api'
import { useUiStore } from './ui'

export const useAppStore = defineStore('app', () => {
  const ui = useUiStore()

  const settings = ref<AppSettings>({ ...DEFAULT_APP_SETTINGS })
  const googleFontsList = ref<string[]>([])
  /** Filenames of fonts uploaded via System → Font Upload. */
  const uploadedFonts = ref<string[]>([])
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
      await endpoints.settings.save(settings.value)
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

  /** Loads the active theme CSS on page load and injects it. */
  async function loadActiveCSS(): Promise<void> {
    try {
      const data = await endpoints.styles.activeCss()
      applyCustomCSS(data.css || '')
      applyFlourishes(data.board_flourish || '', data.number_flourish || '')
    } catch {
      /* silent — custom CSS is optional */
    }
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
  }
})
