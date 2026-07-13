import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { api, apiGet, apiPost, apiUpload, ApiError, setUnauthorizedHandler, API_BASE } from './api'

/** Builds a minimal Response-like stub for the mocked fetch. */
function jsonResponse(body: unknown, init: { ok?: boolean; status?: number } = {}) {
  return {
    ok: init.ok ?? true,
    status: init.status ?? 200,
    json: async () => body,
  } as Response
}

/**
 * Controllable stand-in for XMLHttpRequest. `apiUpload` wires up the handlers,
 * then calls send(); each test sets `FakeXHR.onSend` to drive the outcome
 * (progress → onload / onerror / etc.) synchronously.
 */
class FakeXHR {
  static instances: FakeXHR[] = []
  static onSend: ((xhr: FakeXHR) => void) | null = null
  method = ''
  url = ''
  withCredentials = false
  timeout = -1
  status = 0
  responseText = ''
  upload: { onprogress: ((e: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  ontimeout: (() => void) | null = null
  onabort: (() => void) | null = null
  constructor() {
    FakeXHR.instances.push(this)
  }
  open(method: string, url: string) {
    this.method = method
    this.url = url
  }
  abort() {
    this.onabort?.()
  }
  send() {
    FakeXHR.onSend?.(this)
  }
  /** Fire a progress event with the given fraction (as apiUpload expects it). */
  progress(loaded: number, total: number) {
    this.upload.onprogress?.({ lengthComputable: true, loaded, total } as unknown as ProgressEvent)
  }
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

describe('apiUpload()', () => {
  beforeEach(() => {
    setUnauthorizedHandler(() => {})
    FakeXHR.instances = []
    FakeXHR.onSend = null
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens a credentialed POST against the API base with no client timeout', async () => {
    FakeXHR.onSend = (xhr) => {
      xhr.status = 200
      xhr.responseText = '{}'
      xhr.onload?.()
    }
    await apiUpload('images/upload', new FormData())
    const xhr = FakeXHR.instances[0]
    expect(xhr.method).toBe('POST')
    expect(xhr.url).toBe(`${API_BASE}/images/upload`)
    expect(xhr.withCredentials).toBe(true)
    expect(xhr.timeout).toBe(0) // large uploads must never be aborted on our side
  })

  it('resolves with parsed JSON and reports upload progress', async () => {
    FakeXHR.onSend = (xhr) => {
      xhr.progress(50, 100)
      xhr.progress(100, 100)
      xhr.status = 200
      xhr.responseText = JSON.stringify({ uploaded: ['a.png'], skipped: [] })
      xhr.onload?.()
    }
    const seen: number[] = []
    const res = await apiUpload<{ uploaded: string[] }>('images/upload', new FormData(), {
      onProgress: (p) => seen.push(p),
    })
    expect(res.uploaded).toEqual(['a.png'])
    expect(seen).toEqual([50, 100])
  })

  it('throws an ApiError carrying the server error message on a non-2xx response', async () => {
    FakeXHR.onSend = (xhr) => {
      xhr.status = 400
      xhr.responseText = JSON.stringify({ error: 'Upload failed (max 64MB total)' })
      xhr.onload?.()
    }
    await expect(apiUpload('images/upload', new FormData())).rejects.toMatchObject({
      name: 'ApiError',
      message: 'Upload failed (max 64MB total)',
      status: 400,
    })
  })

  it('invokes the unauthorized handler on a 401 (unless opted out)', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    FakeXHR.onSend = (xhr) => {
      xhr.status = 401
      xhr.responseText = JSON.stringify({ error: 'expired' })
      xhr.onload?.()
    }
    await expect(apiUpload('images/upload', new FormData())).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).toHaveBeenCalledOnce()

    onUnauthorized.mockClear()
    await expect(
      apiUpload('images/upload', new FormData(), { skipAuthRedirect: true }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('rejects with a network-error ApiError when the transfer fails', async () => {
    FakeXHR.onSend = (xhr) => xhr.onerror?.()
    await expect(apiUpload('images/upload', new FormData())).rejects.toMatchObject({
      status: 0,
      message: expect.stringContaining('Network error'),
    })
  })
})
