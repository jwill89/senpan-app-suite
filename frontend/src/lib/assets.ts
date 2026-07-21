/**
 * Asset URL helper.
 *
 * Uploaded images (e.g. raffle prizes) are stored server-side as ROOT-RELATIVE
 * web paths like "images/raffles/abc.png" (no leading slash — see migration
 * v10). Bound directly to an <img src>, such a path resolves against the
 * current SPA route (e.g. /admin/raffles/images/raffles/abc.png) and 404s.
 *
 * `assetUrl` normalizes any such path to an absolute, document-root-relative
 * URL ("/images/raffles/abc.png") so it always resolves regardless of route.
 * Already-absolute paths, full URLs, and data: URIs are returned unchanged.
 */
export function assetUrl(path: string | null | undefined): string {
  if (!path) return ''
  // Absolute path, protocol-relative/full URL, or inline data URI — leave as-is.
  if (path.startsWith('/') || /^(https?:)?\/\//i.test(path) || path.startsWith('data:')) {
    return path
  }
  return '/' + path
}

/**
 * The asset URL escaped so it is safe to interpolate inside a double-quoted CSS
 * `url("…")` token. `assetUrl` returns protocol-relative and `data:` forms
 * verbatim (fine for an `<img src>`, but a `"`, `\`, or newline inside such a
 * value could terminate the string and inject further CSS at a `url()` sink), so
 * this backslash-escapes `"` and `\` and drops control characters. Callers supply
 * the surrounding quotes: `url("${assetCssUrl(path)}")`.
 */
export function assetCssUrl(path: string | null | undefined): string {
  const url = assetUrl(path)
  let out = ''
  for (const ch of url) {
    const code = ch.charCodeAt(0)
    if (code < 0x20 || code === 0x7f) continue // drop control chars / newlines
    if (ch === '"' || ch === '\\') out += '\\' // escape for the quoted CSS string
    out += ch
  }
  return out
}
