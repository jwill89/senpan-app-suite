import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import {
  apiUpload,
  refusedFiles,
  errorMessage,
  ApiError,
  setUnauthorizedHandler,
  API_V1_BASE,
} from './api'

/**
 * Controllable stand-in for XMLHttpRequest. `apiUpload` wires up the handlers,
 * then calls send(); each test sets `FakeXHR.onSend` to drive the outcome
 * (progress -> onload / onerror / etc.) synchronously.
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

describe('apiUpload()', () => {
  beforeEach(() => {
    setUnauthorizedHandler(() => undefined)
    FakeXHR.instances = []
    FakeXHR.onSend = null
    vi.stubGlobal('XMLHttpRequest', FakeXHR)
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('opens a credentialed POST against the API with no client timeout', async () => {
    FakeXHR.onSend = (xhr) => {
      xhr.status = 200
      xhr.responseText = '{}'
      xhr.onload?.()
    }
    await apiUpload('image-categories/raffles/images', new FormData())
    const xhr = FakeXHR.instances[0]!
    expect(xhr.method).toBe('POST')
    expect(xhr.url).toBe(`${API_V1_BASE}/image-categories/raffles/images`)
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
    const res = await apiUpload<{ uploaded: string[] }>(
      'image-categories/raffles/images',
      new FormData(),
      {
        onProgress: (p) => seen.push(p),
      },
    )
    expect(res.uploaded).toEqual(['a.png'])
    expect(seen).toEqual([50, 100])
  })

  it('throws an ApiError carrying the server error message on a non-2xx response', async () => {
    FakeXHR.onSend = (xhr) => {
      xhr.status = 413
      xhr.responseText = JSON.stringify({
        title: 'Request Entity Too Large',
        status: 413,
        detail: 'Request body is too large',
      })
      xhr.onload?.()
    }
    await expect(
      apiUpload('image-categories/raffles/images', new FormData()),
    ).rejects.toMatchObject({
      name: 'ApiError',
      message: 'Request body is too large',
      status: 413,
    })
  })

  it('invokes the unauthorized handler on a 401 (unless opted out)', async () => {
    const onUnauthorized = vi.fn()
    setUnauthorizedHandler(onUnauthorized)
    FakeXHR.onSend = (xhr) => {
      xhr.status = 401
      xhr.responseText = JSON.stringify({
        title: 'Unauthorized',
        status: 401,
        detail: 'Sign in to continue.',
      })
      xhr.onload?.()
    }
    await expect(
      apiUpload('image-categories/raffles/images', new FormData()),
    ).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).toHaveBeenCalledOnce()

    onUnauthorized.mockClear()
    await expect(
      apiUpload('image-categories/raffles/images', new FormData(), { skipAuthRedirect: true }),
    ).rejects.toBeInstanceOf(ApiError)
    expect(onUnauthorized).not.toHaveBeenCalled()
  })

  it('rejects with a network-error ApiError when the transfer fails', async () => {
    FakeXHR.onSend = (xhr) => xhr.onerror?.()
    await expect(
      apiUpload('image-categories/raffles/images', new FormData()),
    ).rejects.toMatchObject({
      status: 0,
      message: expect.stringContaining('Network error'),
    })
  })
})

describe('refusedFiles()', () => {
  it('lists the files a 422 names, with the reason for each', () => {
    const e = new ApiError('None of the files could be saved.', 422, {
      errors: [
        { location: 'body.files[0]', message: 'Not a valid image', value: 'a.png' },
        { location: 'body.files[1]', message: 'Unsupported type', value: 'b.txt' },
        { location: 'body.name', message: 'unrelated', value: 'x' },
      ],
    })
    expect(refusedFiles(e)).toEqual([
      { name: 'a.png', reason: 'Not a valid image' },
      { name: 'b.txt', reason: 'Unsupported type' },
    ])
  })

  it('is empty for any other failure', () => {
    expect(refusedFiles(new ApiError('boom', 500))).toEqual([])
    expect(refusedFiles(new Error('boom'))).toEqual([])
    expect(refusedFiles(null)).toEqual([])
  })
})

describe('errorMessage()', () => {
  it("reads a problem's detail", () => {
    expect(errorMessage({ title: 'Conflict', detail: 'That name is taken.' })).toBe(
      'That name is taken.',
    )
    expect(errorMessage({ title: 'Conflict' })).toBeNull()
    expect(errorMessage('nope')).toBeNull()
  })
})
