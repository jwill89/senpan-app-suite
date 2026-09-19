import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { TeaRoom } from '@/types/api'

// Mock the typed endpoint layer so store actions run without the network.
const ep = vi.hoisted(() => ({
  list: vi.fn(async () => ({ tea_rooms: [] as TeaRoom[], webhook_url: '' })),
  create: vi.fn(async () => ({ tea_room: null })),
  update: vi.fn(async () => ({ tea_room: null })),
  patch: vi.fn(
    async (
      _id: number,
      fields: { open?: boolean; discounted?: boolean; locked?: boolean; locked_until?: string },
    ) => ({
      tea_room: room({ id: _id, ...fields }),
    }),
  ),
  del: vi.fn(async () => undefined),
  reorder: vi.fn(async () => ({ ok: true })),
  post: vi.fn(async () => ({ tea_room: null })),
  setWebhook: vi.fn(async (url: string) => ({ webhook_url: url })),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: {
    teaRooms: {
      list: ep.list,
      create: ep.create,
      update: ep.update,
      patch: ep.patch,
      delete: ep.del,
      reorder: ep.reorder,
      post: ep.post,
      setWebhook: ep.setWebhook,
    },
  },
}))

import { useTeaRoomsStore } from './teaRooms'
import { useUiStore } from './ui'

/** Minimal tea room with the fields the store reads. */
function room(over: Partial<TeaRoom> = {}): TeaRoom {
  return {
    id: 1,
    name: 'R',
    subtitle: '',
    room_owner: '',
    room_number: '1',
    cost_per_half_hour: 0,
    hashtags: '',
    description: '',
    seasonal: false,
    open: true,
    lockable: false,
    discounted: false,
    locked: false,
    locked_until: '',
    image: '',
    color: '#ff3131',
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
  it('loadTeaRooms populates the list and webhook', async () => {
    ep.list.mockResolvedValueOnce({
      tea_rooms: [room(), room({ id: 2 })],
      webhook_url: 'https://discord.com/api/webhooks/x',
    })
    const s = useTeaRoomsStore()
    await s.loadTeaRooms()
    expect(s.teaRooms).toHaveLength(2)
    expect(s.webhookUrl).toContain('discord.com')
    expect(s.loading).toBe(false)
  })
})

describe('form', () => {
  it('newTeaRoomForm defaults to open, no discount, brand accent', () => {
    const s = useTeaRoomsStore()
    s.newTeaRoomForm()
    expect(s.teaRoomForm).toMatchObject({
      id: 0,
      open: true,
      discounted: false,
      color: '#ff3131',
    })
  })

  it('editTeaRoom seeds the form from a room', () => {
    const s = useTeaRoomsStore()
    s.editTeaRoom(room({ id: 7, name: 'Jasmine', cost_per_half_hour: 125000, seasonal: true }))
    expect(s.teaRoomForm).toMatchObject({
      id: 7,
      name: 'Jasmine',
      cost_per_half_hour: 125000,
      seasonal: true,
    })
  })
})

describe('filteredTeaRooms', () => {
  it('filters by name / room number / hashtags', () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [
      room({ id: 1, name: 'Jasmine', hashtags: '#cozy' }),
      room({ id: 2, name: 'Rose', room_number: 'West 3' }),
    ]
    s.search = 'cozy'
    expect(s.filteredTeaRooms.map((r) => r.id)).toEqual([1])
    s.search = 'west'
    expect(s.filteredTeaRooms.map((r) => r.id)).toEqual([2])
    s.search = ''
    expect(s.filteredTeaRooms).toHaveLength(2)
  })
})

describe('saveTeaRoom', () => {
  it('requires a name', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.newTeaRoomForm() // name blank
    const ok = await s.saveTeaRoom()
    expect(ok).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('requires a room number', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.newTeaRoomForm()
    s.teaRoomForm!.name = 'Jasmine' // room number still blank
    const ok = await s.saveTeaRoom()
    expect(ok).toBe(false)
    expect(ep.create).not.toHaveBeenCalled()
  })

  it('creates with the id stripped from the payload', async () => {
    const s = useTeaRoomsStore()
    s.newTeaRoomForm()
    s.teaRoomForm!.name = 'Jasmine'
    s.teaRoomForm!.room_number = '12'
    s.teaRoomForm!.cost_per_half_hour = 125000
    const ok = await s.saveTeaRoom()
    expect(ok).toBe(true)
    expect(ep.create).toHaveBeenCalledTimes(1)
    const payload = (ep.create.mock.calls[0] as unknown[])[0] as Record<string, unknown>
    expect(payload).not.toHaveProperty('id')
    expect(payload.name).toBe('Jasmine')
    expect(s.teaRoomForm).toBeNull()
  })

  it('updates when the form carries an id (id passed separately)', async () => {
    const s = useTeaRoomsStore()
    s.editTeaRoom(room({ id: 4, name: 'Rose' }))
    const ok = await s.saveTeaRoom()
    expect(ok).toBe(true)
    expect(ep.update).toHaveBeenCalledTimes(1)
    expect((ep.update.mock.calls[0] as unknown[])[0]).toBe(4)
    expect(ep.create).not.toHaveBeenCalled()
  })
})

