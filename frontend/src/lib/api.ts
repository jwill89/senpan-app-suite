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
    const msg =
      (data && typeof data === 'object' && 'error' in data && typeof data.error === 'string'
        ? data.error
        : null) || 'Request failed'
    throw new ApiError(msg, res.status)
  }

  return data as T
}

// Convenience helpers ---------------------------------------------------------

export function apiGet<T = unknown>(endpoint: string): Promise<T> {
  return api<T>(endpoint)
}

export function apiPost<T = unknown>(endpoint: string, body: unknown): Promise<T> {
  return api<T>(endpoint, { method: 'POST', body })
}
