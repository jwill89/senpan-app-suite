import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { api, apiGet, apiPost, ApiError, setUnauthorizedHandler, API_BASE } from './api'

/** Builds a minimal Response-like stub for the mocked fetch. */
function jsonResponse(body: unknown, init: { ok?: boolean; status?: number } = {}) {
  return {
    ok: init.ok ?? true,
    status: init.status ?? 200,
    json: async () => body,
  } as Response
}

describe('api()', () => {
  beforeEach(() => {
    vi.stubGlobal('fetch', vi.fn())
    // Reset the module-level 401 handler between tests.
    setUnauthorizedHandler(() => {})
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('prefixes the endpoint with the API base and sends cookies', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ ok: true }))
    await api('auth')
    expect(fetch).toHaveBeenCalledWith(
      `${API_BASE}/auth`,
      expect.objectContaining({ credentials: 'include' }),
    )
  })

  it('JSON-stringifies a plain object body and sets the content-type', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ ok: true }))
    await apiPost('game', { action: 'draw', delay: 3 })
    const [, opts] = vi.mocked(fetch).mock.calls[0]
    expect(opts?.method).toBe('POST')
    expect(opts?.body).toBe('{"action":"draw","delay":3}')
    expect((opts?.headers as Record<string, string>)['Content-Type']).toBe('application/json')
  })

  it('passes FormData through untouched (no JSON content-type)', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ ok: true }))
    const form = new FormData()
    form.append('file', 'x')
    await apiPost('raffles/upload', form)
    const [, opts] = vi.mocked(fetch).mock.calls[0]
    expect(opts?.body).toBeInstanceOf(FormData)
    expect(opts?.headers).toBeUndefined()
  })

  it('returns the parsed JSON payload on success', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ value: 42 }))
    const data = await apiGet<{ value: number }>('settings')
    expect(data.value).toBe(42)
  })

  it('throws an ApiError carrying the server error message and status', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse({ error: 'nope' }, { ok: false, status: 400 }))
    await expect(api('cards')).rejects.toMatchObject({
      name: 'ApiError',
      message: 'nope',
      status: 400,
    })
  })

  it('falls back to a status-bearing message when the error body has no error field', async () => {
    vi.mocked(fetch).mockResolvedValue(jsonResponse(null, { ok: false, status: 500 }))
    await expect(api('cards')).rejects.toBeInstanceOf(ApiError)
    await expect(api('cards')).rejects.toHaveProperty('message', 'Request failed (HTTP 500)')
  })

  it('invokes the global unauthorized handler on a 401', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    vi.mocked(fetch).mockResolvedValue(
      jsonResponse({ error: 'expired' }, { ok: false, status: 401 }),
    )
    await expect(api('game')).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).toHaveBeenCalledOnce()
  })

  it('skips the unauthorized handler when skipAuthRedirect is set', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    vi.mocked(fetch).mockResolvedValue(
      jsonResponse({ error: 'bad password' }, { ok: false, status: 401 }),
    )
    await expect(api('auth', { skipAuthRedirect: true })).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })
})
