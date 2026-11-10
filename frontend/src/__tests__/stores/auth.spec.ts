import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { Account } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const problem = (status: number, detail: string) => ({
  error: { status, title: 'Problem', detail },
  response: new Response(null, { status }),
})
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn() }))
vi.mock('@/api/client', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  api,
}))

import { SIGN_UP_DONE, useAuthStore } from '@/stores/auth'

function account(overrides: Partial<Account> = {}): Account {
  return { id: 1, username: 'tester', isAdmin: false, permissions: [], ...overrides }
}

beforeEach(() => {
  setActivePinia(createPinia())
  api.GET.mockReset()
  api.POST.mockReset()
})

describe('auth hasPermission', () => {
  it('is false when logged out', () => {
    const auth = useAuthStore()
    expect(auth.hasPermission('bingo-game')).toBe(false)
  })

  it('is true for any key when the user is an admin', () => {
    const auth = useAuthStore()
    auth.user = account({ isAdmin: true })
    expect(auth.hasPermission('bingo-game')).toBe(true)
    expect(auth.hasPermission('system-users')).toBe(true)
  })

  it('matches only granted keys for non-admin users', () => {
    const auth = useAuthStore()
    auth.user = account({ permissions: ['bingo-cards'] })
    expect(auth.hasPermission('bingo-cards')).toBe(true)
    expect(auth.hasPermission('system-settings')).toBe(false)
  })
})

describe('auth login', () => {
  it('signs in and stores the account the server answers', async () => {
    api.POST.mockResolvedValue(ok({ account: account({ isAdmin: true }) }))
    const auth = useAuthStore()
    const signedIn = await auth.login('admin', 'pw', 'turnstile-ok')
    expect(signedIn).toBe(true)
    expect(auth.isAdmin).toBe(true)
    expect(auth.user?.username).toBe('tester')
    expect(api.POST).toHaveBeenCalledWith('/auth/login', {
      body: { username: 'admin', password: 'pw', turnstileToken: 'turnstile-ok' },
    })
  })

  it("shows the server's reason when signing in fails", async () => {
    api.POST.mockResolvedValue(problem(401, "That username and password don't match."))
    const auth = useAuthStore()
    const signedIn = await auth.login('x', 'y')
    expect(signedIn).toBe(false)
    expect(auth.authError).toBe("That username and password don't match.")
    expect(auth.user).toBeNull()
  })
})

describe('auth register', () => {
  it('says the account waits for an admin', async () => {
    api.POST.mockResolvedValue(ok(undefined, 202))
    const auth = useAuthStore()
    expect(await auth.register('new', 'password123')).toBe(SIGN_UP_DONE)
    expect(api.POST).toHaveBeenCalledWith('/auth/register', {
      body: { username: 'new', password: 'password123', turnstileToken: undefined },
    })
  })

  it("shows the server's reason when it refuses", async () => {
    api.POST.mockResolvedValue(problem(429, 'Too many sign-ups from here. Try again later.'))
    const auth = useAuthStore()
    expect(await auth.register('new', 'password123')).toBeNull()
    expect(auth.authError).toBe('Too many sign-ups from here. Try again later.')
  })
})

describe('auth checkAuth', () => {
  it('populates the user when someone is signed in', async () => {
    api.GET.mockResolvedValue(ok({ account: account({ permissions: ['bingo-game'] }) }))
    const auth = useAuthStore()
    expect(await auth.checkAuth()).toBe(true)
    expect(api.GET).toHaveBeenCalledWith('/me')
    expect(auth.hasPermission('bingo-game')).toBe(true)
    expect(auth.authChecked).toBe(true)
  })

  it('clears the user when nobody is', async () => {
    api.GET.mockResolvedValue(ok({ account: null }))
    const auth = useAuthStore()
    auth.user = account()
    expect(await auth.checkAuth()).toBe(false)
    expect(auth.user).toBeNull()
    expect(auth.isAdmin).toBe(false)
  })
})

describe('auth logout', () => {
  it('clears the user even if the request fails', async () => {
    api.POST.mockRejectedValue(new Error('network'))
    const auth = useAuthStore()
    auth.user = account({ isAdmin: true })
    await auth.logout()
    expect(api.POST).toHaveBeenCalledWith('/auth/logout')
    expect(auth.user).toBeNull()
    expect(auth.isAdmin).toBe(false)
  })
})
