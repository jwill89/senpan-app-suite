/**
 * Announcement details ↔ Discord embed-field splitting.
 *
 * Discord caps each embed field value at 1024 characters. Long announcement
 * details are posted across multiple consecutive fields (split at a natural line
 * break) so nothing is truncated and the time-first field order is preserved.
 *
 * This mirrors `splitForEmbedFields` in src/internal/server/announcements.go so
 * the editor can warn the admin when a post will be split — keep the two in sync.
 * It runs on the raw editor markdown (the backend additionally normalizes a few
 * markdown artifacts that only ever shorten the text), so the part count here is
 * an upper bound: it never under-reports a split.
 */

/** Discord's per-embed-field value cap. Details beyond this are split. */
export const EMBED_FIELD_VALUE_MAX = 1024

/**
 * Splits `details` into chunks that each fit within `limit` code points, breaking
 * at the last newline within the window (then the last space, then a hard cut).
 * Empty input yields `[]`; text within the cap yields a single chunk.
 */
export function splitDetailParts(details: string, limit = EMBED_FIELD_VALUE_MAX): string[] {
  const text = details.replace(/\r\n/g, '\n').replace(/\r/g, '\n').trim()
  if (text === '') return []
  // Array.from splits by code point (like Go's []rune) so surrogate pairs count as one.
  const chars = Array.from(text)
  if (chars.length <= limit) return [text]

  const parts: string[] = []
  let rest = chars
  while (rest.length > 0) {
    if (rest.length <= limit) {
      const c = rest.join('').trim()
      if (c) parts.push(c)
      break
    }
    const window = rest.slice(0, limit)
    let cut = window.lastIndexOf('\n')
    if (cut <= 0) cut = window.lastIndexOf(' ')
    if (cut <= 0) cut = limit
    const c = rest.slice(0, cut).join('').trim()
    if (c) parts.push(c)
    rest = rest.slice(cut) // leading break char is trimmed off the next chunk
  }
  return parts
}

/** Number of embed fields the details will occupy (>1 means it will be split). */
export function detailPartCount(details: string): number {
  return splitDetailParts(details).length
}
