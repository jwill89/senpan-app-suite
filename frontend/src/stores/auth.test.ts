import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { User } from '@/types/api'

const { check, login, logout, register } = vi.hoisted(() => ({
  check: vi.fn(),
  login: vi.fn(),
  logout: vi.fn(),
  register: vi.fn(),
}))
vi.mock('@/lib/endpoints', () => ({
  endpoints: { auth: { check, login, logout, register } },
}))

import { useAuthStore } from './auth'

function user(overrides: Partial<User> = {}): User {
  return {
    id: 1,
    username: 'tester',
    is_admin: false,
    is_active: true,
    permissions: [],
    created_at: '',
    last_login_at: '',
    ...overrides,
  }
}

beforeEach(() => {
  setActivePinia(createPinia())
  check.mockReset()
  login.mockReset()
  logout.mockReset()
  register.mockReset()
})

describe('auth hasPermission', () => {
  it('is false when logged out', () => {
    const auth = useAuthStore()
    expect(auth.hasPermission('bingo-game')).toBe(false)
  })

  it('is true for any key when the user is an admin', () => {
    const auth = useAuthStore()
    auth.user = user({ is_admin: true })
    expect(auth.hasPermission('bingo-game')).toBe(true)
    expect(auth.hasPermission('system-users')).toBe(true)
  })

  it('matches only granted keys for non-admin users', () => {
    const auth = useAuthStore()
    auth.user = user({ permissions: ['bingo-cards'] })
    expect(auth.hasPermission('bingo-cards')).toBe(true)
    expect(auth.hasPermission('system-settings')).toBe(false)
  })
})

describe('auth login', () => {
  it('stores the returned user and sets isAdmin', async () => {
    login.mockResolvedValue({ success: true, user: user({ is_admin: true }) })
    const auth = useAuthStore()
    const ok = await auth.login('admin', 'pw')
    expect(ok).toBe(true)
    expect(auth.isAdmin).toBe(true)
    expect(auth.user?.username).toBe('tester')
    // The third arg is the optional Turnstile token (undefined when the bot
    // check isn't in play).
    expect(login).toHaveBeenCalledWith('admin', 'pw', undefined)
  })

  it('captures the error message on failure', async () => {
    login.mockRejectedValue(new Error('Invalid username or password'))
    const auth = useAuthStore()
    const ok = await auth.login('x', 'y')
    expect(ok).toBe(false)
    expect(auth.authError).toBe('Invalid username or password')
    expect(auth.user).toBeNull()
  })
})

describe('auth checkAuth', () => {
  it('populates the user when authenticated', async () => {
    check.mockResolvedValue({ authenticated: true, user: user({ permissions: ['bingo-game'] }) })
    const auth = useAuthStore()
    await auth.checkAuth()
    expect(auth.hasPermission('bingo-game')).toBe(true)
    expect(auth.authChecked).toBe(true)
  })

  it('clears the user when not authenticated', async () => {
    check.mockResolvedValue({ authenticated: false, user: null })
    const auth = useAuthStore()
    auth.user = user()
    await auth.checkAuth()
    expect(auth.user).toBeNull()
    expect(auth.isAdmin).toBe(false)
  })
})

describe('auth logout', () => {
  it('clears the user even if the request fails', async () => {
    logout.mockRejectedValue(new Error('network'))
    const auth = useAuthStore()
    auth.user = user({ is_admin: true })
    await auth.logout()
    expect(auth.user).toBeNull()
    expect(auth.isAdmin).toBe(false)
  })
})
