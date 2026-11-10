import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { StyleList } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, response }`.
const ok = (data: unknown, status = 200) => ({
  data,
  response: new Response(null, { status }),
})
const theme = { css: ':root{--accent:#fff}', boardFlourish: 'b.svg', numberFlourish: 'n.svg' }
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))
const applyCustomCSS = vi.hoisted(() => vi.fn())
vi.mock('@/lib/theme', () => ({ applyCustomCSS }))

import { useStylesStore } from '@/stores/styles'
import { useUiStore } from '@/stores/ui'
import { useAppStore } from '@/stores/app'

const list = (over: Partial<StyleList> = {}) =>
  ok({ styles: [], activeStyleId: null, ...over } satisfies StyleList)

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  applyCustomCSS.mockReset()
  api.GET.mockResolvedValue(list())
})

describe('loading', () => {
  it('loadStyles populates list + active id and clears the editor', async () => {
    api.GET.mockResolvedValueOnce(
      list({
        styles: [{ id: 1, name: 'Dusk', isPublic: false, createdAt: '2026-11-09T10:00:00Z' }],
        activeStyleId: 1,
      }),
    )
    const s = useStylesStore()
    s.newStyle()
    await s.loadStyles()
    expect(api.GET).toHaveBeenCalledWith('/styles')
    expect(s.styles).toHaveLength(1)
    expect(s.activeStyleId).toBe(1)
    expect(s.editingStyle).toBeNull()
  })

  it('loadStyle merges saved tokens over the defaults', async () => {
    api.GET.mockResolvedValueOnce(
      ok({
        id: 2,
        name: 'Dusk',
        tokens: { 'page-bg': '#abc' },
        css: '',
        boardFlourish: '',
        numberFlourish: '',
        isPublic: true,
        createdAt: null,
      }),
    )
    const s = useStylesStore()
    await s.loadStyle(2)
    expect(api.GET).toHaveBeenCalledWith('/styles/{id}', { params: { path: { id: 2 } } })
    expect(s.editingStyle?.tokens['page-bg']).toBe('#abc') // saved override
    expect(s.editingStyle?.tokens['panel-bg']).toBe('#272a22') // default filled in
    expect(s.editingStyle?.isPublic).toBe(true)
  })
})

describe('newStyle', () => {
  it('seeds a blank theme with the default token set', () => {
    const s = useStylesStore()
    s.newStyle()
    expect(s.editingStyle).toMatchObject({ id: 0, name: '' })
    expect(s.editingStyle?.tokens.accent).toBe('#d6bdae')
  })
})

describe('saveStyle', () => {
  it('rejects a blank name', async () => {
    const s = useStylesStore()
    s.newStyle()
    await s.saveStyle()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates a new theme with its tokens, then reloads (closing the editor)', async () => {
    api.POST.mockResolvedValueOnce(ok({ id: 11 }, 201))
    const s = useStylesStore()
    s.newStyle()
    s.editingStyle!.name = ' Midnight '
    s.editingStyle!.tokens['page-bg'] = '#000'
    await s.saveStyle()
    expect(api.POST).toHaveBeenCalledTimes(1)
    const [path, init] = api.POST.mock.calls[0]!
    expect(path).toBe('/styles')
    expect(init.body).toMatchObject({
      name: 'Midnight',
      boardFlourish: '',
      numberFlourish: '',
      isPublic: false,
    })
    expect(init.body.tokens['page-bg']).toBe('#000')
    expect(init.body).not.toHaveProperty('id')
    expect(api.GET).toHaveBeenCalledWith('/styles')
    expect(s.editingStyle).toBeNull()
  })

  it('updates an existing theme with its tokens', async () => {
    api.PUT.mockResolvedValueOnce(ok({}))
    const s = useStylesStore()
    s.editingStyle = {
      id: 5,
      name: 'Old',
      tokens: { 'page-bg': '#111' },
      boardFlourish: '',
      numberFlourish: '',
      isPublic: false,
    }
    s.editingStyle.name = 'New'
    await s.saveStyle()
    expect(api.PUT).toHaveBeenCalledWith('/styles/{id}', {
      params: { path: { id: 5 } },
      body: {
        name: 'New',
        tokens: { 'page-bg': '#111' },
        boardFlourish: '',
        numberFlourish: '',
        isPublic: false,
      },
    })
    expect(api.POST).not.toHaveBeenCalled()
  })

  it("shows the server's reason when a save is refused", async () => {
    api.PUT.mockResolvedValueOnce({
      error: { status: 422, title: 'Unprocessable Entity', detail: 'validation failed' },
      response: new Response(null, { status: 422 }),
    })
    const notify = vi.spyOn(useUiStore(), 'notify')
    const s = useStylesStore()
    s.newStyle()
    s.editingStyle!.id = 5
    s.editingStyle!.name = 'Bad'
    await s.saveStyle()
    expect(notify).toHaveBeenCalledWith('validation failed', 'error')
    expect(s.savingStyle).toBe(false)
  })
})

describe('deleteStyle', () => {
  it('skips when cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = useStylesStore()
    await s.deleteStyle(1)
    expect(api.DELETE).not.toHaveBeenCalled()
  })

  it('deletes and clears the editor when the open theme is removed', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    api.DELETE.mockResolvedValueOnce(ok(undefined, 204))
    const s = useStylesStore()
    s.newStyle()
    s.editingStyle!.id = 7
    await s.deleteStyle(7)
    expect(api.DELETE).toHaveBeenCalledWith('/styles/{id}', { params: { path: { id: 7 } } })
    expect(s.editingStyle).toBeNull()
  })
})

describe('setActiveStyle', () => {
  it('activates a theme and applies the stylesheet the server answers with', async () => {
    api.POST.mockResolvedValueOnce(ok({ styleId: 3, ...theme }))
    const s = useStylesStore()
    const flourishes = vi
      .spyOn(useAppStore(), 'applyFlourishes')
      .mockImplementation(() => undefined)
    await s.setActiveStyle(3)
    expect(api.POST).toHaveBeenCalledWith('/styles/{id}/activate', { params: { path: { id: 3 } } })
    expect(s.activeStyleId).toBe(3)
    expect(applyCustomCSS).toHaveBeenCalledWith(theme.css)
    expect(flourishes).toHaveBeenCalledWith('b.svg', 'n.svg')
  })

  it('goes back to the default look with null', async () => {
    api.POST.mockResolvedValueOnce(ok({ css: '', boardFlourish: '', numberFlourish: '' }))
    const s = useStylesStore()
    s.activeStyleId = 3
    await s.setActiveStyle(null)
    expect(api.POST).toHaveBeenCalledWith('/styles/deactivate')
    expect(s.activeStyleId).toBeNull()
    expect(applyCustomCSS).toHaveBeenCalledWith('')
  })

  it('leaves the active theme alone when the server refuses', async () => {
    api.POST.mockResolvedValueOnce({
      error: { status: 404, title: 'Not Found', detail: 'There is no theme 9.' },
      response: new Response(null, { status: 404 }),
    })
    const s = useStylesStore()
    s.activeStyleId = 3
    await s.setActiveStyle(9)
    expect(s.activeStyleId).toBe(3)
    expect(applyCustomCSS).not.toHaveBeenCalled()
  })
})
