/**
 * Small framework-agnostic formatting helpers shared across admin tabs.
 */

/**
 * Slugifies a string for use as a URL folder / on-disk directory name, mirroring
 * the server's slug rules. Lowercases and trims, folds runs of whitespace and the
 * opposite word-joiner into `sep`, drops any other non-alphanumeric characters,
 * collapses repeated separators, and trims leading/trailing separators.
 *
 * @param sep the separator (and the word character kept): `'-'` for public URL
 *   folders (Carrd projects) or `'_'` for image directories.
 */
export function slugify(s: string, sep: '-' | '_' = '-'): string {
  const fold = sep === '-' ? /[\s_]+/g : /[\s-]+/g
  const strip = sep === '-' ? /[^a-z0-9-]/g : /[^a-z0-9_]/g
  const collapse = sep === '-' ? /-+/g : /_+/g
  const trim = sep === '-' ? /^-+|-+$/g : /^_+|_+$/g
  return s
    .toLowerCase()
    .trim()
    .replace(fold, sep)
    .replace(strip, '')
    .replace(collapse, sep)
    .replace(trim, '')
}

/** Human-readable file size (B / KB / MB). */
export function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}
