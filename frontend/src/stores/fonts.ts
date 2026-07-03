/**
 * Fonts store: manages the uploaded fonts (the Atelier → Font Upload admin
 * tab). A logical font GROUPS the uploaded files sharing a base name as format
 * variants (TTF/OTF/WOFF/WOFF2/EOT, plus an auto-converted WOFF2 copy). The
 * store lists fonts, uploads files (multiple at once), renames/deletes
 * individual variant files, deletes whole fonts, and edits a font's metadata:
 * its CSS family name, the variant type it serves publicly, and its PER-FONT
 * allowed-site origins. Uploads of a name that already exists are rejected
 * server-side — the existing file must be deleted first.
 *
 * Fonts are not served as static files: external sites embed the generated kit
 * stylesheet (FONT_KIT_URL) and load fonts through obfuscated, rotating token
 * URLs, gated per font by its origin allowlist. The app itself loads fonts
 * same-origin (see lib/theme.ts) and is always allowed.
 */
import { defineStore } from 'pinia'
import { ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { Font, UploadedFont } from '@/types/api'
import { useUiStore } from './ui'

/** Public base URL of the external fonts host (no trailing slash). Its vhost
 *  reverse-proxies every path to the backend's tokenized font endpoints. */
export const FONT_BASE_URL = 'https://fonts.senpan.cafe'

/** The kit stylesheet external (Carrd) sites embed via <link rel="stylesheet">. */
export const FONT_KIT_URL = `${FONT_BASE_URL}/kit.css`

/** Builds the external tokenized URL for a variant token (rotates every 1–2
 *  weeks — embed the kit stylesheet, not this, for anything permanent). */
export function fontShareUrl(token: string): string {
  return `${FONT_BASE_URL}/f/${encodeURIComponent(token)}`
}

/** Maps a font to the shape applyUploadedFonts registers (its served variant,
 *  under the font's effective CSS family). */
export function toUploadedFont(f: Font): UploadedFont {
  return { name: f.base, family: f.family, token: f.served_token }
}

export const useFontsStore = defineStore('fonts', () => {
  const ui = useUiStore()

  const fonts = ref<Font[]>([])
  /** True while the font list is loading (drives the table spinner). */
  const loading = ref(false)
  /** True while an upload is in flight (drives the upload button). */
  const uploading = ref(false)
  /** True while a metadata save is in flight (drives the Edit modal button). */
  const saving = ref(false)

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
        ui.notify(`Uploaded ${ok} file${ok === 1 ? '' : 's'}`, 'success')
      }
      for (const s of skipped) {
        ui.notify(`${s.name}: ${s.reason}`, 'error')
      }
      // Conversion warnings: the file uploaded, but its WOFF2 conversion
      // failed — an uploaded format is served for that font.
      for (const w of res.warnings ?? []) {
        ui.notify(w, 'info')
      }
      if (ok === 0 && skipped.length === 0) {
        ui.notify('No files were uploaded', 'info')
      }
      await loadFonts()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
    }
  }

  /** Deletes a whole font (every variant file) after confirmation. */
  async function deleteFont(font: Font): Promise<void> {
    const files = font.variants.filter((v) => !v.converted).map((v) => v.name)
    if (
      !(await ui.confirm(`Delete "${font.family}" (${files.join(', ')})? This cannot be undone.`, {
        title: 'Delete font',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.fonts.deleteFont(font.base)
      ui.notify('Font deleted', 'info')
      await loadFonts()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Deletes one variant file after confirmation. */
  async function deleteFile(name: string): Promise<void> {
    if (
      !(await ui.confirm(`Delete the file "${name}"? This cannot be undone.`, {
        title: 'Delete font file',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.fonts.deleteFile(name)
      ui.notify('File deleted', 'info')
      await loadFonts()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Renames one variant file; returns true on success. */
  async function renameFile(name: string, newName: string): Promise<boolean> {
    const trimmed = newName.trim()
    if (!trimmed || trimmed === name) return false
    try {
      await endpoints.fonts.renameFile(name, trimmed)
      ui.notify('File renamed', 'success')
      await loadFonts()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    }
  }

  /**
   * Saves a font's metadata (CSS family name, served variant type, per-font
   * allowed sites) and reloads the list; returns true on success.
   */
  async function updateFamily(
    base: string,
    fields: { family?: string; serve?: string; origins?: string[] },
  ): Promise<boolean> {
    saving.value = true
    try {
      await endpoints.fonts.updateFamily(base, fields)
      ui.notify('Font updated', 'success')
      await loadFonts()
      return true
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return false
    } finally {
      saving.value = false
    }
  }

  return {
    fonts,
    loading,
    uploading,
    saving,
    loadFonts,
    uploadFonts,
    deleteFont,
    deleteFile,
    renameFile,
    updateFamily,
  }
})
