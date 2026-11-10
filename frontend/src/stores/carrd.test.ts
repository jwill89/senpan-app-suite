import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { CarrdProject } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real one).
// Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

/** The calls of a method made to a path, as `[path, init]`. */
function callsTo(method: 'GET' | 'POST', path: string) {
  return api[method].mock.calls.filter(([p]) => p === path) as [string, Record<string, unknown>][]
}

import { useCarrdStore, carrdImageUrl, joinCarrdPath } from './carrd'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) =>
    path === '/carrd-projects'
      ? ok({ projects: [] })
      : ok({ folder: 'art', path: '', folders: [], files: [] }),
  )
  api.POST.mockImplementation(async (path: string) =>
    path === '/carrd-projects'
      ? ok({ folder: 'art', title: 'Art' }, 201)
      : ok({ name: 'sub' }, 201),
  )
})

describe('pure helpers', () => {
  it('carrdImageUrl encodes each segment and skips empty path parts', () => {
    expect(carrdImageUrl('my folder', 'a/b', 'pic 1.png')).toBe(
      'https://carrd.senpan.cafe/my%20folder/a/b/pic%201.png',
    )
    expect(carrdImageUrl('folder', '', 'p.png')).toBe('https://carrd.senpan.cafe/folder/p.png')
  })

  it('joinCarrdPath keeps the root clean (no leading slash)', () => {
    expect(joinCarrdPath('', 'sub')).toBe('sub')
    expect(joinCarrdPath('a', 'b')).toBe('a/b')
  })
})

describe('loadProjects', () => {
  it('drops a selection whose project no longer exists', async () => {
    api.GET.mockResolvedValueOnce(ok({ projects: [{ folder: 'other' } as CarrdProject] }))
    const s = useCarrdStore()
    s.selectedFolder = 'gone'
    s.currentPath = 'sub'
    await s.loadProjects()
    expect(s.selectedFolder).toBeNull()
    expect(s.currentPath).toBe('')
  })

  it('keeps a selection that still exists', async () => {
    api.GET.mockResolvedValueOnce(ok({ projects: [{ folder: 'keep' } as CarrdProject] }))
    const s = useCarrdStore()
    s.selectedFolder = 'keep'
    await s.loadProjects()
    expect(s.selectedFolder).toBe('keep')
  })
})

describe('navigation', () => {
  it('openProject selects the folder at its root and loads contents', async () => {
    const s = useCarrdStore()
    await s.openProject('art')
    expect(s.selectedFolder).toBe('art')
    expect(s.currentPath).toBe('')
    expect(callsTo('GET', '/carrd-projects/{folder}/files')[0]![1]).toMatchObject({
      params: { path: { folder: 'art' }, query: { path: '' } },
    })
  })

  it('navigate moves to a subpath and reloads contents', async () => {
    const s = useCarrdStore()
    await s.openProject('art')
    api.GET.mockClear()
    await s.navigate('chapter1')
    expect(s.currentPath).toBe('chapter1')
    expect(callsTo('GET', '/carrd-projects/{folder}/files')[0]![1]).toMatchObject({
      params: { path: { folder: 'art' }, query: { path: 'chapter1' } },
    })
  })
})

describe('createProject', () => {
  it('rejects a blank title', async () => {
    const s = useCarrdStore()
    expect(await s.createProject('   ')).toBeNull()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates, reloads, and opens the new project', async () => {
    const s = useCarrdStore()
    const folder = await s.createProject('  Art  ')
    expect(callsTo('POST', '/carrd-projects')[0]![1]).toMatchObject({
      body: { title: 'Art', folder: '' },
    })
    expect(folder).toBe('art')
    expect(s.selectedFolder).toBe('art')
  })
})

describe('createDir', () => {
  it('requires an open project', async () => {
    const s = useCarrdStore()
    expect(await s.createDir('sub')).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('rejects a blank folder name', async () => {
    const s = useCarrdStore()
    s.selectedFolder = 'art'
    expect(await s.createDir('   ')).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('createDir (open project)', () => {
  it('makes the folder under the current path', async () => {
    const s = useCarrdStore()
    await s.openProject('art')
    await s.navigate('chapter1')
    expect(await s.createDir('  Panels ')).toBe(true)
    expect(callsTo('POST', '/carrd-projects/{folder}/folders')[0]![1]).toMatchObject({
      params: { path: { folder: 'art' } },
      body: { path: 'chapter1', name: 'Panels' },
    })
  })
})
