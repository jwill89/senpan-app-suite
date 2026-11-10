import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { User } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { useUsersStore } from '@/stores/users'
import { useUiStore } from '@/stores/ui'

function user(overrides: Partial<User> = {}): User {
  return {
    id: 7,
    username: 'tester',
    isAdmin: false,
    isActive: false,
    permissions: ['bingo-cards'],
    createdAt: '2026-11-09T10:00:00Z',
    lastLoginAt: null,
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  Object.values(api).forEach((fn) => fn.mockReset())
  api.GET.mockResolvedValue(ok({ users: [user()] }))
  api.PATCH.mockResolvedValue(ok(user()))
})

describe('users store', () => {
  it('loadUsers fills the list', async () => {
    const users = useUsersStore()
    await users.loadUsers()
    expect(api.GET).toHaveBeenCalledWith('/users')
    expect(users.users).toHaveLength(1)
    expect(users.users[0]!.username).toBe('tester')
  })

  it('updateUser sends only what changed, in one PATCH, then reloads', async () => {
    const users = useUsersStore()
    await users.loadUsers()
    const saved = await users.updateUser(7, {
      isActive: true,
      isAdmin: false,
      permissions: ['bingo-cards'],
    })
    expect(saved).toBe(true)
    expect(api.PATCH).toHaveBeenCalledWith('/users/{id}', {
      params: { path: { id: 7 } },
      body: { isActive: true },
    })
    expect(api.GET).toHaveBeenCalledTimes(2)
  })

  it('updateUser sends new permissions and a password, and none for an admin', async () => {
    const users = useUsersStore()
    await users.loadUsers()
    await users.updateUser(
      7,
      { isActive: false, isAdmin: false, permissions: ['bingo-game'] },
      'pw12345678',
    )
    expect(api.PATCH).toHaveBeenLastCalledWith('/users/{id}', {
      params: { path: { id: 7 } },
      body: { permissions: ['bingo-game'], password: 'pw12345678' },
    })

    await users.updateUser(7, { isActive: false, isAdmin: true, permissions: [] })
    expect(api.PATCH).toHaveBeenLastCalledWith('/users/{id}', {
      params: { path: { id: 7 } },
      body: { isAdmin: true },
    })
  })

  it("updateUser shows the server's reason and reloads when it refuses", async () => {
    api.PATCH.mockResolvedValue({
      error: {
        status: 403,
        title: 'Forbidden',
        detail: "The admin account can't be changed here.",
      },
      response: new Response(null, { status: 403 }),
    })
    const notify = vi.spyOn(useUiStore(), 'notify')
    const users = useUsersStore()
    await users.loadUsers()
    const saved = await users.updateUser(7, {
      isActive: true,
      isAdmin: false,
      permissions: ['bingo-cards'],
    })
    expect(saved).toBe(false)
    expect(notify).toHaveBeenCalledWith("The admin account can't be changed here.", 'error')
    expect(api.GET).toHaveBeenCalledTimes(2)
  })

  it('deleteUser asks first, then deletes and reloads', async () => {
    const ui = useUiStore()
    ui.confirm = vi.fn(async () => true)
    api.DELETE.mockResolvedValue(ok(undefined, 204))
    const users = useUsersStore()
    await users.deleteUser(7, 'tester')
    expect(api.DELETE).toHaveBeenCalledWith('/users/{id}', { params: { path: { id: 7 } } })
    expect(api.GET).toHaveBeenCalled()

    ui.confirm = vi.fn(async () => false)
    api.DELETE.mockClear()
    await users.deleteUser(7, 'tester')
    expect(api.DELETE).not.toHaveBeenCalled()
  })
})
