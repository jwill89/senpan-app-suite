import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Font } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real one).
// Each fake answers a call's `{ data, error, response }`. Uploads go through the XHR
// helper, faked the same way.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
const apiUpload = vi.hoisted(() => vi.fn())
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  apiUpload,
}))

/** The calls of a method made to a path, as `[path, init]`. */
function callsTo(method: 'GET' | 'PATCH' | 'DELETE', path: string) {
  return api[method].mock.calls.filter(([p]) => p === path) as [string, Record<string, unknown>][]
}

import { useFontsStore, fontShareUrl, toUploadedFont, FONT_KIT_URL } from '@/stores/fonts'
import { useUiStore } from '@/stores/ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  apiUpload.mockReset()
  api.GET.mockResolvedValue(ok({ fonts: [] }))
  api.PATCH.mockResolvedValue(ok({}))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
  apiUpload.mockResolvedValue({ uploaded: ['a.ttf'], skipped: [], warnings: [] })
})

/** A representative grouped font row. */
const jasper: Font = {
  base: 'Jasper',
  family: 'Jasper Fancy',
  serve: '',
  servedType: 'WOFF2',
  servedToken: 'conv.woff2',
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
    api.GET.mockResolvedValueOnce(ok({ fonts: [jasper] }))
    const s = useFontsStore()
    await s.loadFonts()
    expect(s.fonts).toHaveLength(1)
    expect(s.fonts[0]!.base).toBe('Jasper')
    expect(s.loading).toBe(false)
  })
})

describe('uploadFonts', () => {
  it('does nothing for an empty selection', async () => {
    const s = useFontsStore()
    await s.uploadFonts([])
    expect(apiUpload).not.toHaveBeenCalled()
  })

  it('uploads files and reloads the list', async () => {
    const s = useFontsStore()
    await s.uploadFonts([new File(['x'], 'a.ttf')])
    expect(apiUpload).toHaveBeenCalledTimes(1)
    expect(apiUpload.mock.calls[0]![0]).toBe('fonts')
    expect(callsTo('GET', '/fonts')).toHaveLength(1)
  })
})

describe('renameFile', () => {
  it('no-ops on a blank or unchanged name', async () => {
    const s = useFontsStore()
    expect(await s.renameFile('a.ttf', '   ')).toBe(false)
    expect(await s.renameFile('a.ttf', 'a.ttf')).toBe(false)
    expect(api.PATCH).not.toHaveBeenCalled()
  })

  it('renames and reloads on success', async () => {
    const s = useFontsStore()
    expect(await s.renameFile('a.ttf', 'b.ttf')).toBe(true)
    expect(callsTo('PATCH', '/font-files/{name}')[0]![1]).toMatchObject({
      params: { path: { name: 'a.ttf' } },
      body: { name: 'b.ttf' },
    })
    expect(callsTo('GET', '/fonts')).toHaveLength(1)
  })
})

describe('updateFamily', () => {
  it('sends the supplied fields and reloads on success', async () => {
    const s = useFontsStore()
    const fields = { family: 'Fancy', serve: 'TTF', origins: ['https://x.example.com'] }
    expect(await s.updateFamily('Jasper', fields)).toBe(true)
    expect(callsTo('PATCH', '/fonts/{base}')[0]![1]).toMatchObject({
      params: { path: { base: 'Jasper' } },
      body: fields,
    })
    expect(callsTo('GET', '/fonts')).toHaveLength(1)
    expect(s.saving).toBe(false)
  })

  it('reports failure without throwing', async () => {
    api.PATCH.mockResolvedValueOnce({
      error: { title: 'Conflict', detail: 'Another font is already named "Dup"' },
      response: new Response(null, { status: 409 }),
    })
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
    expect(api.DELETE).not.toHaveBeenCalled()
  })

  it('deletes the whole font (by base) when confirmed', async () => {
    const confirm = vi.fn(async (_message: string, _options?: unknown) => true)
    useUiStore().confirm = confirm
    const s = useFontsStore()
    await s.deleteFont(jasper)
    expect(callsTo('DELETE', '/fonts/{base}')[0]![1]).toMatchObject({
      params: { path: { base: 'Jasper' } },
    })
    // The confirm names the uploaded files, not the converted copy.
    expect(confirm).toHaveBeenCalledWith(expect.stringContaining('Jasper.ttf'), expect.anything())
    expect(callsTo('GET', '/fonts')).toHaveLength(1)
  })
})

describe('deleteFile', () => {
  it('deletes a single variant file when confirmed', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = useFontsStore()
    await s.deleteFile('Jasper.ttf')
    expect(callsTo('DELETE', '/font-files/{name}')[0]![1]).toMatchObject({
      params: { path: { name: 'Jasper.ttf' } },
    })
  })
})
