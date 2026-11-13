import { describe, it, expect, beforeEach, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import type { User } from '@/api/client'

// The store calls the API client; only its calls are faked (unwrap is the real
// one). Each fake answers a call's `{ data, error, response }`.
const ok = (data: unknown, status = 200) => ({ data, response: new Response(null, { status }) })
const api = vi.hoisted(() => ({ GET: vi.fn(), POST: vi.fn(), PATCH: vi.fn(), DELETE: vi.fn() }))
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
    hasPassword: true,
    discord: null,
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
      username: 'tester',
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
      { username: 'tester', isActive: false, isAdmin: false, permissions: ['bingo-game'] },
      'pw12345678',
    )
    expect(api.PATCH).toHaveBeenLastCalledWith('/users/{id}', {
      params: { path: { id: 7 } },
      body: { permissions: ['bingo-game'], password: 'pw12345678' },
    })

    await users.updateUser(7, {
      username: 'tester',
      isActive: false,
      isAdmin: true,
      permissions: [],
    })
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
      username: 'tester',
      isActive: true,
      isAdmin: false,
      permissions: ['bingo-cards'],
    })
    expect(saved).toBe(false)
    expect(notify).toHaveBeenCalledWith("The admin account can't be changed here.", 'error')
    expect(api.GET).toHaveBeenCalledTimes(2)
  })

  it('updateUser sends a new username, trimmed, and none when it is unchanged', async () => {
    const users = useUsersStore()
    await users.loadUsers()
    await users.updateUser(7, {
      username: '  Kiri  ',
      isActive: false,
      isAdmin: false,
      permissions: ['bingo-cards'],
    })
    expect(api.PATCH).toHaveBeenLastCalledWith('/users/{id}', {
      params: { path: { id: 7 } },
      body: { username: 'Kiri' },
    })
  })

  it('loadDiscordSignIn reads whether Discord sign-in is on, and is off when it cannot', async () => {
    const users = useUsersStore()
    api.GET.mockResolvedValueOnce(ok({ turnstileSiteKey: '', discordSignIn: true }))
    await users.loadDiscordSignIn()
    expect(api.GET).toHaveBeenLastCalledWith('/config')
    expect(users.discordSignIn).toBe(true)
    api.GET.mockRejectedValueOnce(new Error('offline'))
    await users.loadDiscordSignIn()
    expect(users.discordSignIn).toBe(false)
  })

  it('createDiscordInvite makes a sign-up link, or one for an account', async () => {
    const link = {
      url: 'https://apps.example/api/v1/auth/discord/invite/x',
      expiresAt: '2026-11-20T10:00:00Z',
    }
    api.POST.mockResolvedValue(ok(link, 201))
    const users = useUsersStore()
    expect(await users.createDiscordInvite()).toEqual(link)
    expect(api.POST).toHaveBeenLastCalledWith('/discord-invites', { body: {} })
    await users.createDiscordInvite(7)
    expect(api.POST).toHaveBeenLastCalledWith('/discord-invites', { body: { userId: 7 } })
  })

  it("createDiscordInvite shows the server's reason and answers null when refused", async () => {
    api.POST.mockResolvedValue({
      error: {
        status: 403,
        title: 'Forbidden',
        detail: 'Only the admin account can make a link to itself.',
      },
      response: new Response(null, { status: 403 }),
    })
    const notify = vi.spyOn(useUiStore(), 'notify')
    expect(await useUsersStore().createDiscordInvite(1)).toBeNull()
    expect(notify).toHaveBeenCalledWith(
      'Only the admin account can make a link to itself.',
      'error',
    )
  })

  it('unlinkDiscord unlinks and reloads; a refusal is shown', async () => {
    api.DELETE.mockResolvedValue(ok(undefined, 204))
    const users = useUsersStore()
    expect(await users.unlinkDiscord(7)).toBe(true)
    expect(api.DELETE).toHaveBeenCalledWith('/users/{id}/discord', { params: { path: { id: 7 } } })
    expect(api.GET).toHaveBeenCalledWith('/users')

    api.DELETE.mockResolvedValue({
      error: {
        status: 409,
        title: 'Conflict',
        detail: "Discord is this account's only way to sign in.",
      },
      response: new Response(null, { status: 409 }),
    })
    const notify = vi.spyOn(useUiStore(), 'notify')
    expect(await users.unlinkDiscord(7)).toBe(false)
    expect(notify).toHaveBeenCalledWith("Discord is this account's only way to sign in.", 'error')
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
