import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { GamePreset } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { usePresetsStore } from './presets'
import { useUiStore } from './ui'

function preset(overrides: Partial<GamePreset> = {}): GamePreset {
  return {
    id: 3,
    name: 'P',
    patternIds: [1, 2],
    gameDetails: 'GL',
    auto: false,
    autoInterval: 30,
    createdAt: '2026-11-09T10:00:00Z',
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockResolvedValue(ok({ presets: [] }))
  api.POST.mockResolvedValue(ok(preset(), 201))
  api.PUT.mockResolvedValue(ok(preset()))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

describe('loading and editor', () => {
  it('loadPresets populates the list', async () => {
    api.GET.mockResolvedValueOnce(ok({ presets: [preset({ id: 1 })] }))
    const s = usePresetsStore()
    await s.loadPresets()
    expect(api.GET).toHaveBeenCalledWith('/presets')
    expect(s.presets).toHaveLength(1)
  })

  it('newPreset opens a blank form', () => {
    const s = usePresetsStore()
    s.newPreset()
    expect(s.editingPreset).toEqual({
      id: 0,
      name: '',
      patternIds: [],
      gameDetails: '',
      auto: false,
      autoInterval: 30,
    })
  })

  it('editPreset copies values (array is cloned, not shared)', () => {
    const s = usePresetsStore()
    const original = preset()
    s.editPreset(original)
    s.editingPreset!.patternIds.push(99)
    expect(original.patternIds).toEqual([1, 2]) // original untouched
  })
})

describe('savePreset', () => {
  it('rejects a blank name', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.patternIds = [1]
    await s.savePreset()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('rejects when no patterns are selected', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = 'Has Name'
    await s.savePreset()
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates a new preset then reloads', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = ' Quick '
    s.editingPreset!.patternIds = [5]
    await s.savePreset()
    expect(api.POST).toHaveBeenCalledWith('/presets', {
      body: { name: 'Quick', patternIds: [5], gameDetails: '', auto: false, autoInterval: 30 },
    })
    expect(api.GET).toHaveBeenCalledWith('/presets')
    expect(s.editingPreset).toBeNull()
  })

  it('creates a preset with its auto-draw config', async () => {
    const s = usePresetsStore()
    s.newPreset()
    s.editingPreset!.name = 'Auto'
    s.editingPreset!.patternIds = [5]
    s.editingPreset!.auto = true
    s.editingPreset!.autoInterval = 45
    await s.savePreset()
    expect(api.POST).toHaveBeenCalledWith('/presets', {
      body: { name: 'Auto', patternIds: [5], gameDetails: '', auto: true, autoInterval: 45 },
    })
  })

  it('replaces an existing preset', async () => {
    const s = usePresetsStore()
    s.editPreset(
      preset({
        id: 8,
        name: 'Old',
        patternIds: [1],
        gameDetails: '',
        auto: true,
        autoInterval: 20,
      }),
    )
    s.editingPreset!.name = 'New'
    await s.savePreset()
    expect(api.PUT).toHaveBeenCalledWith('/presets/{id}', {
      params: { path: { id: 8 } },
      body: { name: 'New', patternIds: [1], gameDetails: '', auto: true, autoInterval: 20 },
    })
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('keeps the editor open and says why when the save is refused', async () => {
    const s = usePresetsStore()
    s.editPreset(preset())
    api.PUT.mockResolvedValueOnce({
      error: { status: 404, detail: 'There is no such preset.' },
      response: new Response(null, { status: 404 }),
    })
    const ui = useUiStore()
    ui.notify = vi.fn()
    await s.savePreset()
    expect(ui.notify).toHaveBeenCalledWith('There is no such preset.', 'error')
    expect(s.editingPreset).not.toBeNull()
  })
})

describe('deletePreset', () => {
  it('skips when the confirm is cancelled', async () => {
    useUiStore().confirm = vi.fn(async () => false)
    const s = usePresetsStore()
    await s.deletePreset(1)
    expect(api.DELETE).not.toHaveBeenCalled()
  })

  it('deletes and clears the editor if the deleted preset was open', async () => {
    useUiStore().confirm = vi.fn(async () => true)
    const s = usePresetsStore()
    s.editPreset(preset({ id: 4 }))
    await s.deletePreset(4)
    expect(api.DELETE).toHaveBeenCalledWith('/presets/{id}', { params: { path: { id: 4 } } })
    expect(s.editingPreset).toBeNull()
    expect(api.GET).toHaveBeenCalled()
  })
})
