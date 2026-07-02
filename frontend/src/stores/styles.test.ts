import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Style } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ styles: [] as Style[], active_style_id: '' })),
  get: vi.fn(async () => ({ style: { id: 1, tokens: {} } })),
  create: vi.fn(async () => ({ id: 11 })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    styles: { list: ep.list, get: ep.get, create: ep.create, update: ep.update, delete: ep.del },
  },
}))
vi.mock('@/lib/theme', () => ({ applyCustomCSS: vi.fn() }))

import { useStylesStore } from './styles'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('loading', () => {
  it('loadStyles populates list + active id and clears the editor', async () => {
    ep.list.mockResolvedValueOnce({ styles: [{ id: 1 } as Style], active_style_id: '1' })
    const s = useStylesStore()
    s.editingStyle = { id: 9 } as Style
    await s.loadStyles()
    expect(s.styles).toHaveLength(1)
    expect(s.activeStyleId).toBe('1')
    expect(s.editingStyle).toBeNull()
  })

  it('loadStyle merges saved tokens over the defaults', async () => {
    ep.get.mockResolvedValueOnce({ style: { id: 2, tokens: { 'page-bg': '#abc' } } })
    const s = useStylesStore()
    await s.loadStyle(2)
    expect(s.editingStyle?.tokens?.['page-bg']).toBe('#abc') // saved override
    expect(s.editingStyle?.tokens?.['panel-bg']).toBe('#272a22') // default filled in
  })
})

describe('newStyle', () => {
  it('seeds a blank theme with the default token set', () => {
    const s = useStylesStore()
    s.newStyle()
    expect(s.editingStyle).toMatchObject({ id: 0, name: '' })
    expect(s.editingStyle?.tokens?.['accent']).toBe('#d6bdae')
  })
})

describe('saveStyle', () => {
  it('rejects a blank name', async () => {
    const s = useStylesStore()
    s.newStyle()
    await s.saveStyle()
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates a new theme with its tokens, then reloads (closing the editor)', async () => {
    const s = useStylesStore()
    s.newStyle()
    s.editingStyle!.name = 'Midnight'
    s.editingStyle!.tokens!['page-bg'] = '#000'
    await s.saveStyle()
    expect(ep.create).toHaveBeenCalledTimes(1)
    const [name, tokens, board, number] = ep.create.mock.calls[0] as unknown as [
      string,
      Record<string, string>,
      string,
      string,
    ]
    expect(name).toBe('Midnight')
    expect(tokens['page-bg']).toBe('#000')
    expect(board).toBe('')
    expect(number).toBe('')
    expect(ep.list).toHaveBeenCalled()
    expect(s.editingStyle).toBeNull()
  })

  it('updates an existing theme with its tokens', async () => {
    const s = useStylesStore()
    s.editingStyle = {
      id: 5,
      name: 'Old',
      tokens: { 'page-bg': '#111' },
      board_flourish: '',
      number_flourish: '',
      created_at: '',
    }
    s.editingStyle.name = 'New'
    await s.saveStyle()
    expect(ep.update).toHaveBeenCalledWith(5, 'New', { 'page-bg': '#111' }, '', '')
    expect(ep.create).not.toHaveBeenCalled()
  })
})

describe('deleteStyle', () => {
  it('skips when cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = useStylesStore()
    await s.deleteStyle(1)
    expect(ep.del).not.toHaveBeenCalled()
  })

  it('deletes and clears the editor when the open theme is removed', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = useStylesStore()
    s.editingStyle = { id: 7 } as Style
    await s.deleteStyle(7)
    expect(ep.del).toHaveBeenCalledWith(7)
    expect(s.editingStyle).toBeNull()
  })
})
