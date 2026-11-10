import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Announcement, AnnouncementInput } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const failed = (detail: string, status = 500) => ({
  error: { detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PUT: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useAnnouncementsStore } from '@/stores/announcements'
import { useUiStore } from '@/stores/ui'

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) => {
    if (path === '/announcements') {
      return ok({ announcements: [] })
    }
    return ok({ types: [], roles: [] })
  })
  api.POST.mockResolvedValue(ok({}, 201))
  api.PUT.mockResolvedValue(ok({}))
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

/** The call made to a method and path. */
function call(method: 'POST' | 'PUT' | 'DELETE', path: string) {
  return api[method].mock.calls.find((c) => c[0] === path)?.[1]
}

/** The announcement body save() sent (a create, unless told otherwise). */
function savedBody(method: 'POST' | 'PUT' = 'POST', path = '/announcements'): AnnouncementInput {
  return call(method, path).body
}

/** Whether the announcements were read again. */
const reloaded = () => api.GET.mock.calls.some((c) => c[0] === '/announcements')

describe('reorder', () => {
  it('persists the supplied id order via the reorder operation', async () => {
    const store = useAnnouncementsStore()
    await store.reorder([3, 1, 2])
    expect(call('POST', '/announcements/reorder')).toEqual({ body: { orderedIds: [3, 1, 2] } })
  })

  it('reverts to the server order when the save fails', async () => {
    api.POST.mockResolvedValueOnce(failed('boom', 422))
    const store = useAnnouncementsStore()
    await store.reorder([2, 1])
    expect(reloaded()).toBe(true) // reloads the persisted order
  })
})

describe('filteredAnnouncements', () => {
  beforeEach(() => {
    const s = useAnnouncementsStore()
    s.announcements = [
      { id: 1, typeId: 1, typeName: 'News', title: 'Tea Party', details: 'come' },
      { id: 2, typeId: 2, typeName: 'Events', title: 'Bingo Night', details: 'play' },
    ] as Announcement[]
  })

  it('matches title/type/details case-insensitively', () => {
    const s = useAnnouncementsStore()
    s.search = 'bingo'
    expect(s.filteredAnnouncements.map((a) => a.id)).toEqual([2])
  })

  it('filters by type (category) id', () => {
    const s = useAnnouncementsStore()
    s.typeFilter = 1
    expect(s.filteredAnnouncements.map((a) => a.id)).toEqual([1])
  })

  it('combines type filter and search', () => {
    const s = useAnnouncementsStore()
    s.typeFilter = 2
    s.search = 'tea'
    expect(s.filteredAnnouncements).toHaveLength(0)
  })
})

describe('loading', () => {
  it('load reads the types, the roles and the announcements', async () => {
    api.GET.mockImplementation(async (path: string) => {
      switch (path) {
        case '/announcement-types':
          return ok({ types: [{ id: 1, name: 'News' }] })
        case '/announcement-roles':
          return ok({ roles: [{ id: 2, name: 'Crew' }] })
        default:
          return ok({ announcements: [{ id: 3 }] })
      }
    })
    const s = useAnnouncementsStore()
    await s.load()
    expect(s.types.map((t) => t.id)).toEqual([1])
    expect(s.roles.map((r) => r.id)).toEqual([2])
    expect(s.announcements.map((a) => a.id)).toEqual([3])
    expect(s.loading).toBe(false)
  })

  it('shows the problem when a read fails', async () => {
    api.GET.mockResolvedValue(failed('The server is down'))
    const s = useAnnouncementsStore()
    await s.load()
    expect(useUiStore().toast.message).toBe('The server is down')
  })
})

