/**
 * App store: global app settings, Google Fonts list, and active theme CSS.
 *
 * Mirrors the original app.js settings logic (loadSettings/saveSettings,
 * loadGoogleFontsList, _applyHeaderFont, _loadActiveCSS/_applyCustomCSS).
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/lib/api'
import { applyCustomCSS, applyHeaderFont } from '@/lib/theme'
import { DEFAULT_APP_SETTINGS } from '@/lib/constants'
import type { AppSettings, SettingsResponse } from '@/types/api'
import { useUiStore } from './ui'

export const useAppStore = defineStore('app', () => {
  const ui = useUiStore()

  const settings = ref<AppSettings>({ ...DEFAULT_APP_SETTINGS })
  const googleFontsList = ref<string[]>([])
  let googleFontsCacheKey = ''

  /** Loads app settings, applies title + header font, then fetches fonts. */
  async function loadSettings(): Promise<void> {
    try {
      const data = await api<SettingsResponse>('settings')
      if (data.settings) {
        settings.value = { ...settings.value, ...data.settings }
        document.title = settings.value.app_title || 'Senpan App Suite'
        applyHeaderFont(settings.value.header_font)
        loadGoogleFontsList()
      }
    } catch {
      /* silent */
    }
  }

  /** Saves settings to the server, applies them, and notifies. */
  async function saveSettings(): Promise<void> {
    try {
      await api('settings', { method: 'POST', body: { settings: settings.value } })
      document.title = settings.value.app_title || 'Senpan App Suite'
      applyHeaderFont(settings.value.header_font)
      loadGoogleFontsList()
      ui.notify('Settings saved!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
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
      const data = await api<{ css: string }>('styles/active')
      applyCustomCSS(data.css || '')
    } catch {
      /* silent — custom CSS is optional */
    }
  }

  return {
    settings,
    googleFontsList,
    loadSettings,
    saveSettings,
    loadGoogleFontsList,
    previewHeaderFont,
    loadActiveCSS,
  }
})
