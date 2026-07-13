/**
 * Typed REST API client.
 *
 * Mirrors the original `app.js` `api()` helper: same base path (`api/...`),
 * cookie credentials, automatic JSON serialization, and error extraction from
 * the `{ "error": "..." }` response body.
 */

// Absolute (leading slash) so requests always resolve against the site root.
// A relative base ('api') would resolve against the current route — e.g. from
// /admin/cards it would hit /admin/api/... and 404.
export const API_BASE = '/api'

export class ApiError extends Error {
  status: number
  /**
   * The parsed error response body (our `{ error, ... }` JSON), if any. Lets
   * callers read extra fields the server included alongside the message — e.g. a
   * 429's `retry_after` seconds — without a second round-trip.
   */
  body?: unknown
  constructor(message: string, status: number, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

// Default per-request timeout. A request that hasn't responded by now is aborted
// so a hung network can't leave a spinner (or the caller's promise) pending
// forever. Callers that pass their own `signal` opt out of this entirely.
const DEFAULT_TIMEOUT_MS = 30_000

export interface ApiOptions extends Omit<RequestInit, 'body'> {
  // Body objects are auto-stringified (unless FormData).
  body?: unknown
  // Skip the global 401 → "session expired" handler for this request. Set on
  // the auth endpoints themselves (a bad-password login legitimately 401s and
  // must not trigger a redirect/“session expired” toast).
  skipAuthRedirect?: boolean
  // Abort the request after this many ms when the caller doesn't supply its own
  // `signal`. Defaults to 30s; pass 0 to disable the timeout for this request.
  timeoutMs?: number
}

// ── Global 401 handler ────────────────────────────────────────────────────────
// Registered once at startup (see main.ts) so this low-level module stays free
// of router/store imports (avoids a circular dependency). Invoked whenever a
// request that isn't explicitly opted out returns 401, so an expired/cleared
// admin session is handled in one place instead of every call site.
type UnauthorizedHandler = () => void
let onUnauthorized: UnauthorizedHandler | null = null

/** Registers the callback invoked on an unexpected 401 response. */
export function setUnauthorizedHandler(handler: UnauthorizedHandler): void {
  onUnauthorized = handler
}

/**
 * Performs an API request and returns the parsed JSON.
 * @throws {ApiError} with the server's error message if the response is not OK.
 */
export async function api<T = unknown>(endpoint: string, options: ApiOptions = {}): Promise<T> {
  const url = `${API_BASE}/${endpoint}`
  const opts: RequestInit = { credentials: 'include', ...options } as RequestInit

  if (
    options.body !== undefined &&
    options.body !== null &&
    typeof options.body === 'object' &&
    !(options.body instanceof FormData)
  ) {
    opts.headers = {
      'Content-Type': 'application/json',
      ...(options.headers as Record<string, string> | undefined),
    }
    opts.body = JSON.stringify(options.body)
  } else if (options.body instanceof FormData) {
    opts.body = options.body
  } else if (options.body !== undefined) {
    opts.body = options.body as BodyInit
  }

  // Abort/timeout: honor a caller-supplied `signal` as-is (they own its
  // lifetime); otherwise install a default timeout so a hung request can't wait
  // forever. The timer is always cleared in the finally, and an abort is surfaced
  // as a clean error rather than a raw AbortError.
  let timeoutId: ReturnType<typeof setTimeout> | undefined
  if (!options.signal) {
    const timeoutMs = options.timeoutMs ?? DEFAULT_TIMEOUT_MS
    if (timeoutMs > 0) {
      const controller = new AbortController()
      opts.signal = controller.signal
      timeoutId = setTimeout(() => controller.abort(), timeoutMs)
    }
  }

  let res: Response
  try {
    res = await fetch(url, opts)
  } catch (e) {
    if ((e instanceof DOMException || e instanceof Error) && e.name === 'AbortError') {
      // A caller-supplied signal means the caller cancelled; otherwise our own
      // default timeout fired.
      const msg = options.signal ? 'Request cancelled' : 'Request timed out. Please try again.'
      throw new ApiError(msg, 0)
    }
    throw e
  } finally {
    if (timeoutId !== undefined) clearTimeout(timeoutId)
  }

  let data: unknown
  try {
    data = await res.json()
  } catch {
    data = null
  }

  if (!res.ok) {
    // A 401 means the admin session is missing/expired. Surface it once,
    // centrally, unless the caller opted out (the auth endpoints handle their
    // own 401s — e.g. an invalid password).
    if (res.status === 401 && !options.skipAuthRedirect) {
      onUnauthorized?.()
    }
    // Prefer the server's `{ "error": "..." }` message. When the body isn't our
    // JSON (an empty/HTML gateway error from Cloudflare/Apache, e.g. a 502 when
    // the origin is unreachable), fall back to a message that still names the
    // HTTP status so the failure is distinguishable from a generic one.
    const serverMsg =
      data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
        ? data.error
        : null
    const msg = serverMsg || (res.status ? `Request failed (HTTP ${res.status})` : 'Request failed')
    throw new ApiError(msg, res.status, data)
  }

  return data as T
}

// Convenience helpers ---------------------------------------------------------

export function apiGet<T = unknown>(endpoint: string, options: ApiOptions = {}): Promise<T> {
  return api<T>(endpoint, options)
}

export function apiPost<T = unknown>(
  endpoint: string,
  body: unknown,
  options: ApiOptions = {},
): Promise<T> {
  return api<T>(endpoint, { method: 'POST', body, ...options })
}

/** PUT — full replace of the resource at `endpoint` (idempotent). */
export function apiPut<T = unknown>(
  endpoint: string,
  body: unknown,
  options: ApiOptions = {},
): Promise<T> {
  return api<T>(endpoint, { method: 'PUT', body, ...options })
}

/** PATCH — partial/field update of the resource at `endpoint`. */
export function apiPatch<T = unknown>(
  endpoint: string,
  body: unknown,
  options: ApiOptions = {},
): Promise<T> {
  return api<T>(endpoint, { method: 'PATCH', body, ...options })
}

/** DELETE — remove the resource at `endpoint` (idempotent; no body). */
export function apiDelete<T = unknown>(endpoint: string, options: ApiOptions = {}): Promise<T> {
  return api<T>(endpoint, { method: 'DELETE', ...options })
}

// ── File uploads ──────────────────────────────────────────────────────────────

export interface UploadOptions {
  // Invoked with 0–100 as the request body is sent. Fires only while bytes are
  // in flight; once the last byte is sent it sits at 100 while the server saves
  // the files and responds.
  onProgress?: (percent: number) => void
  // Cancels the in-flight upload (e.g. the user navigated away).
  signal?: AbortSignal
  // Skip the global 401 → "session expired" handler (parity with ApiOptions).
  skipAuthRedirect?: boolean
}

/**
 * Uploads a multipart form via XMLHttpRequest and returns the parsed JSON.
 *
 * Uploads go through XHR rather than the fetch-based `api()` for two reasons that
 * matter for large files:
 *   1. No client-side timeout. A big image (up to the server's 64 MB cap) over a
 *      slow uplink can legitimately take minutes; api()'s 30s default would abort
 *      it mid-transfer with a bogus "timed out" error. Here `xhr.timeout` stays 0
 *      — the transfer takes as long as it takes, and only the network or the user
 *      can end it.
 *   2. Upload progress. fetch exposes no upload-progress events; XHR does (via
 *      `upload.onprogress`), so the UI can show a real percentage instead of an
 *      indeterminate spinner that looks hung on a long upload.
 *
 * Credentials, error extraction, and the global 401 handler mirror `api()` so
 * callers get the same {@link ApiError} contract.
 * @throws {ApiError} with the server's error message if the response is not OK.
 */
export function apiUpload<T = unknown>(
  endpoint: string,
  form: FormData,
  options: UploadOptions = {},
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${API_BASE}/${endpoint}`)
    xhr.withCredentials = true // send the admin session cookie, like fetch's credentials:'include'
    xhr.timeout = 0 // never abort on our side; large uploads can take minutes

    const { onProgress } = options
    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
      }
    }

    const fail = (message: string, status: number, body?: unknown) =>
      reject(new ApiError(message, status, body))

    xhr.onload = () => {
      let data: unknown = null
      try {
        data = JSON.parse(xhr.responseText)
      } catch {
        data = null
      }
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve(data as T)
        return
      }
      if (xhr.status === 401 && !options.skipAuthRedirect) onUnauthorized?.()
      const serverMsg =
        data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
          ? data.error
          : null
      fail(
        serverMsg || (xhr.status ? `Upload failed (HTTP ${xhr.status})` : 'Upload failed'),
        xhr.status,
        data,
      )
    }
    xhr.onerror = () => fail('Network error during upload. Please try again.', 0)
    xhr.ontimeout = () => fail('Upload timed out. Please try again.', 0)
    xhr.onabort = () => fail('Upload cancelled', 0)

    if (options.signal) {
      if (options.signal.aborted) {
        fail('Upload cancelled', 0)
        return
      }
      options.signal.addEventListener('abort', () => xhr.abort(), { once: true })
    }

    xhr.send(form)
  })
}
