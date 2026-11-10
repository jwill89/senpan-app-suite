/**
 * Styles store: theme CRUD + activation. A theme is a set of design-token
 * overrides (see lib/theme-tokens), edited via the structured ThemeTokenEditor
 * in the component - this store manages the theme list, the currently-edited
 * theme, and persistence. The applied stylesheet is generated from the tokens
 * server-side, and activating a theme answers with it.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api, unwrap, type StyleInput, type StyleSummary, type ThemeCSS } from '@/api/client'
import { applyCustomCSS } from '@/lib/theme'
import { defaultTokens, withDefaults } from '@/lib/theme-tokens'
import { useUiStore } from './ui'
import { useAppStore } from './app'

/** The theme in the editor: what a save sends, and its id (0 until it is created). */
export type EditingStyle = StyleInput & { id: number }

export const useStylesStore = defineStore('styles', () => {
  const ui = useUiStore()

  const styles = ref<StyleSummary[]>([])
  const editingStyle = ref<EditingStyle | null>(null)
  /** The theme the site wears; null for the default look. */
  const activeStyleId = ref<number | null>(null)
  /** True while the theme list is loading (drives the list spinner). */
  const stylesLoading = ref(false)
  /** True while the current theme is being saved (drives the Save button). */
  const savingStyle = ref(false)

  async function loadStyles(): Promise<void> {
    editingStyle.value = null
    stylesLoading.value = true
    try {
      const data = await unwrap(api.GET('/styles'))
      styles.value = data.styles
      activeStyleId.value = data.activeStyleId
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      stylesLoading.value = false
    }
  }

  async function loadStyle(id: number): Promise<void> {
    try {
      const data = await unwrap(api.GET('/styles/{id}', { params: { path: { id } } }))
      // Merge saved tokens over the defaults so every token has a value for the
      // editor (a theme may have been saved before a token existed).
      editingStyle.value = {
        id: data.id,
        name: data.name,
        tokens: withDefaults(data.tokens),
        boardFlourish: data.boardFlourish,
        numberFlourish: data.numberFlourish,
        isPublic: data.isPublic,
      }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function newStyle(): void {
    editingStyle.value = {
      id: 0,
      name: '',
      tokens: defaultTokens(),
      boardFlourish: '',
      numberFlourish: '',
      isPublic: false,
    }
  }

  async function saveStyle(): Promise<void> {
    if (!editingStyle.value) {
      return
    }
    const { id, ...style } = editingStyle.value
    const name = style.name.trim()
    if (!name) {
      ui.notify('Theme name is required', 'error')
      return
    }
    savingStyle.value = true
    const body: StyleInput = { ...style, name }
    try {
      if (id) {
        await unwrap(api.PUT('/styles/{id}', { params: { path: { id } }, body }))
        ui.notify('Theme saved', 'success')
      } else {
        const made = await unwrap(api.POST('/styles', { body }))
        editingStyle.value.id = made.id
        ui.notify('Theme created', 'success')
      }
      await loadStyles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      savingStyle.value = false
    }
  }

  async function deleteStyle(id: number): Promise<void> {
    if (
      !(await ui.confirm('Delete this theme?', { title: 'Delete theme', confirmText: 'Delete' }))
    ) {
      return
    }
    try {
      await unwrap(api.DELETE('/styles/{id}', { params: { path: { id } } }))
      if (editingStyle.value?.id === id) {
        editingStyle.value = null
      }
      ui.notify('Theme deleted', 'info')
      await loadStyles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Makes a theme the site's, or with null goes back to the default look. */
  async function setActiveStyle(id: number | null): Promise<void> {
    const app = useAppStore()
    try {
      const theme: ThemeCSS = await unwrap(
        id === null
          ? api.POST('/styles/deactivate')
          : api.POST('/styles/{id}/activate', { params: { path: { id } } }),
      )
      activeStyleId.value = id
      // Apply what the server answered with now (it also broadcasts it).
      applyCustomCSS(theme.css)
      app.applyFlourishes(theme.boardFlourish, theme.numberFlourish)
      ui.notify(id === null ? 'Theme cleared' : 'Theme activated', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    styles,
    editingStyle,
    activeStyleId,
    stylesLoading,
    savingStyle,
    loadStyles,
    loadStyle,
    newStyle,
    saveStyle,
    deleteStyle,
    setActiveStyle,
  }
})
