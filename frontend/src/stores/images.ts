/**
 * Images store: manages image "categories" (curated subdirectories of
 * <webRoot>/images) and the images within them — the System → Images admin page.
 *
 * Three categories are permanent (Announcement Main / Announcement Thumbnail /
 * Raffle) and back the announcement + raffle editors; admins may add custom ones
 * (a display name + a directory). Uploading an image whose name already exists
 * overwrites it.
 *
 * The announcement and raffle editors also use this store to LIST their category
 * (via `loadImages(dir)` → `imagesByDir[dir]`) without owning any upload UI.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import type { ImageCategory, ImageEntry } from '@/types/api'
import { useUiStore } from './ui'

/** Permanent category directory names (mirrors the Go permanent categories). */
export const IMAGE_DIR_ANNOUNCEMENTS_MAIN = 'announcements_main'
export const IMAGE_DIR_ANNOUNCEMENTS_THUMB = 'announcements_thumb'
export const IMAGE_DIR_RAFFLES = 'raffles'
export const IMAGE_DIR_FLOURISHES = 'flourishes'

export const useImagesStore = defineStore('images', () => {
  const ui = useUiStore()

  const categories = ref<ImageCategory[]>([])
  /** Categories sorted alphabetically by display name (for selects/filters). */
  const sortedCategories = computed(() =>
    [...categories.value].sort((a, b) => a.name.localeCompare(b.name)),
  )
  /** Loaded images keyed by category directory. */
  const imagesByDir = ref<Record<string, ImageEntry[]>>({})

  const loading = ref(false)
  const loadingImages = ref(false)
  const uploading = ref(false)

  async function loadCategories(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.images.categories()
      categories.value = data.categories || []
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      loading.value = false
    }
  }

  /** Loads (and caches) the images in a category directory. */
  async function loadImages(dir: string): Promise<void> {
    loadingImages.value = true
    try {
      const data = await endpoints.images.list(dir)
      imagesByDir.value = { ...imagesByDir.value, [dir]: data.images || [] }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      imagesByDir.value = { ...imagesByDir.value, [dir]: [] }
    } finally {
      loadingImages.value = false
    }
  }

  /** Creates or renames a category; returns the resulting dir on success, else null. */
  async function saveCategory(
    action: 'create' | 'rename',
    name: string,
    dir = '',
    newDir = '',
  ): Promise<string | null> {
    const trimmed = name.trim()
    if (!trimmed) {
      ui.notify('Category name is required', 'error')
      return null
    }
    try {
      const res = await endpoints.images.saveCategory(action, trimmed, dir.trim(), newDir.trim())
      ui.notify(action === 'create' ? 'Category created' : 'Category updated', 'success')
      await loadCategories()
      return res.category.dir
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      return null
    }
  }

  async function deleteCategory(cat: ImageCategory): Promise<void> {
    if (
      !(await ui.confirm(
        `Delete the “${cat.name}” category and all ${cat.file_count} image${cat.file_count === 1 ? '' : 's'} in it? This cannot be undone.`,
        { title: 'Delete category', confirmText: 'Delete' },
      ))
    )
      return
    try {
      await endpoints.images.deleteCategory(cat.dir)
      ui.notify('Category deleted', 'info')
      const next = { ...imagesByDir.value }
      delete next[cat.dir]
      imagesByDir.value = next
      await loadCategories()
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  /** Uploads files into a category, reporting per-file results, then refreshes. */
  async function uploadImages(dir: string, files: FileList | File[]): Promise<void> {
    const list = Array.from(files)
    if (list.length === 0) return
    const form = new FormData()
    form.append('dir', dir)
    for (const f of list) form.append('files', f)

    uploading.value = true
    try {
      const res = await endpoints.images.upload(form)
      const ok = res.uploaded?.length ?? 0
      const skipped = res.skipped ?? []
      if (ok > 0) ui.notify(`Uploaded ${ok} image${ok === 1 ? '' : 's'}`, 'success')
      for (const s of skipped) ui.notify(`${s.name}: ${s.reason}`, 'error')
      if (ok === 0 && skipped.length === 0) ui.notify('No images were uploaded', 'info')
      await loadImages(dir)
      await loadCategories() // refresh counts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
    }
  }

  async function deleteImage(dir: string, name: string): Promise<void> {
    if (
      !(await ui.confirm(`Delete image “${name}”? This cannot be undone.`, {
        title: 'Delete image',
        confirmText: 'Delete',
      }))
    )
      return
    try {
      await endpoints.images.deleteImage(dir, name)
      ui.notify('Image deleted', 'info')
      await loadImages(dir)
      await loadCategories() // refresh counts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    }
  }

  return {
    categories,
    sortedCategories,
    imagesByDir,
    loading,
    loadingImages,
    uploading,
    loadCategories,
    loadImages,
    saveCategory,
    deleteCategory,
    uploadImages,
    deleteImage,
  }
})
