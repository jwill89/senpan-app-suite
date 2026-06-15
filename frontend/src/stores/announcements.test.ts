import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Announcement } from '@/types/api'

// Capture the payload save() builds; list() backs the post-save refresh.
const { save, list } = vi.hoisted(() => ({
  save: vi.fn(async () => ({ announcement: {} })),
  list: vi.fn(async () => ({ announcements: [] })),
}))
vi.mock('@/lib/endpoints', () => ({ endpoints: { announcements: { save, list } } }))

import { useAnnouncementsStore } from './announcements'

/** Shape of the fields the payload-building tests assert on. */
interface SavedPayload {
  title: string
  schedule_minutes: number
  schedule_weekdays: string
  buttons: { label: string; emoji: string; url: string }[]
}
/** Args of the first save() call (vi infers no params, so widen to read them). */
function firstSaveArgs(): unknown[] {
  return save.mock.calls[0] as unknown[]
}
/** The payload passed to the first save() call, typed for assertions. */
function savedPayload(): SavedPayload {
  return firstSaveArgs()[1] as SavedPayload
}

beforeEach(() => {
  setActivePinia(createPinia())
  save.mockClear()
  list.mockClear()
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
    expect(payload.buttons.map((b) => b.label)).toEqual([
      'Go',
      'One',
      'Two',
      'Three',
      'Four',
    ])
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
    } as unknown as Announcement)
    expect(s.form.id).toBe(7)
    expect(s.form.time_local).toBe('19:30')
    expect(s.form.weekdays).toEqual([1, 3, 5]) // parsed + sorted
    expect(s.form.color).toBe('#e53170') // blank colour falls back to brand default
    expect(s.form.buttons).toEqual([{ label: 'Go', emoji: '', url: 'https://a.com' }])
  })
})
