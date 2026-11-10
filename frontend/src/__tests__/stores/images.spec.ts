import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Image, ImageCategory } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real one).
// Each fake answers a call's `{ data, error, response }`. Uploads go through the XHR
// helper, faked the same way.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const problem = (detail: string, status: number) => ({
  error: { title: 'Problem', detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
}))
const apiUpload = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiUpload,
}))

import { useImagesStore } from '@/stores/images'
import { useUiStore } from '@/stores/ui'

function cat(name: string, dir: string): ImageCategory {
  return { name, dir, fileCount: 0, totalSize: 0 }
}
function entry(name: string): Image {
  return {
    name,
    url: `https://h/images/d/${name}`,
    path: `images/d/${name}`,
    size: 1,
    modified: '2026-07-01T00:00:00Z',
  }
}

/** The calls of a method made to a path. */
function callsTo(method: 'GET' | 'POST' | 'PATCH' | 'DELETE', path: string) {
  return api[method].mock.calls.filter(([p]) => p === path) as [string, Record<string, unknown>][]
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  apiUpload.mockReset()
  api.GET.mockImplementation(async (path: string) =>
    path === '/image-categories' ? ok({ categories: [] }) : ok({ dir: '', images: [] }),
  )
  api.POST.mockResolvedValue(ok(cat('Event Banners', 'event_banners'), 201))
  api.PATCH.mockResolvedValue(ok(cat('Event Banners', 'event_banners')))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
  apiUpload.mockResolvedValue({ uploaded: [entry('a.png')], skipped: [] })
})

describe('images sortedCategories', () => {
  it('sorts categories alphabetically by name', () => {
    const store = useImagesStore()
    store.categories = [cat('Raffle', 'raffles'), cat('Announcement Main', 'announcements_main')]
    expect(store.sortedCategories.map((c) => c.name)).toEqual(['Announcement Main', 'Raffle'])
  })
})

describe('images loaders', () => {
  it('loadCategories populates from the API', async () => {
    api.GET.mockResolvedValueOnce(ok({ categories: [cat('Raffle', 'raffles')] }))
    const store = useImagesStore()
    await store.loadCategories()
    expect(store.categories).toHaveLength(1)
    expect(store.categories[0]!.dir).toBe('raffles')
  })

  it('loadImages caches results keyed by directory', async () => {
    api.GET.mockResolvedValueOnce(ok({ dir: 'raffles', images: [entry('a.png')] }))
    const store = useImagesStore()
    await store.loadImages('raffles')
    expect(callsTo('GET', '/image-categories/{dir}/images')[0]![1]).toMatchObject({
      params: { path: { dir: 'raffles' } },
    })
    expect(store.imagesByDir.raffles!.map((i) => i.name)).toEqual(['a.png'])
  })

  it('ensureCategories fetches once for concurrent callers and skips when loaded', async () => {
    api.GET.mockResolvedValue(ok({ categories: [cat('Raffle', 'raffles')] }))
    const store = useImagesStore()
    await Promise.all([store.ensureCategories(), store.ensureCategories()])
    expect(callsTo('GET', '/image-categories')).toHaveLength(1)
    await store.ensureCategories()
    expect(callsTo('GET', '/image-categories')).toHaveLength(1)
  })

  it('ensureImages fetches a directory once and skips when cached', async () => {
    api.GET.mockResolvedValue(ok({ dir: 'raffles', images: [entry('a.png')] }))
    const store = useImagesStore()
    await Promise.all([store.ensureImages('raffles'), store.ensureImages('raffles')])
    expect(callsTo('GET', '/image-categories/{dir}/images')).toHaveLength(1)
    await store.ensureImages('raffles')
    expect(callsTo('GET', '/image-categories/{dir}/images')).toHaveLength(1)
  })
})

describe('images quiet refreshers (live invalidation)', () => {
  it('refreshCategoriesQuiet updates on success and never toasts on error', async () => {
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')

    api.GET.mockResolvedValueOnce(ok({ categories: [cat('Raffle', 'raffles')] }))
    await store.refreshCategoriesQuiet()
    expect(store.categories.map((c) => c.dir)).toEqual(['raffles'])

    // A 403 (access lost) leaves the list untouched and raises no toast.
    api.GET.mockResolvedValueOnce(problem('Forbidden', 403))
    await store.refreshCategoriesQuiet()
    expect(store.categories.map((c) => c.dir)).toEqual(['raffles'])
    expect(notify).not.toHaveBeenCalled()
  })

  it('refreshImagesQuiet drops a stale dir key on 404 and stays silent otherwise', async () => {
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')
    store.imagesByDir = { gone: [entry('a.png')], kept: [entry('b.png')] }

    // 404 -> the renamed or deleted dir key is pruned.
    api.GET.mockResolvedValueOnce(problem('There is no such image category.', 404))
    await store.refreshImagesQuiet('gone')
    expect('gone' in store.imagesByDir).toBe(false)
    expect('kept' in store.imagesByDir).toBe(true)

    // A transient 502 leaves the cached images in place, silently.
    api.GET.mockResolvedValueOnce(problem('Bad gateway', 502))
    await store.refreshImagesQuiet('kept')
    expect(store.imagesByDir.kept!.map((i) => i.name)).toEqual(['b.png'])
    expect(notify).not.toHaveBeenCalled()
  })
})

