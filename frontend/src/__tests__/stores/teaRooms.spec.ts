import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { TeaRoom } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({
  GET: vi.fn(),
  POST: vi.fn(),
  PUT: vi.fn(),
  PATCH: vi.fn(),
  DELETE: vi.fn(),
}))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { teaRoomUnlockedMessage, useTeaRoomsStore } from '@/stores/teaRooms'
import { useUiStore } from '@/stores/ui'

/** Minimal tea room with the fields the store reads. */
function room(over: Partial<TeaRoom> = {}): TeaRoom {
  return {
    id: 1,
    name: 'R',
    subtitle: '',
    roomOwner: '',
    roomNumber: '1',
    costPerHalfHour: 0,
    hashtags: '',
    description: '',
    seasonal: false,
    open: true,
    lockable: false,
    discounted: false,
    locked: false,
    lockedUntil: null,
    image: '',
    color: '#ff3131',
    sortOrder: 0,
    createdAt: null,
    ...over,
  }
}

/** The body of the PATCH made for a room. */
function patched(id: number): object {
  const call = api.PATCH.mock.calls.find(
    (c) => (c[1] as { params: { path: { id: number } } }).params.path.id === id,
  )
  return (call![1] as { body: object }).body
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockImplementation(async (path: string) =>
    path === '/tea-rooms/webhook' ? ok({ webhookUrl: '' }) : ok({ teaRooms: [] }),
  )
  api.POST.mockResolvedValue(ok(room(), 201))
  api.PUT.mockResolvedValue(ok(room()))
  api.PATCH.mockImplementation(
    async (_path: string, init: { params: { path: { id: number } }; body: object }) =>
      ok(room({ id: init.params.path.id, ...init.body })),
  )
  api.DELETE.mockResolvedValue(ok(undefined, 204))
})

describe('teaRoomUnlockedMessage', () => {
  const tail =
    'has been marked as unlocked. Please check the room to remind the occupants or extend the time.'

  it('names the room and its number', () => {
    expect(teaRoomUnlockedMessage('Jasmine Room', '3')).toBe(`Jasmine Room (Room #3) ${tail}`)
  })

  it('does not double a # the number already carries', () => {
    expect(teaRoomUnlockedMessage('Jasmine Room', '#3')).toBe(`Jasmine Room (Room #3) ${tail}`)
  })

  it('drops the number when there is none, and names a nameless room generically', () => {
    expect(teaRoomUnlockedMessage('Jasmine Room', ' ')).toBe(`Jasmine Room ${tail}`)
    expect(teaRoomUnlockedMessage('', '')).toBe(`A tea room ${tail}`)
  })
})

describe('loading', () => {
  it('loadTeaRooms populates the list and webhook', async () => {
    api.GET.mockImplementation(async (path: string) =>
      path === '/tea-rooms/webhook'
        ? ok({ webhookUrl: 'https://discord.com/api/webhooks/x' })
        : ok({ teaRooms: [room(), room({ id: 2 })] }),
    )
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
    s.editTeaRoom(room({ id: 7, name: 'Jasmine', costPerHalfHour: 125000, seasonal: true }))
    expect(s.teaRoomForm).toMatchObject({
      id: 7,
      name: 'Jasmine',
      costPerHalfHour: 125000,
      seasonal: true,
    })
  })
})

describe('filteredTeaRooms', () => {
  it('filters by name / room number / hashtags', () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [
      room({ id: 1, name: 'Jasmine', hashtags: '#cozy' }),
      room({ id: 2, name: 'Rose', roomNumber: 'West 3' }),
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
    expect(api.POST).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('requires a room number', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.newTeaRoomForm()
    s.teaRoomForm!.name = 'Jasmine' // room number still blank
    const saved = await s.saveTeaRoom()
    expect(saved).toBe(false)
    expect(api.POST).not.toHaveBeenCalled()
  })

  it('creates with the id stripped from the payload', async () => {
    const s = useTeaRoomsStore()
    s.newTeaRoomForm()
    s.teaRoomForm!.name = 'Jasmine'
    s.teaRoomForm!.roomNumber = '12'
    s.teaRoomForm!.costPerHalfHour = 125000
    const saved = await s.saveTeaRoom()
    expect(saved).toBe(true)
    expect(api.POST).toHaveBeenCalledTimes(1)
    const [path, init] = api.POST.mock.calls[0] as [string, { body: Record<string, unknown> }]
    expect(path).toBe('/tea-rooms')
    expect(init.body).not.toHaveProperty('id')
    expect(init.body.name).toBe('Jasmine')
    expect(init.body.lockedUntil).toBeNull()
    expect(s.teaRoomForm).toBeNull()
  })

  it('updates when the form carries an id (id passed separately)', async () => {
    const s = useTeaRoomsStore()
    s.editTeaRoom(room({ id: 4, name: 'Rose' }))
    const saved = await s.saveTeaRoom()
    expect(saved).toBe(true)
    expect(api.PUT).toHaveBeenCalledTimes(1)
    const [path, init] = api.PUT.mock.calls[0] as [string, { params: { path: { id: number } } }]
    expect(path).toBe('/tea-rooms/{id}')
    expect(init.params.path.id).toBe(4)
    expect(api.POST).not.toHaveBeenCalled()
  })
})

describe('toggles', () => {
  it('toggleOpen patches the opposite flag and replaces the row', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1, open: true })]
    await s.toggleOpen(s.teaRooms[0]!)
    expect(patched(1)).toEqual({ open: false })
    expect(s.teaRooms[0]!.open).toBe(false)
  })

  it('toggleDiscounted patches the opposite flag', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1, discounted: false })]
    await s.toggleDiscounted(s.teaRooms[0]!)
    expect(patched(1)).toEqual({ discounted: true })
    expect(s.teaRooms[0]!.discounted).toBe(true)
  })
})

