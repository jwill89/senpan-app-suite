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
  constructor(message: string, status: number) {
    super(message)
    this.name = 'ApiError'
    this.status = status
  }
}

export interface ApiOptions extends Omit<RequestInit, 'body'> {
  // Body objects are auto-stringified (unless FormData).
  body?: unknown
  // Skip the global 401 → "session expired" handler for this request. Set on
  // the auth endpoints themselves (a bad-password login legitimately 401s and
  // must not trigger a redirect/“session expired” toast).
  skipAuthRedirect?: boolean
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
    opts.headers = { 'Content-Type': 'application/json', ...(options.headers || {}) }
    opts.body = JSON.stringify(options.body)
  } else if (options.body instanceof FormData) {
    opts.body = options.body
  } else if (options.body !== undefined) {
    opts.body = options.body as BodyInit
  }

  const res = await fetch(url, opts)
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
    const msg =
      (data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
        ? data.error
        : null) || 'Request failed'
    throw new ApiError(msg, res.status)
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
