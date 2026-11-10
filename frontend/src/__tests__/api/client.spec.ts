import { describe, it, expect, beforeEach, vi } from 'vitest'

// The client takes fetch and Request when it is made, so the fakes go in first:
// fetch answers what each test says, and Request resolves the client's relative
// /api/v1 address against a test origin, as a browser does against the page.
const fetchMock = vi.hoisted(() => {
  const f = vi.fn()
  vi.stubGlobal('fetch', f)
  const Base = globalThis.Request
  vi.stubGlobal(
    'Request',
    class extends Base {
      constructor(input: RequestInfo | URL, init?: RequestInit) {
        super(typeof input === 'string' ? new URL(input, 'http://test.invalid') : input, init)
      }
    },
  )
  return f
})
const handleUnauthorized = vi.hoisted(() => vi.fn())
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<object>()),
  handleUnauthorized,
}))

import { api, unwrap } from '@/api/client'

/** A problem details answer with status. */
function problem(status: number, detail: string): Response {
  return new Response(JSON.stringify({ status, title: 'Problem', detail }), {
    status,
    headers: { 'Content-Type': 'application/problem+json' },
  })
}

beforeEach(() => {
  fetchMock.mockReset()
  handleUnauthorized.mockReset()
})

describe('the API client', () => {
  it('reads a 401 anywhere as the session having gone', async () => {
    fetchMock.mockResolvedValue(problem(401, 'Sign in to continue.'))
    await expect(unwrap(api.GET('/me/token'))).rejects.toThrow('Sign in to continue.')
    expect(handleUnauthorized).toHaveBeenCalledTimes(1)
  })

  it('reads a 401 from signing in as wrong credentials, not an expired session', async () => {
    fetchMock.mockResolvedValue(problem(401, "That username and password don't match."))
    await expect(
      unwrap(api.POST('/auth/login', { body: { username: 'yao', password: 'nope' } })),
    ).rejects.toThrow("That username and password don't match.")
    fetchMock.mockResolvedValue(problem(401, "That passkey didn't sign you in."))
    await expect(
      unwrap(api.POST('/auth/passkey/finish', { body: { credential: {} } })),
    ).rejects.toThrow("That passkey didn't sign you in.")
    expect(handleUnauthorized).not.toHaveBeenCalled()
  })

  it('carries the problem with its status on any other failure', async () => {
    fetchMock.mockResolvedValue(problem(422, 'The current password is wrong.'))
    const failed = await unwrap(
      api.PUT('/me/password', { body: { currentPassword: 'x', newPassword: 'password123' } }),
    ).catch((e: unknown) => e)
    expect(failed).toMatchObject({ status: 422, message: 'The current password is wrong.' })
    expect(handleUnauthorized).not.toHaveBeenCalled()
  })
})