describe('locking', () => {
  it('locks with no expiry when no unlock time is given', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    expect(await s.lockRoom(s.teaRooms[0]!, '')).toBe(true)
    expect(patched(1)).toEqual({ locked: true })
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

    expect(await s.lockRoom(s.teaRooms[0]!, local)).toBe(true)
    const fields = patched(1) as { locked: boolean; lockedUntil: string }
    expect(fields.locked).toBe(true)
    // The same instant, expressed in UTC - not the local string passed straight
    // through, which would land the unlock at the wrong moment for anyone else.
    expect(fields.lockedUntil).toMatch(/Z$/)
    expect(new Date(fields.lockedUntil).getTime()).toBe(new Date(local).getTime())
  })

  it('refuses an unlock time that has already passed', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    expect(await s.lockRoom(s.teaRooms[0]!, '2020-01-01T00:00')).toBe(false)
    expect(api.PATCH).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('unlockRoom clears the lock', async () => {
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 4, locked: true, lockedUntil: '2030-01-01T00:00:00Z' })]
    await s.unlockRoom(s.teaRooms[0]!)
    expect(patched(4)).toEqual({ locked: false })
  })
})

describe('postRoom', () => {
  it('refuses to post without a webhook', async () => {
    const ui = useUiStore()
    ui.notify = vi.fn()
    const s = useTeaRoomsStore()
    s.webhookUrl = ''
    await s.postRoom(room({ id: 1 }))
    expect(api.POST).not.toHaveBeenCalled()
    expect(ui.notify).toHaveBeenCalled()
  })

  it('posts after confirmation when a webhook is set', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useTeaRoomsStore()
    s.webhookUrl = 'https://discord.com/api/webhooks/x'
    await s.postRoom(room({ id: 3 }))
    expect(api.POST).toHaveBeenCalledWith('/tea-rooms/{id}/post', {
      params: { path: { id: 3 } },
    })
  })
})

describe('deleteTeaRoom', () => {
  it('does nothing when the user cancels', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => false)
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 })]
    await s.deleteTeaRoom(s.teaRooms[0]!)
    expect(api.DELETE).not.toHaveBeenCalled()
    expect(s.teaRooms).toHaveLength(1)
  })

  it('deletes and drops the room when confirmed', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    const s = useTeaRoomsStore()
    s.teaRooms = [room({ id: 1 }), room({ id: 2 })]
    await s.deleteTeaRoom(s.teaRooms[0]!)
    expect(api.DELETE).toHaveBeenCalledWith('/tea-rooms/{id}', { params: { path: { id: 1 } } })
    expect(s.teaRooms.map((r) => r.id)).toEqual([2])
  })
})

describe('saveWebhook', () => {
  it('saves and updates the stored webhook', async () => {
    const s = useTeaRoomsStore()
    api.PUT.mockResolvedValueOnce(ok({ webhookUrl: 'https://discord.com/api/webhooks/y' }))
    const saved = await s.saveWebhook('  https://discord.com/api/webhooks/y  ')
    expect(saved).toBe(true)
    expect(api.PUT).toHaveBeenCalledWith('/tea-rooms/webhook', {
      body: { webhookUrl: 'https://discord.com/api/webhooks/y' },
    })
    expect(s.webhookUrl).toBe('https://discord.com/api/webhooks/y')
  })
})
