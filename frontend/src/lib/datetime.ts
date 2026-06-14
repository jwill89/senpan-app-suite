/**
 * Date/time helpers for converting between stored UTC timestamps and the value
 * an `<input type="datetime-local">` works with (LOCAL wall-clock, no timezone).
 *
 * Raffle availability dates are stored/transmitted as UTC (RFC-3339, e.g.
 * "2026-06-13T20:00:00.000Z"). The datetime-local input, however, always deals
 * in the *browser's* local time. Converting on the way in/out means an admin in
 * one timezone sees the correct local time for a window another admin set in a
 * different timezone — and the server can compare a single, unambiguous instant.
 */

/**
 * Converts a stored UTC timestamp into the "YYYY-MM-DDTHH:mm" (LOCAL time) value
 * a datetime-local input expects. Returns '' for empty/invalid input.
 *
 * Accepts both RFC-3339 UTC strings ("…Z" / with an offset) and legacy naive
 * strings (no timezone), the latter interpreted as UTC to match how the server
 * historically compared them.
 */
export function utcToDatetimeLocal(value: string | null | undefined): string {
  if (!value) return ''
  // A trailing Z or ±HH:MM means the string already carries its zone; otherwise
  // it's a legacy naive value we treat as UTC by appending Z.
  const hasZone = /[zZ]$|[+-]\d{2}:?\d{2}$/.test(value)
  const date = new Date(hasZone ? value : `${value}Z`)
  if (Number.isNaN(date.getTime())) return ''
  const pad = (n: number): string => String(n).padStart(2, '0')
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
    `T${pad(date.getHours())}:${pad(date.getMinutes())}`
  )
}

/**
 * Converts a datetime-local input value (LOCAL wall-clock, no timezone) into a
 * UTC RFC-3339 string for storage. Returns '' for empty/invalid input.
 */
export function datetimeLocalToUtc(value: string | null | undefined): string {
  if (!value) return ''
  // `new Date("YYYY-MM-DDTHH:mm")` parses the (zone-less) value as LOCAL time.
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return ''
  return date.toISOString()
}

/**
 * Parses a backend timestamp into epoch milliseconds.
 *
 * Handles SQLite's `CURRENT_TIMESTAMP` form ("YYYY-MM-DD HH:MM:SS" — UTC,
 * space-separated, no zone) as well as ISO-8601 strings. A zone-less value is
 * always treated as UTC, never local — `new Date("YYYY-MM-DD HH:MM:SS")` would
 * otherwise interpret it as local time and shift it by the viewer's offset.
 * Returns NaN for empty/invalid input.
 */
export function parseServerTimestamp(ts: string | null | undefined): number {
  if (!ts) return NaN
  // Already carries a zone designator (Z or ±HH:MM) → parse as-is.
  if (/[zZ]$|[+-]\d{2}:?\d{2}$/.test(ts)) return new Date(ts).getTime()
  // Otherwise treat as UTC: normalize the space form to ISO and append Z.
  return new Date(ts.replace(' ', 'T') + 'Z').getTime()
}

/**
 * Formats a backend (UTC) timestamp as a localized date+time string in the
 * viewer's timezone. Returns '' for empty/invalid input.
 */
export function formatServerTimestamp(ts: string | null | undefined): string {
  const ms = parseServerTimestamp(ts)
  return Number.isFinite(ms) ? new Date(ms).toLocaleString() : ''
}
