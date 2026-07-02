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
