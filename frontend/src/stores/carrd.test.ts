import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { CarrdProject } from '@/types/api'

const ep = vi.hoisted(() => ({
  projects: vi.fn(async () => ({ projects: [] as CarrdProject[] })),
  images: vi.fn(async () => ({ dirs: [] as string[], images: [] })),
  createProject: vi.fn(async () => ({ project: { folder: 'art', title: 'Art' } })),
  createDir: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    carrd: {
      projects: ep.projects,
      images: ep.images,
      createProject: ep.createProject,
      createDir: ep.createDir,
    },
  },
}))

import { useCarrdStore, carrdImageUrl, joinCarrdPath } from './carrd'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
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
    ep.projects.mockResolvedValueOnce({ projects: [{ folder: 'other' } as CarrdProject] })
    const s = useCarrdStore()
    s.selectedFolder = 'gone'
    s.currentPath = 'sub'
    await s.loadProjects()
    expect(s.selectedFolder).toBeNull()
    expect(s.currentPath).toBe('')
  })

  it('keeps a selection that still exists', async () => {
    ep.projects.mockResolvedValueOnce({ projects: [{ folder: 'keep' } as CarrdProject] })
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
    expect(ep.images).toHaveBeenCalledWith('art', '')
  })

  it('navigate moves to a subpath and reloads contents', async () => {
    const s = useCarrdStore()
    await s.openProject('art')
    ep.images.mockClear()
    await s.navigate('chapter1')
    expect(s.currentPath).toBe('chapter1')
    expect(ep.images).toHaveBeenCalledWith('art', 'chapter1')
  })
})

describe('createProject', () => {
  it('rejects a blank title', async () => {
    const s = useCarrdStore()
    expect(await s.createProject('   ')).toBeNull()
    expect(ep.createProject).not.toHaveBeenCalled()
  })

  it('creates, reloads, and opens the new project', async () => {
    const s = useCarrdStore()
    const folder = await s.createProject('  Art  ')
    expect(ep.createProject).toHaveBeenCalledWith('Art', '')
    expect(folder).toBe('art')
    expect(s.selectedFolder).toBe('art')
  })
})

describe('createDir', () => {
  it('requires an open project', async () => {
    const s = useCarrdStore()
    expect(await s.createDir('sub')).toBe(false)
    expect(ep.createDir).not.toHaveBeenCalled()
  })

  it('rejects a blank folder name', async () => {
    const s = useCarrdStore()
    s.selectedFolder = 'art'
    expect(await s.createDir('   ')).toBe(false)
    expect(ep.createDir).not.toHaveBeenCalled()
  })
})
