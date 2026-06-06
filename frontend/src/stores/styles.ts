/**
 * Styles store: custom CSS theme CRUD + activation. The CSS editor itself is
 * CodeMirror 6 (vue-codemirror) in the component — this store just manages the
 * theme list, the currently-edited theme, and persistence. Applying the active
 * CSS live is done via the theme lib helper.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { api } from '@/lib/api'
import { applyCustomCSS } from '@/lib/theme'
import type { Style, StylesResponse } from '@/types/api'
import { useUiStore } from './ui'

export const useStylesStore = defineStore('styles', () => {
  const ui = useUiStore()

  const styles = ref<Style[]>([])
  const editingStyle = ref<Style | null>(null)
  const activeStyleId = ref('')

  async function loadStyles(): Promise<void> {
    editingStyle.value = null
    try {
      const data = await api<StylesResponse>('styles')
      styles.value = data.styles || []
      activeStyleId.value = data.active_style_id || ''
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadStyle(id: number): Promise<void> {
    try {
      const data = await api<{ style: Style }>('styles', {
        method: 'POST',
        body: { action: 'get', id },
      })
      // Guarantee css_content is a string for the v-model-bound CSS editor.
      editingStyle.value = { ...data.style, css_content: data.style.css_content ?? '' }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  function newStyle(): void {
    editingStyle.value = { id: 0, name: '', css_content: '', created_at: '' }
  }

  async function saveStyle(): Promise<void> {
    if (!editingStyle.value) return
    const name = editingStyle.value.name.trim()
    if (!name) {
      ui.notify('Theme name is required', 'error')
      return
    }
    try {
      if (editingStyle.value.id) {
        await api('styles', {
          method: 'POST',
          body: {
            action: 'update',
            id: editingStyle.value.id,
            name,
            css_content: editingStyle.value.css_content,
          },
        })
        ui.notify('Theme saved', 'success')
      } else {
        const data = await api<{ id: number }>('styles', {
          method: 'POST',
          body: { action: 'create', name, css_content: editingStyle.value.css_content },
        })
        editingStyle.value.id = data.id
        ui.notify('Theme created', 'success')
      }
      await loadStyles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function deleteStyle(id: number): Promise<void> {
    if (!(await ui.confirm('Delete this theme?', { title: 'Delete theme', confirmText: 'Delete' })))
      return
    try {
      await api('styles', { method: 'POST', body: { action: 'delete', id } })
      if (editingStyle.value && editingStyle.value.id === id) editingStyle.value = null
      ui.notify('Theme deleted', 'info')
      await loadStyles()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function setActiveStyle(id: number): Promise<void> {
    try {
      await api('styles', { method: 'POST', body: { action: 'set_active', id } })
      activeStyleId.value = id > 0 ? String(id) : ''
      // Apply locally immediately.
      if (id > 0 && editingStyle.value && editingStyle.value.id === id) {
        applyCustomCSS(editingStyle.value.css_content || '')
      } else if (id > 0) {
        await loadActiveCSS()
      } else {
        applyCustomCSS('')
      }
      ui.notify(id > 0 ? 'Theme activated' : 'Theme cleared', 'success')
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  async function loadActiveCSS(): Promise<void> {
    try {
      const data = await api<{ css: string }>('styles/active')
      applyCustomCSS(data.css || '')
    } catch {
      /* silent */
    }
  }

  return {
    styles,
    editingStyle,
    activeStyleId,
    loadStyles,
    loadStyle,
    newStyle,
    saveStyle,
    deleteStyle,
    setActiveStyle,
    loadActiveCSS,
  }
})
