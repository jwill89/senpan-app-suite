import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Style } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ styles: [] as Style[], active_style_id: '' })),
  get: vi.fn(async () => ({ style: { id: 1, css_content: null } })),
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

  it('loadStyle coerces null css_content to an empty string for the editor', async () => {
    ep.get.mockResolvedValueOnce({ style: { id: 2, css_content: null } as never })
    const s = useStylesStore()
    await s.loadStyle(2)
    expect(s.editingStyle?.css_content).toBe('')
  })
})

describe('newStyle', () => {
  it('seeds a blank editable theme', () => {
    const s = useStylesStore()
    s.newStyle()
    expect(s.editingStyle).toMatchObject({ id: 0, name: '', css_content: '' })
  })
})

describe('saveStyle', () => {
  it('rejects a blank name', async () => {
    const s = useStylesStore()
    s.newStyle()
    await s.saveStyle()
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates a new theme then reloads (which closes the editor)', async () => {
    const s = useStylesStore()
    s.newStyle()
    s.editingStyle!.name = 'Midnight'
    s.editingStyle!.css_content = ':root{}'
    await s.saveStyle()
    expect(ep.create).toHaveBeenCalledWith('Midnight', ':root{}', '', '')
    expect(ep.list).toHaveBeenCalled()
    // loadStyles() runs after a successful save and clears the editor.
    expect(s.editingStyle).toBeNull()
  })

  it('updates an existing theme', async () => {
    const s = useStylesStore()
    s.editingStyle = {
      id: 5, name: 'Old', css_content: 'a', board_flourish: '', number_flourish: '', created_at: '',
    }
    s.editingStyle.name = 'New'
    await s.saveStyle()
    expect(ep.update).toHaveBeenCalledWith(5, 'New', 'a', '', '')
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