describe('types and roles', () => {
  it('saveType creates a trimmed type, or replaces one that has an id', async () => {
    const s = useAnnouncementsStore()
    s.typeForm = { id: 0, name: '  News  ', webhookUrl: ' https://discord.com/api/webhooks/1/x ' }
    expect(await s.saveType()).toBe(true)
    expect(call('POST', '/announcement-types')).toEqual({
      body: { name: 'News', webhookUrl: 'https://discord.com/api/webhooks/1/x' },
    })

    s.typeForm = { id: 5, name: 'News', webhookUrl: '' }
    await s.saveType()
    expect(call('PUT', '/announcement-types/{id}')).toEqual({
      params: { path: { id: 5 } },
      body: { name: 'News', webhookUrl: '' },
    })
  })

  it('saveType needs a name', async () => {
    const s = useAnnouncementsStore()
    s.typeForm = { id: 0, name: '  ', webhookUrl: '' }
    expect(await s.saveType()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('saveRole sends the role id, which is digits only', async () => {
    const s = useAnnouncementsStore()
    s.roleForm = { id: 0, name: 'Crew', roleId: '12a' }
    expect(await s.saveRole()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()

    s.roleForm = { id: 0, name: ' Crew ', roleId: ' 123 ' }
    expect(await s.saveRole()).toBe(true)
    expect(call('POST', '/announcement-roles')).toEqual({ body: { name: 'Crew', roleId: '123' } })
  })

  it("a type in use is refused with the server's reason", async () => {
    api.DELETE.mockResolvedValueOnce(failed('This type is used by 2 announcement(s).', 409))
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useAnnouncementsStore()
    await s.deleteType({ id: 4, name: 'News', webhookUrl: '', createdAt: null })
    expect(ui.toast.message).toBe('This type is used by 2 announcement(s).')
  })
})

describe('save() payload building', () => {
  it('encodes a weekly schedule (minutes + deduped/sorted weekdays)', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      typeId: 3,
      title: '  Weekly Tea  ',
      details: 'every week',
      scheduleKind: 'weekly',
      timeLocal: '19:30',
      weekdays: [5, 1, 3, 1], // out of order + duplicate
    })
    const ok = await s.save()
    expect(ok).toBe(true)
    const payload = savedBody()
    expect(payload.title).toBe('Weekly Tea') // trimmed
    expect(payload.typeId).toBe(3)
    expect(payload.scheduleMinutes).toBe(19 * 60 + 30)
    expect(payload.scheduleWeekdays).toBe('1,3,5')
    expect(payload).not.toHaveProperty('id')
    expect(payload).not.toHaveProperty('timeLocal')
  })

  it('replaces an existing announcement through its own path', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { id: 9, typeId: 1, title: 'Edited', details: 'x' })
    await s.save()
    expect(call('PUT', '/announcements/{id}').params).toEqual({ path: { id: 9 } })
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('sanitizes buttons: trims, drops blank-label / non-http, caps at 5', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      typeId: 1,
      title: 'Has buttons',
      details: 'x',
      buttons: [
        { label: '  Go  ', emoji: ' 😀 ', url: '  https://a.com  ' }, // kept (trimmed)
        { label: '', emoji: '', url: 'https://b.com' }, // dropped: blank label
        { label: 'Bad', emoji: '', url: 'ftp://c.com' }, // dropped: non-http
        { label: 'One', emoji: '', url: 'https://1.com' },
        { label: 'Two', emoji: '', url: 'https://2.com' },
        { label: 'Three', emoji: '', url: 'https://3.com' },
        { label: 'Four', emoji: '', url: 'https://4.com' },
        { label: 'Five', emoji: '', url: 'https://5.com' }, // 6th valid -> sliced off
      ],
    })
    await s.save()
    const payload = savedBody()
    expect(payload.buttons).toHaveLength(5)
    expect(payload.buttons[0]).toEqual({ label: 'Go', emoji: '😀', url: 'https://a.com' })
    expect(payload.buttons.map((b) => b.label)).toEqual(['Go', 'One', 'Two', 'Three', 'Four'])
  })

  it('passes the selected role tag through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { typeId: 1, title: 'Tagged', details: 'x', mention: 'role:4' })
    await s.save()
    expect(savedBody().mention).toBe('role:4')
  })

  it('passes the selected Discord start/end formats through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      typeId: 1,
      title: 'Timed',
      details: 'x',
      startFormat: 'R',
      endFormat: 'T',
    })
    await s.save()
    expect(savedBody().startFormat).toBe('R')
    expect(savedBody().endFormat).toBe('T')
  })

  it('passes the dynamic-dates flag through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { typeId: 1, title: 'Recurring event', details: 'x', dynamicDates: true })
    await s.save()
    expect(savedBody().dynamicDates).toBe(true)
  })

  it('passes both the image and the thumbnail through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      typeId: 1,
      title: 'Pics',
      details: 'x',
      image: 'https://h/images/announcements/main.png',
      thumbnail: 'https://h/images/announcements/thumb.png',
    })
    await s.save()
    expect(savedBody().image).toBe('https://h/images/announcements/main.png')
    expect(savedBody().thumbnail).toBe('https://h/images/announcements/thumb.png')
  })

  it('blocks save and does not hit the API when the title is missing', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { typeId: 1, title: '   ', details: 'x' })
    const ok = await s.save()
    expect(ok).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('requires a weekday for a weekly schedule', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      typeId: 1,
      title: 'No days',
      details: 'x',
      scheduleKind: 'weekly',
      timeLocal: '12:00',
      weekdays: [],
    })
    expect(await s.save()).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it("keeps the form and shows the server's reason when a save is refused", async () => {
    api.POST.mockResolvedValueOnce(failed("The announcement can't be saved as it is.", 422))
    const s = useAnnouncementsStore()
    Object.assign(s.form, { typeId: 1, title: 'Refused', details: 'x' })
    expect(await s.save()).toBe(false)
    expect(s.form.title).toBe('Refused')
    expect(s.saving).toBe(false)
  })
})

