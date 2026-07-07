import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { ImageCategory, ImageEntry } from '@/types/api'

// Stub the endpoint layer; each fn is spied so the tests can assert on calls.
const { categories, saveCategory, deleteCategory, list, upload, deleteImage } = vi.hoisted(() => ({
  categories: vi.fn(async () => ({ categories: [] as ImageCategory[] })),
  saveCategory: vi.fn(async () => ({ ok: true, category: { dir: 'event_banners' } })),
  deleteCategory: vi.fn(async () => ({ ok: true })),
  list: vi.fn(async () => ({ dir: '', images: [] as ImageEntry[] })),
  upload: vi.fn(async () => ({ uploaded: ['a.png'], skipped: [] })),
  deleteImage: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { images: { categories, saveCategory, deleteCategory, list, upload, deleteImage } },
}))

import { useImagesStore } from './images'
import { useUiStore } from './ui'
import { ApiError } from '@/lib/api'

function cat(name: string, dir: string): ImageCategory {
  return { name, dir, file_count: 0, total_size: 0 }
}
function entry(name: string): ImageEntry {
  return {
    name,
    url: `https://h/images/d/${name}`,
    path: `images/d/${name}`,
    size: 1,
    modified: '',
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  vi.clearAllMocks()
})

describe('images sortedCategories', () => {
  it('sorts categories alphabetically by name', () => {
    const store = useImagesStore()
    store.categories = [cat('Raffle', 'raffles'), cat('Announcement Main', 'announcements_main')]
    expect(store.sortedCategories.map((c) => c.name)).toEqual(['Announcement Main', 'Raffle'])
  })
})

describe('images loaders', () => {
  it('loadCategories populates from the endpoint', async () => {
    categories.mockResolvedValueOnce({ categories: [cat('Raffle', 'raffles')] })
    const store = useImagesStore()
    await store.loadCategories()
    expect(store.categories).toHaveLength(1)
    expect(store.categories[0].dir).toBe('raffles')
  })

  it('loadImages caches results keyed by directory', async () => {
    list.mockResolvedValueOnce({ dir: 'raffles', images: [entry('a.png')] })
    const store = useImagesStore()
    await store.loadImages('raffles')
    expect(list).toHaveBeenCalledWith('raffles')
    expect(store.imagesByDir.raffles.map((i) => i.name)).toEqual(['a.png'])
  })

  it('ensureCategories fetches once for concurrent callers and skips when loaded', async () => {
    categories.mockResolvedValue({ categories: [cat('Raffle', 'raffles')] })
    const store = useImagesStore()
    await Promise.all([store.ensureCategories(), store.ensureCategories()])
    expect(categories).toHaveBeenCalledTimes(1)
    await store.ensureCategories()
    expect(categories).toHaveBeenCalledTimes(1)
  })

  it('ensureImages fetches a directory once and skips when cached', async () => {
    list.mockResolvedValue({ dir: 'raffles', images: [entry('a.png')] })
    const store = useImagesStore()
    await Promise.all([store.ensureImages('raffles'), store.ensureImages('raffles')])
    expect(list).toHaveBeenCalledTimes(1)
    await store.ensureImages('raffles')
    expect(list).toHaveBeenCalledTimes(1)
  })
})

describe('images quiet refreshers (live invalidation)', () => {
  it('refreshCategoriesQuiet updates on success and never toasts on error', async () => {
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')

    categories.mockResolvedValueOnce({ categories: [cat('Raffle', 'raffles')] })
    await store.refreshCategoriesQuiet()
    expect(store.categories.map((c) => c.dir)).toEqual(['raffles'])

    // A 403 (access lost) leaves the list untouched and raises no toast.
    categories.mockRejectedValueOnce(new ApiError('Forbidden', 403))
    await store.refreshCategoriesQuiet()
    expect(store.categories.map((c) => c.dir)).toEqual(['raffles'])
    expect(notify).not.toHaveBeenCalled()
  })

  it('refreshImagesQuiet drops a stale dir key on 400 and stays silent otherwise', async () => {
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')
    store.imagesByDir = { gone: [entry('a.png')], kept: [entry('b.png')] }

    // 400 Unknown image category → the renamed/deleted dir key is pruned.
    list.mockRejectedValueOnce(new ApiError('Unknown image category', 400))
    await store.refreshImagesQuiet('gone')
    expect('gone' in store.imagesByDir).toBe(false)
    expect('kept' in store.imagesByDir).toBe(true)

    // A transient 502 leaves the cached images in place, silently.
    list.mockRejectedValueOnce(new ApiError('Bad gateway', 502))
    await store.refreshImagesQuiet('kept')
    expect(store.imagesByDir.kept.map((i) => i.name)).toEqual(['b.png'])
    expect(notify).not.toHaveBeenCalled()
  })
})

describe('images saveCategory', () => {
  it('creates a category with trimmed args and returns its dir', async () => {
    const store = useImagesStore()
    const dir = await store.saveCategory('create', '  Event Banners  ', '  ')
    expect(saveCategory).toHaveBeenCalledWith('create', 'Event Banners', '', '')
    expect(dir).toBe('event_banners')
    expect(categories).toHaveBeenCalled() // refreshes the list
  })

  it('rejects a blank name without calling the endpoint', async () => {
    const store = useImagesStore()
    const dir = await store.saveCategory('create', '   ')
    expect(dir).toBeNull()
    expect(saveCategory).not.toHaveBeenCalled()
  })
})

describe('images uploadImages', () => {
  it('posts a FormData with the dir and each file, then refreshes', async () => {
    const store = useImagesStore()
    const files = [new File(['x'], 'a.png', { type: 'image/png' })]
    await store.uploadImages('raffles', files)
    expect(upload).toHaveBeenCalledTimes(1)
    const form = (upload.mock.calls[0] as unknown[])[0] as FormData
    expect(form.get('dir')).toBe('raffles')
    expect((form.getAll('files')[0] as File).name).toBe('a.png')
    expect(list).toHaveBeenCalledWith('raffles') // reloads the category
  })

  it('does nothing for an empty file list', async () => {
    const store = useImagesStore()
    await store.uploadImages('raffles', [])
    expect(upload).not.toHaveBeenCalled()
  })
})

describe('images deletes (confirm-gated)', () => {
  it('deleteCategory calls the endpoint once confirmed', async () => {
    const store = useImagesStore()
    vi.spyOn(useUiStore(), 'confirm').mockResolvedValue(true)
    await store.deleteCategory(cat('Temp', 'temp_cat'))
    expect(deleteCategory).toHaveBeenCalledWith('temp_cat')
  })

  it('deleteImage is a no-op when the confirm is declined', async () => {
    const store = useImagesStore()
    vi.spyOn(useUiStore(), 'confirm').mockResolvedValue(false)
    await store.deleteImage('raffles', 'a.png')
    expect(deleteImage).not.toHaveBeenCalled()
  })
})
