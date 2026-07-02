import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Announcement } from '@/types/api'

// Capture the payload save() builds; list() backs the post-save refresh.
const { save, list, reorder } = vi.hoisted(() => ({
  save: vi.fn(async () => ({ announcement: {} })),
  list: vi.fn(async () => ({ announcements: [] })),
  reorder: vi.fn(async () => ({ ok: true })),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { announcements: { save, list, reorder } } }))

import { useAnnouncementsStore } from './announcements'

/** Shape of the fields the payload-building tests assert on. */
interface SavedPayload {
  title: string
  schedule_minutes: number
  schedule_weekdays: string
  buttons: { label: string; emoji: string; url: string }[]
  mention: string
  start_format: string
  end_format: string
  image: string
  thumbnail: string
  dynamic_dates: boolean
}
/** Args of the first save() call (vi infers no params, so widen to read them). */
function firstSaveArgs(): unknown[] {
  return save.mock.calls[0]
}
/** The payload passed to the first save() call, typed for assertions. */
function savedPayload(): SavedPayload {
  return firstSaveArgs()[1] as SavedPayload
}

beforeEach(() => {
  setActivePinia(createPinia())
  save.mockClear()
  list.mockClear()
  reorder.mockClear()
})

describe('reorder', () => {
  it('persists the supplied id order via the reorder endpoint', async () => {
    const store = useAnnouncementsStore()
    await store.reorder([3, 1, 2])
    expect(reorder).toHaveBeenCalledWith([3, 1, 2])
  })

  it('reverts to the server order when the save fails', async () => {
    reorder.mockRejectedValueOnce(new Error('boom'))
    const store = useAnnouncementsStore()
    await store.reorder([2, 1])
    expect(list).toHaveBeenCalled() // reloads the persisted order
  })
})

describe('filteredAnnouncements', () => {
  beforeEach(() => {
    const s = useAnnouncementsStore()
    s.announcements = [
      { id: 1, type_id: 1, type_name: 'News', title: 'Tea Party', details: 'come' },
      { id: 2, type_id: 2, type_name: 'Events', title: 'Bingo Night', details: 'play' },
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

describe('save() payload building', () => {
  it('encodes a weekly schedule (minutes + deduped/sorted weekdays)', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 3,
      title: '  Weekly Tea  ',
      details: 'every week',
      schedule_kind: 'weekly',
      time_local: '19:30',
      weekdays: [5, 1, 3, 1], // out of order + duplicate
    })
    const ok = await s.save()
    expect(ok).toBe(true)
    expect(firstSaveArgs()[0]).toBe(0) // id (0 = create)
    const payload = savedPayload()
    expect(payload.title).toBe('Weekly Tea') // trimmed
    expect(payload.schedule_minutes).toBe(19 * 60 + 30)
    expect(payload.schedule_weekdays).toBe('1,3,5')
  })

  it('sanitizes buttons: trims, drops blank-label / non-http, caps at 5', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 1,
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
        { label: 'Five', emoji: '', url: 'https://5.com' }, // 6th valid → sliced off
      ],
    })
    await s.save()
    const payload = savedPayload()
    expect(payload.buttons).toHaveLength(5)
    expect(payload.buttons[0]).toEqual({ label: 'Go', emoji: '😀', url: 'https://a.com' })
    expect(payload.buttons.map((b) => b.label)).toEqual(['Go', 'One', 'Two', 'Three', 'Four'])
  })

  it('passes the selected role tag through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { type_id: 1, title: 'Tagged', details: 'x', mention: 'role:4' })
    await s.save()
    expect(savedPayload().mention).toBe('role:4')
  })

  it('passes the selected Discord start/end formats through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 1,
      title: 'Timed',
      details: 'x',
      start_format: 'R',
      end_format: 'T',
    })
    await s.save()
    expect(savedPayload().start_format).toBe('R')
    expect(savedPayload().end_format).toBe('T')
  })

  it('passes the dynamic-dates flag through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 1,
      title: 'Recurring event',
      details: 'x',
      dynamic_dates: true,
    })
    await s.save()
    expect(savedPayload().dynamic_dates).toBe(true)
  })

  it('passes both the image and the thumbnail through to the payload', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 1,
      title: 'Pics',
      details: 'x',
      image: 'https://h/images/announcements/main.png',
      thumbnail: 'https://h/images/announcements/thumb.png',
    })
    await s.save()
    expect(savedPayload().image).toBe('https://h/images/announcements/main.png')
    expect(savedPayload().thumbnail).toBe('https://h/images/announcements/thumb.png')
  })

  it('blocks save and does not hit the API when the title is missing', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, { type_id: 1, title: '   ', details: 'x' })
    const ok = await s.save()
    expect(ok).toBe(false)
    expect(save).not.toHaveBeenCalled()
  })

  it('requires a weekday for a weekly schedule', async () => {
    const s = useAnnouncementsStore()
    Object.assign(s.form, {
      type_id: 1,
      title: 'No days',
      details: 'x',
      schedule_kind: 'weekly',
      time_local: '12:00',
      weekdays: [],
    })
    expect(await s.save()).toBe(false)
    expect(save).not.toHaveBeenCalled()
  })
})

describe('editAnnouncement round-trip', () => {
  it('expands stored schedule fields back into the form', () => {
    const s = useAnnouncementsStore()
    s.editAnnouncement({
      id: 7,
      type_id: 2,
      title: 'Recurring',
      details: 'd',
      image: '',
      color: '',
      schedule_kind: 'weekly',
      schedule_minutes: 19 * 60 + 30,
      schedule_weekdays: '5,1,3',
      schedule_week_of_month: 2,
      buttons: [{ label: 'Go', emoji: '', url: 'https://a.com' }],
      mention: 'everyone',
      dynamic_dates: false,
    } as unknown as Announcement)
    expect(s.form.id).toBe(7)
    expect(s.form.time_local).toBe('19:30')
    expect(s.form.weekdays).toEqual([1, 3, 5]) // parsed + sorted
    expect(s.form.color).toBe('#ff3131') // blank colour falls back to brand default
    expect(s.form.thumbnail).toBe('') // absent thumbnail → empty
    expect(s.form.dynamic_dates).toBe(false) // flag round-trips (API always sends it — no omitempty)
    expect(s.form.start_format).toBe('F') // blank start format → default
    expect(s.form.end_format).toBe('t') // blank end format → default
    // Buttons round-trip; each also gains a client-only `_uid` for stable
    // repeater keying (stripped again when the payload is built).
    expect(s.form.buttons).toMatchObject([{ label: 'Go', emoji: '', url: 'https://a.com' }])
    expect(s.form.buttons[0]._uid).toEqual(expect.any(Number))
    expect(s.form.mention).toBe('everyone')
  })

  it('handles a button-less announcement (empty array) without throwing', () => {
    const s = useAnnouncementsStore()
    expect(() =>
      s.editAnnouncement({
        id: 8,
        type_id: 1,
        title: 'No buttons',
        details: 'd',
        image: '',
        color: '',
        buttons: [],
        mention: '',
        dynamic_dates: false,
      } as unknown as Announcement),
    ).not.toThrow()
    expect(s.form.buttons).toEqual([])
  })
})