describe('editAnnouncement round-trip', () => {
  it('expands stored schedule fields back into the form', () => {
    const s = useAnnouncementsStore()
    s.editAnnouncement({
      id: 7,
      typeId: 2,
      title: 'Recurring',
      details: 'd',
      image: '',
      color: '',
      scheduleKind: 'weekly',
      scheduleMinutes: 19 * 60 + 30,
      scheduleWeekdays: '5,1,3',
      scheduleWeekOfMonth: 2,
      buttons: [{ label: 'Go', emoji: '', url: 'https://a.com' }],
      mention: 'everyone',
      dynamicDates: false,
      startFormat: '',
      endFormat: '',
    } as unknown as Announcement)
    expect(s.form.id).toBe(7)
    expect(s.form.timeLocal).toBe('19:30')
    expect(s.form.weekdays).toEqual([1, 3, 5]) // parsed + sorted
    expect(s.form.color).toBe('#ff3131') // blank colour falls back to brand default
    expect(s.form.thumbnail).toBe('') // absent thumbnail -> empty
    expect(s.form.dynamicDates).toBe(false)
    expect(s.form.startFormat).toBe('F') // blank start format -> default
    expect(s.form.endFormat).toBe('t') // blank end format -> default
    // Buttons round-trip; each also gains a client-only `_uid` for stable
    // repeater keying (stripped again when the payload is built).
    expect(s.form.buttons).toMatchObject([{ label: 'Go', emoji: '', url: 'https://a.com' }])
    expect(s.form.buttons[0]!._uid).toEqual(expect.any(Number))
    expect(s.form.mention).toBe('everyone')
  })

  it('handles a button-less announcement (empty array) without throwing', () => {
    const s = useAnnouncementsStore()
    expect(() =>
      s.editAnnouncement({
        id: 8,
        typeId: 1,
        title: 'No buttons',
        details: 'd',
        image: '',
        color: '',
        buttons: [],
        mention: '',
        dynamicDates: false,
      } as unknown as Announcement),
    ).not.toThrow()
    expect(s.form.buttons).toEqual([])
  })
})

describe('sendNow', () => {
  it('posts after confirming, then reads the list again', async () => {
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useAnnouncementsStore()
    await s.sendNow({ id: 4, title: 'Bingo Night' } as Announcement)
    expect(call('POST', '/announcements/{id}/send')).toEqual({ params: { path: { id: 4 } } })
    expect(reloaded()).toBe(true)
    expect(s.sendingId).toBeNull()
  })

  it('does nothing when the confirmation is declined', async () => {
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(false)
    const s = useAnnouncementsStore()
    await s.sendNow({ id: 4, title: 'Bingo Night' } as Announcement)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('shows why Discord refused', async () => {
    api.POST.mockResolvedValueOnce(failed('Discord is limiting posts right now.', 429))
    const ui = useUiStore()
    vi.spyOn(ui, 'confirm').mockResolvedValue(true)
    const s = useAnnouncementsStore()
    await s.sendNow({ id: 4, title: 'Bingo Night' } as Announcement)
    expect(ui.toast.message).toBe('Discord is limiting posts right now.')
    expect(s.sendingId).toBeNull()
  })
})

describe('setSkip', () => {
  it('sends the requested count and refreshes the list', async () => {
    const s = useAnnouncementsStore()
    const a = { id: 4, title: 'Bingo Night' } as Announcement
    expect(await s.setSkip(a, 2)).toBe(true)
    expect(call('POST', '/announcements/{id}/skip')).toEqual({
      params: { path: { id: 4 } },
      body: { count: 2 },
    })
    // The list is reloaded so the row's badge reflects the new count.
    expect(reloaded()).toBe(true)
    expect(s.skippingId).toBeNull()
  })

  it('sends 0 to clear a pending skip', async () => {
    const s = useAnnouncementsStore()
    expect(await s.setSkip({ id: 4, title: 'X' } as Announcement, 0)).toBe(true)
    expect(call('POST', '/announcements/{id}/skip').body).toEqual({ count: 0 })
  })

  it('reports failure and clears the in-flight marker', async () => {
    api.POST.mockResolvedValueOnce(failed("This announcement isn't scheduled.", 409))
    const s = useAnnouncementsStore()
    expect(await s.setSkip({ id: 4, title: 'X' } as Announcement, 1)).toBe(false)
    expect(s.skippingId).toBeNull()
  })
})