describe('toggles', () => {
  it('toggleOpen patches the opposite flag and replaces the row', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1, open: true })]
    await s.toggleOpen(s.teaRooms[0])
    expect(ep.patch).toHaveBeenCalledWith(1, { open: false })
    expect(s.teaRooms[0].open).toBe(false)
  })

  it('toggleDiscounted patches the opposite flag', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1, discounted: false })]
    await s.toggleDiscounted(s.teaRooms[0])
    expect(ep.patch).toHaveBeenCalledWith(1, { discounted: true })
    expect(s.teaRooms[0].discounted).toBe(true)
  })
})

describe('locking', () => {
  it('locks with no expiry when no unlock time is given', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    expect(await s.lockRoom(s.teaRooms[0], '')).toBe(true)
    expect(ep.patch).toHaveBeenCalledWith(1, { locked: true, locked_until: '' })
  })

  it('sends the unlock time as a UTC instant', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    // A local wall-clock value from the datetime-local input, well in the future.
    const future = new Date(Date.now() + 60 * 60 * 1000)
    const pad = (n: number): string => String(n).padStart(2, '0')
    const local =
      `${future.getFullYear()}-${pad(future.getMonth() + 1)}-${pad(future.getDate())}` +
      `T${pad(future.getHours())}:${pad(future.getMinutes())}`

    expect(await s.lockRoom(s.teaRooms[0], local)).toBe(true)
    const [, fields] = ep.patch.mock.calls[0]
    expect(fields.locked).toBe(true)
    // The same instant, expressed in UTC - not the local string passed straight
    // through, which would land the unlock at the wrong moment for anyone else.
    expect(fields.locked_until).toMatch(/Z$/)
    expect(new Date(fields.locked_until as string).getTime()).toBe(new Date(local).getTime())
  })

  it('refuses an unlock time that has already passed', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    expect(await s.lockRoom(s.teaRooms[0], '2020-01-01T00:00')).toBe(false)
    expect(ep.patch).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('unlockRoom clears the lock', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 4, locked: true, locked_until: '2030-01-01T00:00:00Z' })]
    await s.unlockRoom(s.teaRooms[0])
    expect(ep.patch).toHaveBeenCalledWith(4, { locked: false })
  })
})

describe('postRoom', () => {
  it('refuses to post without a webhook', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.webhookUrl = ''
    await s.postRoom(room({ id: 1 }))
    expect(ep.post).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('posts after confirmation when a webhook is set', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useTeaRoomsStore()
    s.webhookUrl = 'https://discord.com/api/webhooks/x'
    await s.postRoom(room({ id: 3 }))
    expect(ep.post).toHaveBeenCalledWith(3)
  })
})

describe('deleteTeaRoom', () => {
  it('does nothing when the user cancels', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    await s.deleteTeaRoom(s.teaRooms[0])
    expect(ep.del).not.toHaveBeenCalled()
    expect(s.teaRooms).toHaveLength(1)
  })

  it('deletes and drops the room when confirmed', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 }), room({ id: 2 })]
    await s.deleteTeaRoom(s.teaRooms[0])
    expect(ep.del).toHaveBeenCalledWith(1)
    expect(s.teaRooms.map((r) => r.id)).toEqual([2])
  })
})

describe('saveWebhook', () => {
  it('saves and updates the stored webhook', async () => {
    const s = useTeaRoomsStore()
    const ok = await s.saveWebhook('  https://discord.com/api/webhooks/y  ')
    expect(ok).toBe(true)
    expect(ep.setWebhook).toHaveBeenCalledWith('https://discord.com/api/webhooks/y')
    expect(s.webhookUrl).toBe('https://discord.com/api/webhooks/y')
  })
})
