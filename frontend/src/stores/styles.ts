/**
 * Styles store: custom CSS theme CRUD + activation. The CSS editor itself is
 * CodeMirror 6 (vue-codemirror) in the component — this store just manages the
 * theme list, the currently-edited theme, and persistence. Applying the active
 * CSS live is done via the theme lib helper.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { applyCustomCSS } from '@/lib/theme'
import type { Style } from '@/types/api'
import { useUiStore } from './ui'
import { useAppStore } from './app'

export const useStylesStore = defineStore('styles', () => {
  const ui = useUiStore()

  const styles = ref<Style[]>([])
  const editingStyle = ref<Style | null>(null)
  const activeStyleId = ref('')
  /** True while the theme list is loading (drives the list spinner). */
  const stylesLoading = ref(false)
  /** True while the current theme is being saved (drives the Save button). */
  const savingStyle = ref(false)

  async function loadStyles(): Promise<void> {
    editingStyle.value = null
    stylesLoading.value = true
    try {
      const data = await endpoints.styles.list()
      styles.value = data.styles || []
      activeStyleId.value = data.active_style_id || ''
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      stylesLoading.value = false
    }
  }

  async function loadStyle(id: number): Promise<void> {
    try {
      const data = await endpoints.styles.get(id)
      // Guarantee css_content is a string for the v-model-bound CSS editor.
      editingStyle.value = { ...data.style, css_content: data.style.css_content ?? '' }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function newStyle(): void {
    editingStyle.value = {
      id: 0,
      name: '',
      css_content: '',
      board_flourish: '',
      number_flourish: '',
      created_at: '',
    }
  }

  async function saveStyle(): Promise<void> {
    if (!editingStyle.value) return
    const name = editingStyle.value.name.trim()
    if (!name) {
      ui.notify('Theme name is required', 'error')
      return
    }
    savingStyle.value = true
    const css = editingStyle.value.css_content ?? ''
    const board = editingStyle.value.board_flourish ?? ''
    const number = editingStyle.value.number_flourish ?? ''
    try {
      if (editingStyle.value.id) {
        await endpoints.styles.update(editingStyle.value.id, name, css, board, number)
        ui.notify('Theme saved', 'success')
      } else {
        const data = await endpoints.styles.create(name, css, board, number)
        editingStyle.value.id = data.id
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
    if (!(await ui.confirm('Delete this theme?', { title: 'Delete theme', confirmText: 'Delete' })))
      return
    try {
      await endpoints.styles.delete(id)
      if (editingStyle.value && editingStyle.value.id === id) editingStyle.value = null
      ui.notify('Theme deleted', 'info')
      await loadStyles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function setActiveStyle(id: number): Promise<void> {
    const app = useAppStore()
    try {
      await endpoints.styles.setActive(id)
      activeStyleId.value = id > 0 ? String(id) : ''
      // Apply CSS + flourishes locally immediately (the server also broadcasts).
      if (id > 0 && editingStyle.value && editingStyle.value.id === id) {
        applyCustomCSS(editingStyle.value.css_content || '')
        app.applyFlourishes(
          editingStyle.value.board_flourish || '',
          editingStyle.value.number_flourish || '',
        )
      } else if (id > 0) {
        await app.loadActiveCSS()
      } else {
        applyCustomCSS('')
        app.applyFlourishes('', '')
      }
      ui.notify(id > 0 ? 'Theme activated' : 'Theme cleared', 'success')
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
