import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { User } from '@/types/api'

const { list, setActive, setAdmin, setPermissions, setPassword } = vi.hoisted(() => ({
  list: vi.fn(),
  setActive: vi.fn(async () => ({})),
  setAdmin: vi.fn(async () => ({})),
  setPermissions: vi.fn(async () => ({})),
  setPassword: vi.fn(async () => ({})),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { users: { list, setActive, setAdmin, setPermissions, setPassword } },
}))

import { useUsersStore } from './users'

function user(overrides: Partial<User> = {}): User {
  return {
    id: 7,
    username: 'tester',
    is_admin: false,
    is_active: false,
    permissions: [],
    created_at: '',
    last_login_at: '',
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  list.mockReset()
  setActive.mockClear()
  setAdmin.mockClear()
  setPermissions.mockClear()
  setPassword.mockClear()
  list.mockResolvedValue({ users: [user()] })
})

describe('users store', () => {
  it('loadUsers fills the list from the endpoint', async () => {
    const users = useUsersStore()
    await users.loadUsers()
    expect(users.users).toHaveLength(1)
    expect(users.users[0].username).toBe('tester')
  })

  it('setActive calls the endpoint then refreshes', async () => {
    const users = useUsersStore()
    await users.setActive(7, true)
    expect(setActive).toHaveBeenCalledWith(7, true)
    expect(list).toHaveBeenCalled()
  })

  it('setAdmin calls the endpoint', async () => {
    const users = useUsersStore()
    await users.setAdmin(7, true)
    expect(setAdmin).toHaveBeenCalledWith(7, true)
  })

  it('setPermissions forwards the keys and returns true on success', async () => {
    const users = useUsersStore()
    const ok = await users.setPermissions(7, ['bingo-game'])
    expect(ok).toBe(true)
    expect(setPermissions).toHaveBeenCalledWith(7, ['bingo-game'])
  })

  it('setPassword returns false when the endpoint rejects', async () => {
    setPassword.mockRejectedValueOnce(new Error('boom'))
    const users = useUsersStore()
    const ok = await users.setPassword(7, 'password123')
    expect(ok).toBe(false)
  })
})
