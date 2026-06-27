import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { FontFile } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ fonts: [] as FontFile[] })),
  upload: vi.fn(async () => ({ uploaded: ['a.ttf'], skipped: [] })),
  del: vi.fn(async () => ({ ok: true })),
  rename: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { fonts: { list: ep.list, upload: ep.upload, delete: ep.del, rename: ep.rename } },
}))

import { useFontsStore, fontUrl } from './fonts'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('fontUrl', () => {
  it('builds an encoded public URL', () => {
    expect(fontUrl('My Font.ttf')).toBe('https://fonts.senpan.cafe/My%20Font.ttf')
  })
})

describe('loadFonts', () => {
  it('populates the font list', async () => {
    ep.list.mockResolvedValueOnce({ fonts: [{ name: 'a.ttf' } as FontFile] })
    const s = useFontsStore()
    await s.loadFonts()
    expect(s.fonts).toHaveLength(1)
    expect(s.loading).toBe(false)
  })
})

describe('uploadFonts', () => {
  it('does nothing for an empty selection', async () => {
    const s = useFontsStore()
    await s.uploadFonts([])
    expect(ep.upload).not.toHaveBeenCalled()
  })

  it('uploads files and reloads the list', async () => {
    const s = useFontsStore()
    await s.uploadFonts([new File(['x'], 'a.ttf')])
    expect(ep.upload).toHaveBeenCalledTimes(1)
    expect(ep.list).toHaveBeenCalled()
  })
})

describe('renameFont', () => {
  it('no-ops on a blank or unchanged name', async () => {
    const s = useFontsStore()
    expect(await s.renameFont('a.ttf', '   ')).toBe(false)
    expect(await s.renameFont('a.ttf', 'a.ttf')).toBe(false)
    expect(ep.rename).not.toHaveBeenCalled()
  })

  it('renames and reloads on success', async () => {
    const s = useFontsStore()
    expect(await s.renameFont('a.ttf', 'b.ttf')).toBe(true)
    expect(ep.rename).toHaveBeenCalledWith('a.ttf', 'b.ttf')
    expect(ep.list).toHaveBeenCalled()
  })
})

describe('deleteFont', () => {
  it('skips when the confirm is cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = useFontsStore()
    await s.deleteFont('a.ttf')
    expect(ep.del).not.toHaveBeenCalled()
  })

  it('deletes and reloads when confirmed', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = useFontsStore()
    await s.deleteFont('a.ttf')
    expect(ep.del).toHaveBeenCalledWith('a.ttf')
    expect(ep.list).toHaveBeenCalled()
  })
})
