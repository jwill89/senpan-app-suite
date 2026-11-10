/**
 * App store: global app settings, Google Fonts list, and active theme CSS.
 *
 * Mirrors the original app.js settings logic (loadSettings/saveSettings,
 * loadGoogleFontsList, _applyHeaderFont, _loadActiveCSS/_applyCustomCSS).
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { api, unwrap, type Integrations, type SettingsInput, type UploadedFont } from '@/api/client'
import {
  applyCustomCSS,
  applyHeaderFont,
  applyUploadedFonts,
  applyNumberFlourish,
} from '@/lib/theme'
import { DEFAULT_SETTINGS } from '@/lib/constants'
import { useUiStore } from './ui'

export const useAppStore = defineStore('app', () => {
  const ui = useUiStore()

  const settings = ref<SettingsInput>({ ...DEFAULT_SETTINGS })
  /**
   * The keys the server uses with outside services (Settings -> Integrations): admin-only,
   * loaded by the settings page for an admin, and blank for anyone else.
   */
  const integrations = ref<Integrations>({ googleFontsApiKey: '' })
  const googleFontsList = ref<string[]>([])
  /** Fonts uploaded via System -> Font Upload (name + serving token). */
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

  /**
   * True once the server's settings have been read (successfully or not). Until
   * then `settings` holds DEFAULT_SETTINGS, which is a guess - anything that
   * would SHOW something the settings might hide has to wait for this, or the
   * hidden thing flashes up and vanishes, and can be clicked while it's there.
   */
  const settingsLoaded = ref(false)

  /**
   * Whether the public home page hides its Bingo entry points (Join Bingo and the
   * Custom Card request that belongs with it).
   */
  const hideBingo = computed(() => settings.value.hideBingo)

  /**
   * Whether the home page hides the Custom Card request on its own, leaving Join
   * Bingo where it is - for a stretch when staff aren't taking requests but games
   * are still running. `hideBingo` is the wider switch of the two: it hides this
   * section as well, so the home page checks both.
   */
  const hideCustomCards = computed(() => settings.value.hideCustomCards)

  /** Sets the active flourishes + applies the number-flourish CSS variable. */
  function applyFlourishes(board: string, number: string): void {
    activeBoardFlourish.value = board || ''
    activeNumberFlourish.value = number || ''
    applyNumberFlourish(activeNumberFlourish.value)
  }

  /** Loads app settings, applies title + header font, then fetches fonts. */
  async function loadSettings(): Promise<void> {
    try {
      const { uploadedFonts: fonts, ...data } = await unwrap(api.GET('/settings'))
      // Register uploaded fonts first so applyHeaderFont knows to skip Google
      // for an uploaded family.
      uploadedFonts.value = fonts
      applyUploadedFonts(uploadedFonts.value)
      settings.value = data
      document.title = settings.value.appTitle || 'Senpan App Suite'
      applyHeaderFont(settings.value.headerFont)
    } catch {
      /* silent */
    } finally {
      // Set even on failure: the defaults are then the best answer available, and
      // leaving this false would hide the home page's cards for good.
      settingsLoaded.value = true
    }
  }

  /** Loads the integrations (admin-only), then the Google Fonts list their key opens. */
  async function loadIntegrations(): Promise<void> {
    try {
      integrations.value = await unwrap(api.GET('/settings/integrations'))
      void loadGoogleFontsList()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /**
   * Saves the settings, and the integrations too for an admin, then applies them
   * and notifies. A save made from an older copy is refused (409), with the
   * server's message saying to reload.
   */
  async function saveSettings(withIntegrations = false): Promise<void> {
    savingSettings.value = true
    try {
      const { uploadedFonts: fonts, ...saved } = await unwrap(
        api.PUT('/settings', { body: settings.value }),
      )
      settings.value = saved
      uploadedFonts.value = fonts
      if (withIntegrations) {
        integrations.value = await unwrap(
          api.PUT('/settings/integrations', { body: integrations.value }),
        )
        void loadGoogleFontsList()
      }
      document.title = settings.value.appTitle || 'Senpan App Suite'
      applyHeaderFont(settings.value.headerFont)
      ui.notify('Settings saved!', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingSettings.value = false
    }
  }

  /** Fetches the Google Fonts list if an API key is configured (cached). */
  async function loadGoogleFontsList(): Promise<void> {
    const key = integrations.value.googleFontsApiKey.trim()
    if (!key) {
      googleFontsList.value = []
      googleFontsCacheKey = ''
      return
    }
    if (key === googleFontsCacheKey && googleFontsList.value.length > 0) {
      return
    }
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
    const font = settings.value.headerFont.trim()
    if (font) {
      applyHeaderFont(font)
    }
  }

  /** Loads the active (admin-selected) theme CSS and injects it. This is the
   *  "Default" look - used when the player hasn't picked a specific public theme. */
  async function loadActiveCSS(): Promise<void> {
    try {
      const data = await unwrap(api.GET('/styles/active'))
      applyCustomCSS(data.css)
      applyFlourishes(data.boardFlourish, data.numberFlourish)
    } catch {
      /* silent - custom CSS is optional */
    }
  }

  // -- Per-browser theme preference (public theme picker) ----------------------
  // Players may pick any Public theme for themselves; the choice is persisted per
  // browser as 'default' | '<style id>'. 'default' follows whatever theme the admin
  // has activated (see loadActiveCSS) and is always labelled "Default" in the picker
  // - the admin theme's real name is never shown, since the active-CSS endpoint
  // carries CSS only. A theme_changed event has a page wearing the site's theme, or
  // the theme written, read it again (useLiveEvents).
  const THEME_PREF_KEY = 'bingo_theme'
  const themePreference = ref(localStorage.getItem(THEME_PREF_KEY) || 'default')

  /** Fetches + injects a Public theme's CSS by id. Returns false if it isn't
   *  public/available (so the caller can fall back to Default). */
  async function applyPublicTheme(id: number): Promise<boolean> {
    try {
      const data = await unwrap(api.GET('/styles/public/{id}', { params: { path: { id } } }))
      applyCustomCSS(data.css)
      applyFlourishes(data.boardFlourish, data.numberFlourish)
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
      if (Number.isFinite(id) && id > 0 && (await applyPublicTheme(id))) {
        return
      }
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
    integrations,
    googleFontsList,
    uploadedFonts,
    savingSettings,
    activeBoardFlourish,
    activeNumberFlourish,
    settingsLoaded,
    hideBingo,
    hideCustomCards,
    applyFlourishes,
    loadSettings,
    loadIntegrations,
    saveSettings,
    loadGoogleFontsList,
    previewHeaderFont,
    loadActiveCSS,
    themePreference,
    applyThemePreference,
    setThemePreference,
  }
})