describe('images saveCategory', () => {
  it('creates a category with trimmed args and returns its dir', async () => {
    const store = useImagesStore()
    const dir = await store.saveCategory('create', '  Event Banners  ', '  ')
    expect(callsTo('POST', '/image-categories')[0]![1]).toMatchObject({
      body: { name: 'Event Banners', dir: '' },
    })
    expect(dir).toBe('event_banners')
    expect(callsTo('GET', '/image-categories')).toHaveLength(1) // refreshes the list
  })

  it('renames a category by its current dir', async () => {
    const store = useImagesStore()
    const dir = await store.saveCategory('rename', 'Festival Banners', 'event_banners', '')
    expect(callsTo('PATCH', '/image-categories/{dir}')[0]![1]).toMatchObject({
      params: { path: { dir: 'event_banners' } },
      body: { name: 'Festival Banners', dir: '' },
    })
    expect(dir).toBe('event_banners')
  })

  it('rejects a blank name without calling the API', async () => {
    const store = useImagesStore()
    const dir = await store.saveCategory('create', '   ')
    expect(dir).toBeNull()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it("shows the server's reason when the category is refused", async () => {
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')
    api.POST.mockResolvedValueOnce(
      problem('That directory name is reserved for another feature', 409),
    )
    expect(await store.saveCategory('create', 'Covers', 'bookclub')).toBeNull()
    expect(notify).toHaveBeenCalledWith(
      'That directory name is reserved for another feature',
      'error',
    )
  })
})

describe('images uploadImages', () => {
  it('posts a FormData of each file to the category, then refreshes', async () => {
    const store = useImagesStore()
    const files = [new File(['x'], 'a.png', { type: 'image/png' })]
    await store.uploadImages('raffles', files)
    expect(apiUpload).toHaveBeenCalledTimes(1)
    const [endpoint, form] = apiUpload.mock.calls[0] as [string, FormData]
    expect(endpoint).toBe('image-categories/raffles/images')
    expect(form.has('dir')).toBe(false)
    expect((form.getAll('files')[0] as File).name).toBe('a.png')
    expect(callsTo('GET', '/image-categories/{dir}/images')).toHaveLength(1) // reloads the category
  })

  it('does nothing for an empty file list', async () => {
    const store = useImagesStore()
    await store.uploadImages('raffles', [])
    expect(apiUpload).not.toHaveBeenCalled()
  })

  it('tracks upload progress via the callback and resets it when done', async () => {
    const store = useImagesStore()
    let midFlight = -99
    apiUpload.mockImplementationOnce(
      async (_endpoint: string, _form: FormData, opts: { onProgress?: (p: number) => void }) => {
        opts.onProgress?.(42) // server-driven progress event
        midFlight = store.uploadProgress
        return { uploaded: [entry('a.png')], skipped: [] }
      },
    )
    expect(store.uploadProgress).toBe(-1) // idle before
    await store.uploadImages('raffles', [new File(['x'], 'a.png', { type: 'image/png' })])
    expect(midFlight).toBe(42) // the callback drove the ref mid-upload
    expect(store.uploadProgress).toBe(-1) // reset in finally
  })

  it('names each refused file when none could be saved', async () => {
    const { ApiError } = await import('@/lib/api')
    apiUpload.mockRejectedValueOnce(
      new ApiError('None of the files could be saved.', 422, {
        errors: [{ location: 'body.files[0]', message: 'Not a valid image', value: 'fake.png' }],
      }),
    )
    const store = useImagesStore()
    const notify = vi.spyOn(useUiStore(), 'notify')
    await store.uploadImages('raffles', [new File(['x'], 'fake.png')])
    expect(notify).toHaveBeenCalledWith('fake.png: Not a valid image', 'error')
  })
})

describe('images deletes (confirm-gated)', () => {
  it('deleteCategory calls the API once confirmed', async () => {
    const store = useImagesStore()
    vi.spyOn(useUiStore(), 'confirm').mockResolvedValue(true)
    await store.deleteCategory(cat('Temp', 'temp_cat'))
    expect(callsTo('DELETE', '/image-categories/{dir}')[0]![1]).toMatchObject({
      params: { path: { dir: 'temp_cat' } },
    })
  })

  it('deleteImage is a no-op when the confirm is declined', async () => {
    const store = useImagesStore()
    vi.spyOn(useUiStore(), 'confirm').mockResolvedValue(false)
    await store.deleteImage('raffles', 'a.png')
    expect(api.DELETE).not.toHaveBeenCalled()
  })

  it('deleteImage removes the file by dir and name', async () => {
    const store = useImagesStore()
    vi.spyOn(useUiStore(), 'confirm').mockResolvedValue(true)
    await store.deleteImage('raffles', 'a b.png')
    expect(callsTo('DELETE', '/image-categories/{dir}/images/{name}')[0]![1]).toMatchObject({
      params: { path: { dir: 'raffles', name: 'a b.png' } },
    })
  })
})
