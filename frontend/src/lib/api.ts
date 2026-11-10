/**
 * What the API client (api/client.ts) leaves to this module: the error every failed
 * call is reported as, the session-expired callback, and the multipart upload, which
 * goes through XHR for its progress events.
 */

// Absolute (leading slash) so requests always resolve against the site root.
export const API_V1_BASE = '/api/v1'

export class ApiError extends Error {
  status: number
  /**
   * The parsed problem details body, if any. Lets callers read what the server
   * included beside the message, such as the `errors` naming each field of a failed
   * validation.
   */
  body?: unknown
  constructor(message: string, status: number, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

// -- Global 401 handler --------------------------------------------------------
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

/** Calls the registered 401 callback: the session has gone. The huma client (api/client.ts) reports through it too. */
export function handleUnauthorized(): void {
  onUnauthorized?.()
}

/** The message an error body carries: a problem details body's `detail`, else null. */
export function errorMessage(data: unknown): string | null {
  if (data && typeof data === 'object' && 'detail' in data && typeof data.detail === 'string') {
    return data.detail || null
  }
  return null
}

/**
 * The files a failed upload refused, each with the reason: a 422 whose `errors` name
 * the files (`body.files[i]`, the file's name as `value`). Empty for any other failure.
 */
export function refusedFiles(e: unknown): { name: string; reason: string }[] {
  if (!(e instanceof ApiError) || !e.body || typeof e.body !== 'object' || !('errors' in e.body)) {
    return []
  }
  const errors = Array.isArray(e.body.errors) ? (e.body.errors as unknown[]) : []
  const out: { name: string; reason: string }[] = []
  for (const err of errors) {
    if (!err || typeof err !== 'object') {
      continue
    }
    const { location, message, value } = err as Record<string, unknown>
    if (typeof location === 'string' && location.startsWith('body.files[')) {
      out.push({
        name: typeof value === 'string' ? value : location,
        reason: typeof message === 'string' ? message : 'Refused',
      })
    }
  }
  return out
}

// -- File uploads --------------------------------------------------------------

export interface UploadOptions {
  // Invoked with 0-100 as the request body is sent. Fires only while bytes are
  // in flight; once the last byte is sent it sits at 100 while the server saves
  // the files and responds.
  onProgress?: (percent: number) => void
  // Cancels the in-flight upload (e.g. the user navigated away).
  signal?: AbortSignal
  // Skip the global 401 -> "session expired" handler.
  skipAuthRedirect?: boolean
}

/**
 * Uploads a multipart form via XMLHttpRequest and returns the parsed JSON.
 *
 * Uploads go through XHR rather than the API client's fetch for two reasons that
 * matter for large files:
 *   1. No client-side timeout. A big image (up to the server's 64 MB cap) over a
 *      slow uplink can legitimately take minutes; a fetch timeout would abort it
 *      mid-transfer with a bogus "timed out" error. Here `xhr.timeout` stays 0
 *      - the transfer takes as long as it takes, and only the network or the user
 *      can end it.
 *   2. Upload progress. fetch exposes no upload-progress events; XHR does (via
 *      `upload.onprogress`), so the UI can show a real percentage instead of an
 *      indeterminate spinner that looks hung on a long upload.
 *
 * Credentials, error extraction, and the global 401 handler match the API client's
 * (api/client.ts), so callers get the same {@link ApiError} contract.
 * @throws {ApiError} with the server's error message if the response is not OK.
 */
export function apiUpload<T = unknown>(
  endpoint: string,
  form: FormData,
  options: UploadOptions = {},
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', `${API_V1_BASE}/${endpoint}`)
    xhr.withCredentials = true // send the admin session cookie, like fetch's credentials:'include'
    xhr.timeout = 0 // never abort on our side; large uploads can take minutes

    const { onProgress } = options
    if (onProgress) {
      xhr.upload.onprogress = (e) => {
        if (e.lengthComputable) {
          onProgress(Math.round((e.loaded / e.total) * 100))
        }
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
      if (xhr.status === 401 && !options.skipAuthRedirect) {
        onUnauthorized?.()
      }
      const serverMsg = errorMessage(data)
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
