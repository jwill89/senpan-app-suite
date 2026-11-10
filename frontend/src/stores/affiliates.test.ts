import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Affiliate } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useAffiliatesStore } from './affiliates'
import { useUiStore } from './ui'

/** Minimal affiliate with the fields the store reads. */
function affiliate(over: Partial<Affiliate> = {}): Affiliate {
  return {
    id: 1,
    name: 'A',
    subtitle: '',
    owners: [],
    location: '',
    timezone: 'UTC',
    hours: [],
    details: '',
    logo: '',
    screenshot: '',
    embedColor: '',
    discordLink: '',
    carrdLink: '',
    sortOrder: 0,
    createdAt: '2026-11-09T10:00:00Z',
    ...over,
  }
}

const hook = (webhookUrl = '') => ok({ webhookUrl })

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) =>
    path === '/affiliates' ? ok({ affiliates: [] }) : hook(),
  )
  api.POST.mockResolvedValue(ok({}, 201))
  api.PUT.mockResolvedValue(ok({}))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

/** The body of the call made to a method and path. */
function sent(method: 'POST' | 'PUT', path: string): { body: Record<string, unknown> } {
  const call = api[method].mock.calls.find((c) => c[0] === path)
  return call![1]
}

describe('loading', () => {
  it('loadAffiliates populates the list and the shared webhook', async () => {
    api.GET.mockImplementation(async (path: string) =>
      path === '/affiliates'
        ? ok({ affiliates: [affiliate(), affiliate({ id: 2 })] })
        : hook('https://discord.com/api/webhooks/1/x'),
    )
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
    expect(api.POST).not.toHaveBeenCalled()
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
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates with blank owners/hours stripped', async () => {
    const s = useAffiliatesStore()
    s.newAffiliateForm()
    s.affiliateForm!.name = 'Tavern'
    s.affiliateForm!.owners = [{ value: 'Tataru' }, { value: '  ' }, { value: 'Hildibrand' }]
    s.affiliateForm!.hours = [
      { label: 'Mon-Fri', start: '18:00', end: '23:00' },
      { label: 'blank', start: '  ', end: '' }, // dropped (no start)
    ]
    const ok = await s.saveAffiliate()
    expect(ok).toBe(true)
    expect(api.POST).toHaveBeenCalledTimes(1)
    const payload = sent('POST', '/affiliates').body as { owners: string[]; hours: unknown[] }
    expect(payload.owners).toEqual(['Tataru', 'Hildibrand'])
    expect(payload.hours).toHaveLength(1)
    expect(s.affiliateForm).toBeNull()
  })

  it('updates when the form carries an id', async () => {
    const s = useAffiliatesStore()
    s.editAffiliateForm(affiliate({ id: 4, name: 'Tavern', owners: ['Solo'] }))
    const ok = await s.saveAffiliate()
    expect(ok).toBe(true)
    expect(api.PUT).toHaveBeenCalledWith(
      '/affiliates/{id}',
      expect.objectContaining({ params: { path: { id: 4 } } }),
    )
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('deleteAffiliate', () => {
  it('does nothing when the user cancels the confirm', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useAffiliatesStore()
    s.affiliates = [affiliate({ id: 1 })]
    await s.deleteAffiliate(1)
    expect(api.DELETE).not.toHaveBeenCalled()
    expect(s.affiliates).toHaveLength(1)
  })

  it('deletes and drops the affiliate from the list when confirmed', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useAffiliatesStore()
    s.affiliates = [affiliate({ id: 1 }), affiliate({ id: 2 })]
    await s.deleteAffiliate(1)
    expect(api.DELETE).toHaveBeenCalledWith('/affiliates/{id}', { params: { path: { id: 1 } } })
    expect(s.affiliates.map((a) => a.id)).toEqual([2])
  })
})

describe('reorder / webhook / post', () => {
  it('reorder persists the new id order', async () => {
    const s = useAffiliatesStore()
    await s.reorder([3, 1, 2])
    expect(api.POST).toHaveBeenCalledWith('/affiliates/reorder', {
      body: { orderedIds: [3, 1, 2] },
    })
  })

  it('reorder reverts (reloads) on failure', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    api.POST.mockRejectedValueOnce(new Error('nope'))
    const s = useAffiliatesStore()
    await s.reorder([1])
    expect(api.GET).toHaveBeenCalledWith('/affiliates') // reload to revert
    expect(ui.notify).toHaveBeenCalled()
  })

  it('saveWebhook stores the trimmed url', async () => {
    api.PUT.mockResolvedValue(hook('https://discord.com/api/webhooks/1/x'))
    const s = useAffiliatesStore()
    const ok = await s.saveWebhook('  https://discord.com/api/webhooks/1/x  ')
    expect(ok).toBe(true)
    expect(api.PUT).toHaveBeenCalledWith('/affiliates/webhook', {
      body: { webhookUrl: 'https://discord.com/api/webhooks/1/x' },
    })
    expect(s.webhookUrl).toBe('https://discord.com/api/webhooks/1/x')
  })

  it('postAffiliate refuses without a webhook', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useAffiliatesStore()
    s.webhookUrl = ''
    await s.postAffiliate(affiliate({ id: 5 }))
    expect(api.POST).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('postAffiliate posts after confirmation when a webhook is set', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useAffiliatesStore()
    s.webhookUrl = 'https://discord.com/api/webhooks/1/x'
    await s.postAffiliate(affiliate({ id: 5 }))
    expect(api.POST).toHaveBeenCalledWith('/affiliates/{id}/post', {
      params: { path: { id: 5 } },
    })
  })
})
