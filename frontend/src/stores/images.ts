/**
 * Images store: manages image "categories" (curated subdirectories of
 * <webRoot>/images) and the images within them — the System → Images admin page.
 *
 * Every category is admin-managed (a display name + a directory) and can be
 * created, renamed, and deleted. Uploading an image whose name already exists
 * overwrites it.
 *
 * The shared ImagePicker (announcements, raffles, garapons, affiliates, stamp
 * rallies, theme flourishes) also uses this store to browse categories and list
 * images (`ensureCategories()` / `ensureImages(dir)` → `imagesByDir[dir]`)
 * without owning any upload UI.
 */
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { endpoints } from '@/lib/endpoints'
import { ApiError } from '@/lib/api'
import type { ImageCategory, ImageEntry } from '@/types/api'
import { useUiStore } from './ui'

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
  // 0–100 while an upload's bytes are in flight; sits at 100 while the server
  // saves the files. -1 when no upload is running (indeterminate/idle).
  const uploadProgress = ref(-1)

  async function loadCategories(): Promise<void> {
    loading.value = true
    try {
      const data = await endpoints.images.categories()
      categories.value = data.categories
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
      imagesByDir.value = { ...imagesByDir.value, [dir]: data.images }
    } catch (e) {
      ui.notify((e as Error).message, 'error')
      imagesByDir.value = { ...imagesByDir.value, [dir]: [] }
    } finally {
      loadingImages.value = false
    }
  }

  // Silent, self-pruning variants used by the live-invalidation handler
  // (admin.ts refreshResource). Unlike loadCategories/loadImages these NEVER
  // toast — a background refresh triggered by someone else's mutation must not
  // interrupt the user — and they self-heal the cache instead of blanking it.
  async function refreshCategoriesQuiet(): Promise<void> {
    try {
      const data = await endpoints.images.categories()
      categories.value = data.categories
    } catch {
      // Stay silent. A 403 (access lost or never held) or a transient error
      // leaves the existing list untouched rather than clearing it or toasting.
    }
  }

  async function refreshImagesQuiet(dir: string): Promise<void> {
    try {
      const data = await endpoints.images.list(dir)
      imagesByDir.value = { ...imagesByDir.value, [dir]: data.images }
    } catch (e) {
      // A 400 "Unknown image category" means this dir was renamed or deleted:
      // drop the stale key so it stops being refetched (and stops re-toasting on
      // every future invalidation). Any other error (403, transient 5xx) leaves
      // the cached images in place, silently.
      if (e instanceof ApiError && e.status === 400) {
        const next = { ...imagesByDir.value }
        Reflect.deleteProperty(next, dir)
        imagesByDir.value = next
      }
    }
  }

  // In-flight de-duplication for the ensure* helpers: several pickers mount at
  // once (a form can hold four), and each asks for the same data.
  let categoriesPromise: Promise<void> | null = null
  const imagesPromises = new Map<string, Promise<void>>()

  /** Loads the category list only if it isn't already loaded or loading. */
  async function ensureCategories(): Promise<void> {
    if (categories.value.length > 0) return
    categoriesPromise ??= loadCategories().finally(() => {
      categoriesPromise = null
    })
    return categoriesPromise
  }

  /** Loads a category's images only if they aren't already cached or loading. */
  async function ensureImages(dir: string): Promise<void> {
    if (dir in imagesByDir.value) return
    let pending = imagesPromises.get(dir)
    if (!pending) {
      pending = loadImages(dir).finally(() => imagesPromises.delete(dir))
      imagesPromises.set(dir, pending)
    }
    return pending
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
      Reflect.deleteProperty(next, cat.dir)
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
    uploadProgress.value = 0
    try {
      const res = await endpoints.images.upload(form, (pct) => {
        uploadProgress.value = pct
      })
      const ok = res.uploaded.length
      const skipped = res.skipped
      if (ok > 0) ui.notify(`Uploaded ${ok} image${ok === 1 ? '' : 's'}`, 'success')
      for (const s of skipped) ui.notify(`${s.name}: ${s.reason}`, 'error')
      if (ok === 0 && skipped.length === 0) ui.notify('No images were uploaded', 'info')
      await loadImages(dir)
      await loadCategories() // refresh counts
    } catch (e) {
      ui.notify((e as Error).message, 'error')
    } finally {
      uploading.value = false
      uploadProgress.value = -1
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
    uploadProgress,
    loadCategories,
    loadImages,
    refreshCategoriesQuiet,
    refreshImagesQuiet,
    ensureCategories,
    ensureImages,
    saveCategory,
    deleteCategory,
    uploadImages,
    deleteImage,
  }
})
