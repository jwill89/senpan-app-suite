/**
 * Fonts store: manages the font files in <webRoot>/fonts (the System → Font
 * Upload admin tab). Lists, uploads (multiple at once), renames, and deletes
 * font files. Uploads of a name that already exists are rejected server-side —
 * the existing file must be deleted first.
 *
 * Font files are served publicly from FONT_BASE_URL; the table links each file
 * to its public URL.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { FontFile } from '@/types/api'
import { useUiStore } from './ui'

/** Public base URL the fonts directory is served from (no trailing slash). */
export const FONT_BASE_URL = 'https://fonts.senpan.cafe'

/** Builds the public URL for a font file name. */
export function fontUrl(name: string): string {
  return `${FONT_BASE_URL}/${encodeURIComponent(name)}`
}

export const useFontsStore = defineStore('fonts', () => {
  const ui = useUiStore()

  const fonts = ref<FontFile[]>([])
  /** True while the font list is loading (drives the table spinner). */
  const loading = ref(false)
  /** True while an upload is in flight (drives the upload button). */
  const uploading = ref(false)

  async function loadFonts(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.fonts.list()
      fonts.value = data.fonts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  /** Uploads the selected files, then refreshes the list. */
  async function uploadFonts(files: FileList | File[]): Promise<void> {
    const list = Array.from(files)
    if (list.length === 0) return
    const form = new FormData()
    for (const f of list) form.append('files', f)

    uploading.value = true
    try {
      const res = await endpoints.fonts.upload(form)
      const ok = res.uploaded.length
      const skipped = res.skipped
      if (ok > 0) {
        ui.notify(`Uploaded ${ok} font${ok === 1 ? '' : 's'}`, 'success')
      }
      for (const s of skipped) {
        ui.notify(`${s.name}: ${s.reason}`, 'error')
      }
      if (ok === 0 && skipped.length === 0) {
        ui.notify('No fonts were uploaded', 'info')
      }
      await loadFonts()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
    }
  }

  async function deleteFont(name: string): Promise<void> {
    if (
      !(await ui.confirm(`Delete "${name}"? This cannot be undone.`, {
        title: 'Delete font',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.fonts.delete(name)
      ui.notify('Font deleted', 'info')
      await loadFonts()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Renames a font file; returns true on success. */
  async function renameFont(name: string, newName: string): Promise<boolean> {
    const trimmed = newName.trim()
    if (!trimmed || trimmed === name) return false
    try {
      await endpoints.fonts.rename(name, trimmed)
      ui.notify('Font renamed', 'success')
      await loadFonts()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  return {
    fonts,
    loading,
    uploading,
    loadFonts,
    uploadFonts,
    deleteFont,
    renameFont,
  }
})
