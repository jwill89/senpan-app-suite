import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { GamePreset } from '@/types/api'

const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ presets: [] as GamePreset[] })),
  create: vi.fn(async () => ({ id: 1 })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { presets: { list: ep.list, create: ep.create, update: ep.update, delete: ep.del } },
}))

import { usePresetsStore } from './presets'
import { useUiStore } from './ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('loading and editor', () => {
  it('loadPresets populates the list', async () => {
    ep.list.mockResolvedValueOnce({ presets: [{ id: 1 } as GamePreset] })
    const s = usePresetsStore()
    await s.loadPresets()
    expect(s.presets).toHaveLength(1)
  })

  it('newPreset opens a blank form', () => {
    const s = usePresetsStore()
    s.newPreset()
    expect(s.editingPreset).toEqual({
      id: 0,
      name: '',
      pattern_ids: [],
      game_details: '',
      auto: false,
      auto_interval: 30,
    })
  })

  it('editPreset copies values (array is cloned, not shared)', () => {
    const s = usePresetsStore()
    const preset = { id: 3, name: 'P', pattern_ids: [1, 2], game_details: 'GL' } as GamePreset
    s.editPreset(preset)
    s.editingPreset!.pattern_ids.push(99)
    expect(preset.pattern_ids).toEqual([1, 2]) // original untouched
  })
})

describe('savePreset', () => {
  it('rejects a blank name', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.pattern_ids = [1]
    await s.savePreset()
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('rejects when no patterns are selected', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = 'Has Name'
    await s.savePreset()
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates a new preset then reloads', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = 'Quick'
    s.editingPreset!.pattern_ids = [5]
    await s.savePreset()
    expect(ep.create).toHaveBeenCalledWith('Quick', [5], '', false, 30)
    expect(ep.list).toHaveBeenCalled()
    expect(s.editingPreset).toBeNull()
  })

  it('creates a preset with its auto-draw config', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = 'Auto'
    s.editingPreset!.pattern_ids = [5]
    s.editingPreset!.auto = true
    s.editingPreset!.auto_interval = 45
    await s.savePreset()
    expect(ep.create).toHaveBeenCalledWith('Auto', [5], '', true, 45)
  })

  it('updates an existing preset', async () => {
    const s = usePresetsStore()
    s.editPreset({
      id: 8,
      name: 'Old',
      pattern_ids: [1],
      game_details: '',
      auto: true,
      auto_interval: 20,
    } as GamePreset)
    s.editingPreset!.name = 'New'
    await s.savePreset()
    expect(ep.update).toHaveBeenCalledWith(8, 'New', [1], '', true, 20)
    expect(ep.create).not.toHaveBeenCalled()
  })
})

describe('deletePreset', () => {
  it('skips when the confirm is cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = usePresetsStore()
    await s.deletePreset(1)
    expect(ep.del).not.toHaveBeenCalled()
  })

  it('deletes and clears the editor if the deleted preset was open', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = usePresetsStore()
    s.editPreset({ id: 4, name: 'X', pattern_ids: [1], game_details: '' } as GamePreset)
    await s.deletePreset(4)
    expect(ep.del).toHaveBeenCalledWith(4)
    expect(s.editingPreset).toBeNull()
    expect(ep.list).toHaveBeenCalled()
  })
})
