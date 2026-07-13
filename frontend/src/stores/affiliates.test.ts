import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Affiliate } from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ affiliates: [] as Affiliate[], webhook_url: '' })),
  create: vi.fn(async () => ({ ok: true })),
  update: vi.fn(async () => ({ ok: true })),
  del: vi.fn(async () => ({ ok: true })),
  reorder: vi.fn(async () => ({ ok: true })),
  post: vi.fn(async () => ({ ok: true })),
  setWebhook: vi.fn(async (url: string) => ({ webhook_url: url })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    affiliates: {
      list: ep.list,
      create: ep.create,
      update: ep.update,
      delete: ep.del,
      reorder: ep.reorder,
      post: ep.post,
      setWebhook: ep.setWebhook,
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
    embed_color: '',
    discord_link: '',
    carrd_link: '',
    sort_order: 0,
    created_at: '',
    ...over,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(ep).forEach((fn) => fn.mockClear())
})

describe('loading', () => {
  it('loadAffiliates populates the list and the shared webhook', async () => {
    ep.list.mockResolvedValueOnce({
      affiliates: [affiliate(), affiliate({ id: 2 })],
      webhook_url: 'https://discord.com/api/webhooks/1/x',
    })
    const s = useAffiliatesStore()
    await s.loadAffiliates()
    expect(s.affiliates).toHaveLength(2)
    expect(s.webhookUrl).toBe('https://discord.com/api/webhooks/1/x')
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
      affiliate({
        id: 7,
        name: 'Tavern',
        owners: ['Tataru'],
        hours: [{ label: '', start: '09:00', end: '' }],
      }),
    )
    // Owners are wrapped in form rows ({ value, _uid }) so the repeater can key
    // on a stable id; the plain names are unwrapped again in the save payload.
    expect(s.affiliateForm).toMatchObject({ id: 7, name: 'Tavern', owners: [{ value: 'Tataru' }] })
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
    s.affiliateForm!.owners = [{ value: '  ' }] // blanks only
    const ok = await s.saveAffiliate()
    expect(ok).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates with blank owners/hours stripped', async () => {
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.affiliateForm!.name = 'Tavern'
    s.affiliateForm!.owners = [{ value: 'Tataru' }, { value: '  ' }, { value: 'Hildibrand' }]
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
    // PUT /api/affiliates/{id}: the id is passed separately from the payload.
    expect((ep.update.mock.calls[0] as unknown[])[0]).toBe(4)
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

describe('reorder / webhook / post', () => {
  it('reorder persists the new id order', async () => {
    const s = useAffiliatesStore()
    await s.reorder([3, 1, 2])
    expect(ep.reorder).toHaveBeenCalledWith([3, 1, 2])
  })

  it('reorder reverts (reloads) on failure', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    ep.reorder.mockRejectedValueOnce(new Error('nope'))
    const s = useAffiliatesStore()
    await s.reorder([1])
    expect(ep.list).toHaveBeenCalled() // reload to revert
    expect(ui.notify).toHaveBeenCalled()
  })

  it('saveWebhook stores the trimmed url', async () => {
    const s = useAffiliatesStore()
    const ok = await s.saveWebhook('  https://discord.com/api/webhooks/1/x  ')
    expect(ok).toBe(true)
    expect(ep.setWebhook).toHaveBeenCalledWith('https://discord.com/api/webhooks/1/x')
    expect(s.webhookUrl).toBe('https://discord.com/api/webhooks/1/x')
  })

  it('postAffiliate refuses without a webhook', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useAffiliatesStore()
    s.webhookUrl = ''
    await s.postAffiliate(affiliate({ id: 5 }))
    expect(ep.post).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('postAffiliate posts after confirmation when a webhook is set', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useAffiliatesStore()
    s.webhookUrl = 'https://discord.com/api/webhooks/1/x'
    await s.postAffiliate(affiliate({ id: 5 }))
    expect(ep.post).toHaveBeenCalledWith(5)
  })
})
