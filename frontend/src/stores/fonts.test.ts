import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Font } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ fonts: [] as Font[] })),
  upload: vi.fn(async () => ({ uploaded: ['a.ttf'], skipped: [] })),
  deleteFile: vi.fn(async () => ({ ok: true })),
  deleteFont: vi.fn(async () => ({ ok: true })),
  renameFile: vi.fn(async () => ({ ok: true })),
  updateFamily: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    fonts: {
      list: ep.list,
      upload: ep.upload,
      deleteFile: ep.deleteFile,
      deleteFont: ep.deleteFont,
      renameFile: ep.renameFile,
      updateFamily: ep.updateFamily,
    },
  },
}))

import { useFontsStore, fontShareUrl, toUploadedFont, FONT_KIT_URL } from './fonts'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

/** A representative grouped font row. */
const jasper: Font = {
  base: 'Jasper',
  family: 'Jasper Fancy',
  serve: '',
  served_type: 'WOFF2',
  served_token: 'conv.woff2',
  origins: ['https://mysite.carrd.co'],
  modified: '2026-07-02T00:00:00Z',
  variants: [
    {
      name: 'Jasper.ttf',
      type: 'TTF',
      converted: false,
      size: 100,
      modified: '2026-07-02T00:00:00Z',
      token: 'orig.ttf',
    },
    {
      name: 'Jasper.woff2',
      type: 'WOFF2',
      converted: true,
      size: 40,
      modified: '2026-07-02T00:00:00Z',
      token: 'conv.woff2',
    },
  ],
}

describe('font URLs', () => {
  it('builds an encoded tokenized share URL', () => {
    expect(fontShareUrl('AbC-123.woff2')).toBe('https://fonts.senpan.cafe/f/AbC-123.woff2')
  })

  it('exposes the kit stylesheet URL', () => {
    expect(FONT_KIT_URL).toBe('https://fonts.senpan.cafe/kit.css')
  })
})

describe('toUploadedFont', () => {
  it('registers the served variant under the CSS family', () => {
    expect(toUploadedFont(jasper)).toEqual({
      name: 'Jasper',
      family: 'Jasper Fancy',
      token: 'conv.woff2',
    })
  })
})

describe('loadFonts', () => {
  it('populates the grouped font list', async () => {
    ep.list.mockResolvedValueOnce({ fonts: [jasper] })
    const s = useFontsStore()
    await s.loadFonts()
    expect(s.fonts).toHaveLength(1)
    expect(s.fonts[0].base).toBe('Jasper')
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

describe('renameFile', () => {
  it('no-ops on a blank or unchanged name', async () => {
    const s = useFontsStore()
    expect(await s.renameFile('a.ttf', '   ')).toBe(false)
    expect(await s.renameFile('a.ttf', 'a.ttf')).toBe(false)
    expect(ep.renameFile).not.toHaveBeenCalled()
  })

  it('renames and reloads on success', async () => {
    const s = useFontsStore()
    expect(await s.renameFile('a.ttf', 'b.ttf')).toBe(true)
    expect(ep.renameFile).toHaveBeenCalledWith('a.ttf', 'b.ttf')
    expect(ep.list).toHaveBeenCalled()
  })
})

describe('updateFamily', () => {
  it('sends the supplied fields and reloads on success', async () => {
    const s = useFontsStore()
    const fields = { family: 'Fancy', serve: 'TTF', origins: ['https://x.example.com'] }
    expect(await s.updateFamily('Jasper', fields)).toBe(true)
    expect(ep.updateFamily).toHaveBeenCalledWith('Jasper', fields)
    expect(ep.list).toHaveBeenCalled()
    expect(s.saving).toBe(false)
  })

  it('reports failure without throwing', async () => {
    ep.updateFamily.mockRejectedValueOnce(new Error('conflict'))
    const s = useFontsStore()
    expect(await s.updateFamily('Jasper', { family: 'Dup' })).toBe(false)
    expect(s.saving).toBe(false)
  })
})

describe('deleteFont', () => {
  it('skips when the confirm is cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = useFontsStore()
    await s.deleteFont(jasper)
    expect(ep.deleteFont).not.toHaveBeenCalled()
  })

  it('deletes the whole font (by base) when confirmed', async () => {
    const confirm = vi.fn(async (_message: string, _options?: unknown) => true)
    useUiStore().confirm = confirm
    const s = useFontsStore()
    await s.deleteFont(jasper)
    expect(ep.deleteFont).toHaveBeenCalledWith('Jasper')
    // The confirm names the uploaded files, not the converted copy.
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('Jasper.ttf'), expect.anything())
    expect(ep.list).toHaveBeenCalled()
  })
})

describe('deleteFile', () => {
  it('deletes a single variant file when confirmed', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = useFontsStore()
    await s.deleteFile('Jasper.ttf')
    expect(ep.deleteFile).toHaveBeenCalledWith('Jasper.ttf')
  })
})
