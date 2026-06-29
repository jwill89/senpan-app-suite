import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Affiliate } from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ affiliates: [] as Affiliate[] })),
  create: vi.fn(async () => ({ ok: true })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    affiliates: {
      list: ep.list,
      create: ep.create,
      update: ep.update,
      delete: ep.del,
    },
  },
}))

import { useAffiliatesStore } from './affiliates'
import { useUiStore } from './ui'

/** Minimal affiliate with the fields the store reads. */
function affiliate(over: Partial<Affiliate> = {}): Affiliate {
  return {
    id: 1,
    name: 'A',
    owners: [],
    location: '',
    timezone: 'UTC',
    hours: [],
    details: '',
    logo: '',
    screenshot: '',
    created_at: '',
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('loading', () => {
  it('loadAffiliates populates the list', async () => {
    ep.list.mockResolvedValueOnce({ affiliates: [affiliate(), affiliate({ id: 2 })] })
    const s = useAffiliatesStore()
    await s.loadAffiliates()
    expect(s.affiliates).toHaveLength(2)
    expect(s.affiliatesLoading).toBe(false)
  })
})

describe('form rows', () => {
  it('add/remove owner keeps at least one row', () => {
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.addOwner()
    expect(s.affiliateForm!.owners).toHaveLength(2)
    s.removeOwner(0)
    expect(s.affiliateForm!.owners).toHaveLength(1)
    s.removeOwner(0) // refuses to drop the last row
    expect(s.affiliateForm!.owners).toHaveLength(1)
  })

  it('add/remove hour keeps at least one row', () => {
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.addHour()
    expect(s.affiliateForm!.hours).toHaveLength(2)
    s.removeHour(0)
    expect(s.affiliateForm!.hours).toHaveLength(1)
    s.removeHour(0)
    expect(s.affiliateForm!.hours).toHaveLength(1)
  })

  it('editAffiliateForm seeds the form from an affiliate', () => {
    const s = useAffiliatesStore()
    s.editAffiliateForm(
      affiliate({ id: 7, name: 'Tavern', owners: ['Tataru'], hours: [{ label: '', start: '09:00', end: '' }] }),
    )
    expect(s.affiliateForm).toMatchObject({ id: 7, name: 'Tavern', owners: ['Tataru'] })
    expect(s.affiliateForm!.hours).toHaveLength(1)
  })
})

describe('saveAffiliate', () => {
  it('requires a name', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useAffiliatesStore()
    s.newAffiliateForm() // name is blank
    const ok = await s.saveAffiliate()
    expect(ok).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('requires at least one owner', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.affiliateForm!.name = 'Tavern'
    s.affiliateForm!.owners = ['  '] // blanks only
    const ok = await s.saveAffiliate()
    expect(ok).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates with blank owners/hours stripped', async () => {
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.affiliateForm!.name = 'Tavern'
    s.affiliateForm!.owners = ['Tataru', '  ', 'Hildibrand']
    s.affiliateForm!.hours = [
      { label: 'Mon–Fri', start: '18:00', end: '23:00' },
      { label: 'blank', start: '  ', end: '' }, // dropped (no start)
    ]
    const ok = await s.saveAffiliate()
    expect(ok).toBe(true)
    expect(ep.create).toHaveBeenCalledTimes(1)
    const payload = (ep.create.mock.calls[0] as unknown[])[0] as {
      owners: string[]
      hours: unknown[]
    }
    expect(payload.owners).toEqual(['Tataru', 'Hildibrand'])
    expect(payload.hours).toHaveLength(1)
    expect(s.affiliateForm).toBeNull()
  })

  it('updates when the form carries an id', async () => {
    const s = useAffiliatesStore()
    s.editAffiliateForm(affiliate({ id: 4, name: 'Tavern', owners: ['Solo'] }))
    const ok = await s.saveAffiliate()
    expect(ok).toBe(true)
    expect(ep.update).toHaveBeenCalledTimes(1)
    expect(ep.create).not.toHaveBeenCalled()
  })
})

describe('deleteAffiliate', () => {
  it('does nothing when the user cancels the confirm', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useAffiliatesStore()
    s.affiliates = [affiliate({ id: 1 })]
    await s.deleteAffiliate(1)
    expect(ep.del).not.toHaveBeenCalled()
    expect(s.affiliates).toHaveLength(1)
  })

  it('deletes and drops the affiliate from the list when confirmed', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useAffiliatesStore()
    s.affiliates = [affiliate({ id: 1 }), affiliate({ id: 2 })]
    await s.deleteAffiliate(1)
    expect(ep.del).toHaveBeenCalledWith(1)
    expect(s.affiliates.map((a) => a.id)).toEqual([2])
  })
})
